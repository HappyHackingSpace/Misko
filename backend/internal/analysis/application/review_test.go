package application

import (
	"context"
	"errors"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

// reviewStore holds results by test subject; experiment is the status
// LockResult reports.
type reviewStore struct {
	fakeStore
	results    map[string]domain.SubjectResult
	blinded    bool
	experiment string
	runs       map[string]domain.Run
	filters    []ReviewFilter
}

func (r *reviewStore) Transaction(_ context.Context, fn func(Store) error) error {
	// Decisions apply only when the whole transaction succeeds.
	saved := map[string]domain.SubjectResult{}
	for k, v := range r.results {
		saved[k] = v
	}
	if err := fn(r); err != nil {
		r.results = saved
		return err
	}
	return nil
}

func (r *reviewStore) LockResult(_ context.Context, id string) (domain.SubjectResult, string, error) {
	res, ok := r.results[id]
	if !ok {
		return domain.SubjectResult{}, "", ErrResultNotFound
	}
	return res, r.experiment, nil
}

func (r *reviewStore) UpdateResult(_ context.Context, res domain.SubjectResult) (domain.SubjectResult, error) {
	r.results[res.TestSubjectID] = res
	return res, nil
}

func (r *reviewStore) Run(_ context.Context, id string) (domain.Run, error) {
	run, ok := r.runs[id]
	if !ok {
		return domain.Run{}, ErrRunNotFound
	}
	return run, nil
}

func (r *reviewStore) ReviewItems(_ context.Context, f ReviewFilter) ([]ReviewItem, int, error) {
	r.filters = append(r.filters, f)
	var out []ReviewItem
	for id, res := range r.results {
		if f.TestSubjectID != "" && id != f.TestSubjectID {
			continue
		}
		out = append(out, ReviewItem{Result: res, Blinded: r.blinded, SubjectCode: "M-" + id[len(id)-2:], LatestRunID: "run-2",
			Group: &ReviewGroup{ID: "g1", Code: "G1", Role: "TREATMENT", SubstanceName: "Compound X"}})
	}
	return out, len(out), nil
}

func (r *reviewStore) RunMetrics(_ context.Context, ids []string) (map[string][]domain.MetricValue, error) {
	v := 12.5
	out := map[string][]domain.MetricValue{}
	for _, id := range ids {
		out[id] = []domain.MetricValue{{Key: "distance_cm", Unit: "cm", Value: &v}}
	}
	return out, nil
}

func (r *reviewStore) SubjectRuns(context.Context, string) ([]domain.Run, error) {
	return []domain.Run{r.runs["run-2"], {ID: "run-x", Status: domain.Failed}, r.runs["run-1"]}, nil
}

const otherSubject = "01a00000-0000-7000-8000-0000000000c2"

func newReviewStore() *reviewStore {
	return &reviewStore{
		experiment: "RUNNING",
		results: map[string]domain.SubjectResult{
			subjectID:    {TestSubjectID: subjectID, RunID: "run-1", Status: domain.ResultPending},
			otherSubject: {TestSubjectID: otherSubject, RunID: "run-9", Status: domain.ResultPending},
		},
		runs: map[string]domain.Run{
			"run-1": {ID: "run-1", Source: domain.Source{TestSubjectID: subjectID}, Status: domain.Succeeded},
			"run-2": {ID: "run-2", Source: domain.Source{TestSubjectID: subjectID}, Status: domain.Succeeded},
			"run-9": {ID: "run-9", Source: domain.Source{TestSubjectID: otherSubject}, Status: domain.Succeeded},
		},
	}
}

func reviewService(store *reviewStore) *Service {
	return New(store, &fakeObjects{}, &fakeCatalog{}, fakeTokens{}, settings, func() time.Time { return now })
}

func TestReviewDecisionsAndBlinding(t *testing.T) {
	reviewer := access.Actor{UserID: "reviewer", Role: access.Technician}
	store := newReviewStore()
	s := reviewService(store)

	item, err := s.Accept(ctx, reviewer, subjectID, "ok")
	if err != nil || item.Result.Status != domain.ResultAccepted || item.Result.ReviewedBy != "reviewer" || !item.Result.ReviewedAt.Equal(now) ||
		len(item.Metrics) != 1 || item.Group == nil || !item.NewerRunAvailable() {
		t.Fatalf("accept: %+v %v", item, err)
	}
	if item, err = s.Reject(ctx, reviewer, subjectID, "WRONG_ANIMAL", ""); err != nil || item.Result.Reason != "WRONG_ANIMAL" {
		t.Fatalf("reject: %+v %v", item, err)
	}
	if _, err := s.Reject(ctx, reviewer, subjectID, "", ""); !errors.Is(err, domain.ErrInvalidRejection) {
		t.Fatalf("reject without reason: %v", err)
	}
	if item, err = s.ChooseRun(ctx, reviewer, subjectID, "run-2"); err != nil || item.Result.RunID != "run-2" || item.Result.Status != domain.ResultPending || item.NewerRunAvailable() {
		t.Fatalf("choose newer run: %+v %v", item, err)
	}
	if _, err := s.ChooseRun(ctx, reviewer, subjectID, "run-9"); !errors.Is(err, domain.ErrRunNotChosen) {
		t.Fatalf("run of another animal: %v", err)
	}
	if _, err := s.ChooseRun(ctx, reviewer, subjectID, "missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("unknown run: %v", err)
	}
	detail, err := s.Result(ctx, reviewer, subjectID)
	if err != nil || len(detail.Runs) != 2 {
		t.Fatalf("detail lists succeeded runs only: %+v %v", detail.Runs, err)
	}
	if _, err := s.Accept(ctx, reviewer, "01a00000-0000-7000-8000-0000000000ff", ""); !errors.Is(err, ErrResultNotFound) {
		t.Fatalf("animal without a result: %v", err)
	}

	// A blinded experiment never reveals the group to the reviewer.
	store.blinded = true
	page, err := s.Results(ctx, reviewer, ReviewQuery{})
	if err != nil || len(page.Items) != 2 || page.PageSize != 50 || store.filters[len(store.filters)-1].Status != "PENDING" {
		t.Fatalf("pending list: %+v %v", page, err)
	}
	for _, i := range page.Items {
		if i.Group != nil {
			t.Fatalf("blinded item reveals its group: %+v", i)
		}
	}
	if item, err := s.Accept(ctx, reviewer, subjectID, ""); err != nil || item.Group != nil {
		t.Fatalf("blinded accept: %+v %v", item, err)
	}
	if _, err := s.Results(ctx, reviewer, ReviewQuery{Status: "ALL", PageSize: 500}); err != nil || store.filters[len(store.filters)-1].Status != "" || store.filters[len(store.filters)-1].Limit != 200 {
		t.Fatalf("all statuses: %+v %v", store.filters, err)
	}
	for _, q := range []ReviewQuery{{Status: "DONE"}, {ExperimentID: "E1"}, {Page: -1}} {
		if _, err := s.Results(ctx, reviewer, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}

	// Completing the experiment freezes every decision.
	store.experiment = "COMPLETED"
	if _, err := s.Reject(ctx, reviewer, subjectID, "OTHER", ""); !errors.Is(err, ErrExperimentClosed) {
		t.Fatalf("review after completion: %v", err)
	}
}

func TestBulkAcceptanceIsAllOrNothing(t *testing.T) {
	reviewer := access.Actor{UserID: "reviewer", Role: access.Researcher}
	store := newReviewStore()
	s := reviewService(store)
	for name, ids := range map[string][]string{
		"empty":     nil,
		"duplicate": {subjectID, subjectID},
		"too many":  make([]string, MaxBulkAccept+1),
	} {
		if _, err := s.AcceptMany(ctx, reviewer, ids, ""); !errors.Is(err, ErrInvalidBulk) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := s.AcceptMany(ctx, reviewer, []string{subjectID, "01a00000-0000-7000-8000-0000000000ff"}, ""); !errors.Is(err, ErrResultNotFound) ||
		store.results[subjectID].Status != domain.ResultPending {
		t.Fatalf("a missing result rolls the others back: %v %+v", err, store.results[subjectID])
	}
	items, err := s.AcceptMany(ctx, reviewer, []string{otherSubject, subjectID}, "batch")
	if err != nil || len(items) != 2 || items[0].Result.TestSubjectID != otherSubject || store.results[subjectID].Status != domain.ResultAccepted ||
		store.results[otherSubject].Note != "batch" {
		t.Fatalf("bulk: %+v %v", items, err)
	}
}
