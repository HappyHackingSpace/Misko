// Package application contains the experiment side of automatic analysis.
// An orchestrator submits a job to the analysis service when an animal of a
// test is ready (a verified current video, an effective calibration with every
// required zone, a non-excluded allocation and a running or completed
// experiment) and links the job to the animal. Users list an animal's
// analyses and the review of its result with *:read, and reanalyse and review
// with test:run.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/domain"
)

var (
	ErrTestSubjectNotFound = errors.New("test subject not found")
	ErrNotReady            = errors.New("the animal needs a verified current video of a test that is not cancelled, a running or completed experiment, no exclusion and, when the paradigm requires one, a valid calibration of its arena with every required zone")
	ErrRunPending          = errors.New("an analysis of this animal is already queued or running")
	ErrRunNotFound         = errors.New("analysis not found for this animal")
)

// Analysis is the analysis service's port as this module uses it.
type Analysis interface {
	// Submit returns ErrNotReady when the service refuses the request's
	// inputs (the video, the calibration or the paradigm version).
	Submit(ctx context.Context, req domain.JobRequest) (domain.Job, error)
	// Jobs returns the known jobs among ids.
	Jobs(ctx context.Context, ids []string) ([]domain.Job, error)
	// Metrics returns the published metrics of the jobs by id.
	Metrics(ctx context.Context, ids []string) (map[string][]domain.Metric, error)
}

// Catalog tells what a paradigm version requires before analysis.
type Catalog interface {
	Requirement(key string, version int) (domain.Requirement, error)
	// Calibrated lists the paradigm versions ("KEY@version") that need a calibration.
	Calibrated() []string
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	// ReadyTestSubjects lists eligible animals with a valid effective
	// calibration when their paradigm version is among calibrated and no
	// automatic job for the current video and calibration. Every returned
	// candidate is eligible.
	ReadyTestSubjects(ctx context.Context, calibrated []string) ([]domain.Candidate, error)
	// Candidate returns ErrTestSubjectNotFound for an unknown test subject.
	Candidate(ctx context.Context, testSubjectID string) (domain.Candidate, error)
	// LockTestSubject serializes job submission for one animal until the
	// transaction ends.
	LockTestSubject(ctx context.Context, testSubjectID string) error
	// CreateLink reports false when an equal automatic link exists.
	CreateLink(ctx context.Context, l domain.Link) (bool, error)
	// Link returns ErrRunNotFound when the job analyses no animal.
	Link(ctx context.Context, jobID string) (domain.Link, error)
	// Links lists the links of animals, newest first.
	Links(ctx context.Context, testSubjectIDs []string) ([]domain.Link, error)
	// TestLinks lists the links of a test, of one animal when testSubjectID is set.
	TestLinks(ctx context.Context, testID, testSubjectID string) ([]domain.Link, error)
	// Unopened lists animals with a succeeded job and no result, each with its
	// first succeeded job.
	Unopened(ctx context.Context, limit int) ([]domain.Link, error)
	// OpenResult creates the PENDING result of an animal unless it has one; a
	// job that is not linked to the animal opens nothing.
	OpenResult(ctx context.Context, r domain.SubjectResult) error
	// LockResult locks an animal's result and returns its experiment's status,
	// or ErrResultNotFound.
	LockResult(ctx context.Context, testSubjectID string) (domain.SubjectResult, string, error)
	UpdateResult(ctx context.Context, r domain.SubjectResult) (domain.SubjectResult, error)
	ReviewItems(ctx context.Context, f ReviewFilter) ([]ReviewItem, int, error)
}

type Service struct {
	store    Store
	analysis Analysis
	catalog  Catalog
	now      func() time.Time
}

func New(store Store, analysis Analysis, catalog Catalog, now func() time.Time) *Service {
	return &Service{store: store, analysis: analysis, catalog: catalog, now: now}
}

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// TickReport counts what an orchestrator tick did. Skipped holds animals that
// could not be submitted; a tick logs them and carries on, so one bad row
// never blocks the others.
type TickReport struct {
	Submitted, Opened int
	Skipped           []error
}

// unopenedLimit bounds the results one tick opens.
const unopenedLimit = 500

// Tick submits one automatic job per ready animal and opens the result of
// every animal whose first job succeeded without the analysis service telling
// this module. Concurrent ticks submit each automatic job once.
func (s *Service) Tick(ctx context.Context) (TickReport, error) {
	var report TickReport
	candidates, err := s.store.ReadyTestSubjects(ctx, s.catalog.Calibrated())
	if err != nil {
		return TickReport{}, err
	}
	for _, c := range candidates {
		created, err := s.submitAutomatic(ctx, c)
		if err != nil {
			report.Skipped = append(report.Skipped, fmt.Errorf("test subject %s: %w", c.TestSubjectID, err))
			continue
		}
		if created {
			report.Submitted++
		}
	}
	unopened, err := s.store.Unopened(ctx, unopenedLimit)
	if err != nil {
		return TickReport{}, err
	}
	for _, l := range unopened {
		if err := s.store.OpenResult(ctx, domain.OpenResult(l.TestSubjectID, l.JobID)); err != nil {
			report.Skipped = append(report.Skipped, fmt.Errorf("result of test subject %s: %w", l.TestSubjectID, err))
			continue
		}
		report.Opened++
	}
	return report, nil
}

