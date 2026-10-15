package application

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

var (
	ErrResultNotFound   = errors.New("this animal has no analysis result yet")
	ErrExperimentClosed = errors.New("results can be reviewed only while the experiment is running")
	ErrInvalidQuery     = errors.New("invalid result query")
	ErrInvalidBulk      = errors.New("bulk acceptance needs 1 to 100 distinct test subjects")
)

// MaxBulkAccept bounds one bulk acceptance.
const MaxBulkAccept = 100

var uuidPattern = regexp.MustCompile(`^(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ReviewFilter selects results. An empty ExperimentID or TestSubjectID does
// not filter; an empty Status matches every status.
type ReviewFilter struct {
	ExperimentID, TestSubjectID, Status string
	Limit, Offset                       int
}

// ReviewGroup is what a reviewer may learn about the animal's group. It is
// withheld for blinded experiments.
type ReviewGroup struct {
	ID, Code, Name, Role string
	DiseaseModelName     string
	SubstanceName        string
	DoseMicro            *int64
	Unit                 string
}

// ReviewRun describes the run under review.
type ReviewRun struct {
	ID, Trigger, ModelVersion, CalibrationID, RecordingID string
	MetricEngineVersion                                   int
	FinishedAt                                            *time.Time
}

// ReviewItem is one animal's result as the review screen shows it: animal,
// arena and test, the run under review with its metrics, whether a newer
// succeeded run exists, and the group unless the experiment is blinded.
type ReviewItem struct {
	Result                                                domain.SubjectResult
	TestID, ExperimentID, ExperimentCode                  string
	ExperimentStatus                                      string
	Blinded                                               bool
	SubjectCode                                           string
	Excluded                                              bool
	EnvironmentRevisionID, EnvironmentID, EnvironmentName string
	EnvironmentRevision                                   int
	TimepointID, TimepointName                            string
	ParadigmKey                                           string
	ParadigmVersion                                       int
	ScheduledAt                                           time.Time
	Run                                                   ReviewRun
	LatestRunID                                           string
	Metrics                                               []domain.MetricValue
	Group                                                 *ReviewGroup
}

// NewerRunAvailable reports a succeeded run newer than the one under review.
func (i ReviewItem) NewerRunAvailable() bool { return i.LatestRunID != i.Result.RunID }

type ReviewQuery struct {
	ExperimentID, Status string
	Page, PageSize       int
}

type ReviewPage struct {
	Items                 []ReviewItem
	Total, Page, PageSize int
}

// Results lists results for review, PENDING by default and oldest first.
// Status ALL lists every status.
func (s *Service) Results(ctx context.Context, actor access.Actor, q ReviewQuery) (ReviewPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return ReviewPage{}, err
	}
	status := q.Status
	switch status {
	case "":
		status = string(domain.ResultPending)
	case "ALL":
		status = ""
	case string(domain.ResultPending), string(domain.ResultAccepted), string(domain.ResultRejected):
	default:
		return ReviewPage{}, ErrInvalidQuery
	}
	if (q.ExperimentID != "" && !uuidPattern.MatchString(q.ExperimentID)) || q.Page < 0 || q.Page > 1<<20 || q.PageSize < 0 {
		return ReviewPage{}, ErrInvalidQuery
	}
	page, size := max(q.Page, 1), min(q.PageSize, 200)
	if size == 0 {
		size = 50
	}
	items, total, err := s.store.ReviewItems(ctx, ReviewFilter{ExperimentID: q.ExperimentID, Status: status, Limit: size, Offset: (page - 1) * size})
	if err != nil {
		return ReviewPage{}, err
	}
	if items, err = s.withMetrics(ctx, items); err != nil {
		return ReviewPage{}, err
	}
	return ReviewPage{Items: items, Total: total, Page: page, PageSize: size}, nil
}

// ResultDetail is one animal's result with every succeeded run a reviewer can choose.
type ResultDetail struct {
	Item ReviewItem
	Runs []domain.Run
}

func (s *Service) Result(ctx context.Context, actor access.Actor, testSubjectID string) (ResultDetail, error) {
	if err := actor.Require(access.Read); err != nil {
		return ResultDetail{}, err
	}
	item, err := s.item(ctx, testSubjectID)
	if err != nil {
		return ResultDetail{}, err
	}
	runs, err := s.store.SubjectRuns(ctx, testSubjectID)
	if err != nil {
		return ResultDetail{}, err
	}
	succeeded := slices.DeleteFunc(runs, func(r domain.Run) bool { return r.Status != domain.Succeeded })
	return ResultDetail{Item: item, Runs: succeeded}, nil
}

// Accept records that an animal's result counts.
func (s *Service) Accept(ctx context.Context, actor access.Actor, testSubjectID, note string) (ReviewItem, error) {
	return s.review(ctx, actor, testSubjectID, func(r domain.SubjectResult, _ Store, at time.Time) (domain.SubjectResult, error) {
		return r.Accept(actor.UserID, note, at)
	})
}

// Reject keeps an animal's measurement out of statistics with a reason.
func (s *Service) Reject(ctx context.Context, actor access.Actor, testSubjectID, reason, note string) (ReviewItem, error) {
	return s.review(ctx, actor, testSubjectID, func(r domain.SubjectResult, _ Store, at time.Time) (domain.SubjectResult, error) {
		return r.Reject(actor.UserID, reason, note, at)
	})
}

// ChooseRun puts another succeeded run of the same animal under review and
// sends the result back to PENDING.
func (s *Service) ChooseRun(ctx context.Context, actor access.Actor, testSubjectID, runID string) (ReviewItem, error) {
	return s.review(ctx, actor, testSubjectID, func(r domain.SubjectResult, tx Store, _ time.Time) (domain.SubjectResult, error) {
		run, err := tx.Run(ctx, runID)
		if err != nil {
			return domain.SubjectResult{}, err
		}
		return r.Choose(run)
	})
}

// AcceptMany accepts several clean results at once, all or none.
func (s *Service) AcceptMany(ctx context.Context, actor access.Actor, testSubjectIDs []string, note string) ([]ReviewItem, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return nil, err
	}
	ids := slices.Sorted(slices.Values(testSubjectIDs))
	if len(ids) == 0 || len(ids) > MaxBulkAccept || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		return nil, ErrInvalidBulk
	}
	at := s.clock()
	err := s.store.Transaction(ctx, func(tx Store) error {
		// Sorted ids lock in one order, so two bulk requests cannot deadlock.
		for _, id := range ids {
			if err := s.decide(ctx, tx, id, at, func(r domain.SubjectResult, _ Store, at time.Time) (domain.SubjectResult, error) {
				return r.Accept(actor.UserID, note, at)
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]ReviewItem, 0, len(testSubjectIDs))
	for _, id := range testSubjectIDs {
		item, err := s.item(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type decision func(domain.SubjectResult, Store, time.Time) (domain.SubjectResult, error)

func (s *Service) review(ctx context.Context, actor access.Actor, testSubjectID string, decide decision) (ReviewItem, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return ReviewItem{}, err
	}
	at := s.clock()
	if err := s.store.Transaction(ctx, func(tx Store) error { return s.decide(ctx, tx, testSubjectID, at, decide) }); err != nil {
		return ReviewItem{}, err
	}
	return s.item(ctx, testSubjectID)
}

// decide locks one result and its experiment's status and stores a decision.
// Completing an experiment freezes its results.
func (s *Service) decide(ctx context.Context, tx Store, testSubjectID string, at time.Time, decide decision) error {
	current, experimentStatus, err := tx.LockResult(ctx, testSubjectID)
	if err != nil {
		return err
	}
	if experimentStatus != "RUNNING" {
		return ErrExperimentClosed
	}
	next, err := decide(current, tx, at)
	if err != nil {
		return err
	}
	_, err = tx.UpdateResult(ctx, next)
	return err
}

func (s *Service) item(ctx context.Context, testSubjectID string) (ReviewItem, error) {
	if !uuidPattern.MatchString(testSubjectID) {
		return ReviewItem{}, ErrResultNotFound
	}
	items, _, err := s.store.ReviewItems(ctx, ReviewFilter{TestSubjectID: testSubjectID, Limit: 1})
	if err != nil {
		return ReviewItem{}, err
	}
	if len(items) == 0 {
		return ReviewItem{}, ErrResultNotFound
	}
	items, err = s.withMetrics(ctx, items)
	if err != nil {
		return ReviewItem{}, err
	}
	return items[0], nil
}

// withMetrics adds the metrics of each run under review and withholds the
// group of animals in blinded experiments.
func (s *Service) withMetrics(ctx context.Context, items []ReviewItem) ([]ReviewItem, error) {
	ids := make([]string, 0, len(items))
	for _, i := range items {
		ids = append(ids, i.Result.RunID)
	}
	metrics := map[string][]domain.MetricValue{}
	if len(ids) > 0 {
		var err error
		if metrics, err = s.store.RunMetrics(ctx, ids); err != nil {
			return nil, err
		}
	}
	for n := range items {
		items[n].Metrics = metrics[items[n].Result.RunID]
		if items[n].Blinded {
			items[n].Group = nil
		}
	}
	return items, nil
}