func (s *Service) submitAutomatic(ctx context.Context, c domain.Candidate) (bool, error) {
	if c.Invalid != nil {
		return false, c.Invalid
	}
	req, err := s.catalog.Requirement(c.ParadigmKey, c.ParadigmVersion)
	if err != nil {
		return false, err
	}
	if !c.Ready(req) {
		return false, ErrNotReady
	}
	job, err := s.analysis.Submit(ctx, c.Request(req, domain.Automatic, ""))
	if err != nil {
		return false, err
	}
	return s.store.CreateLink(ctx, c.Link(req, job.ID, domain.Automatic, ""))
}

// JobSucceeded opens the PENDING result of the animal a succeeded job
// analysed, unless the animal has one. It is told about every success the
// analysis service has for an animal of a test.
func (s *Service) JobSucceeded(ctx context.Context, jobID, testSubjectID string) error {
	return s.store.OpenResult(ctx, domain.OpenResult(testSubjectID, jobID))
}

// Reanalyze submits a new manual job of one animal with the current inputs:
// the test's current video and the arena's current effective calibration.
// Earlier jobs, their results and the reviewed result stay unchanged.
func (s *Service) Reanalyze(ctx context.Context, actor access.Actor, testSubjectID string) (domain.Run, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Run{}, err
	}
	c, err := s.store.Candidate(ctx, testSubjectID)
	if err != nil {
		return domain.Run{}, err
	}
	req, err := s.catalog.Requirement(c.ParadigmKey, c.ParadigmVersion)
	if err != nil || !c.Ready(req) {
		return domain.Run{}, ErrNotReady
	}
	var out domain.Run
	err = s.store.Transaction(ctx, func(tx Store) error {
		// One analysis per animal at a time: a repeated request must not flood the workers.
		if err := tx.LockTestSubject(ctx, c.TestSubjectID); err != nil {
			return err
		}
		earlier, err := s.runs(ctx, tx, []string{c.TestSubjectID})
		if err != nil {
			return err
		}
		if domain.AnyPending(earlier) {
			return ErrRunPending
		}
		job, err := s.analysis.Submit(ctx, c.Request(req, domain.Manual, actor.UserID))
		if err != nil {
			return err
		}
		link := c.Link(req, job.ID, domain.Manual, actor.UserID)
		if _, err := tx.CreateLink(ctx, link); err != nil {
			return err
		}
		link.CreatedAt = job.CreatedAt
		out = domain.Run{Link: link, Job: job}
		return nil
	})
	return out, err
}

// Runs lists the analyses of a test, newest first, optionally of one animal.
func (s *Service) Runs(ctx context.Context, actor access.Actor, testID, testSubjectID string) ([]domain.Run, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	links, err := s.store.TestLinks(ctx, testID, testSubjectID)
	if err != nil {
		return nil, err
	}
	return s.withJobs(ctx, links)
}

// runs reads every analysis of the animals with the jobs' state.
func (s *Service) runs(ctx context.Context, store Store, testSubjectIDs []string) ([]domain.Run, error) {
	links, err := store.Links(ctx, testSubjectIDs)
	if err != nil {
		return nil, err
	}
	return s.withJobs(ctx, links)
}

// portBatch keeps every port call within the analysis service's batch limit.
const portBatch = 500

// withJobs joins links with their jobs; a link whose job the service does not
// report is an inconsistency and fails the call.
func (s *Service) withJobs(ctx context.Context, links []domain.Link) ([]domain.Run, error) {
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.JobID)
	}
	jobs := make(map[string]domain.Job, len(ids))
	for start := 0; start < len(ids); start += portBatch {
		batch, err := s.analysis.Jobs(ctx, ids[start:min(start+portBatch, len(ids))])
		if err != nil {
			return nil, err
		}
		for _, j := range batch {
			jobs[j.ID] = j
		}
	}
	out := make([]domain.Run, 0, len(links))
	for _, l := range links {
		job, ok := jobs[l.JobID]
		if !ok {
			return nil, fmt.Errorf("analysis job %s of test subject %s is unknown to the analysis service", l.JobID, l.TestSubjectID)
		}
		out = append(out, domain.Run{Link: l, Job: job})
	}
	return out, nil
}
