package application

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/domain"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
)

const (
	subjectID    = "01a00000-0000-7000-8000-0000000000c1"
	otherSubject = "01a00000-0000-7000-8000-0000000000c2"
)

var errStoreCalled = errors.New("store called")

func guard(fn func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = errStoreCalled
		}
	}()
	return fn()
}

func second[T any](_ T, err error) error { return err }

func TestUserOperationsRequireTheirPermission(t *testing.T) {
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"reanalyze", access.TestRun, func(s *Service, a access.Actor) error { return second(s.Reanalyze(ctx, a, subjectID)) }},
		{"list runs", access.Read, func(s *Service, a access.Actor) error { return second(s.Runs(ctx, a, "test", "")) }},
		{"list results", access.Read, func(s *Service, a access.Actor) error { return second(s.Results(ctx, a, ReviewQuery{})) }},
		{"result", access.Read, func(s *Service, a access.Actor) error { return second(s.Result(ctx, a, subjectID)) }},
		{"accept", access.TestRun, func(s *Service, a access.Actor) error { return second(s.Accept(ctx, a, subjectID, "")) }},
		{"reject", access.TestRun, func(s *Service, a access.Actor) error { return second(s.Reject(ctx, a, subjectID, "OTHER", "")) }},
		{"choose run", access.TestRun, func(s *Service, a access.Actor) error { return second(s.ChooseRun(ctx, a, subjectID, "job-2")) }},
		{"bulk accept", access.TestRun, func(s *Service, a access.Actor) error { return second(s.AcceptMany(ctx, a, []string{subjectID}, "")) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error {
				return op.run(New(&fakeStore{}, &fakeAnalysis{}, fakeCatalog{}, time.Now), access.Actor{UserID: "u", Role: role})
			})
			if role.Allows(op.permission) {
				if errors.Is(err, access.ErrForbidden) {
					t.Errorf("%s %s: denied", role, op.name)
				}
				continue
			}
			if !errors.Is(err, access.ErrForbidden) {
				t.Errorf("%s %s: err=%v want forbidden before any store call", role, op.name, err)
			}
		}
	}
}

// ready is an eligible animal with a verified current video.
func ready(id, key, calibration string, zones ...string) domain.Candidate {
	return domain.Candidate{ExperimentID: "e", TestID: "t-" + id, TestSubjectID: id, RecordingID: "rec", SourceAssetID: "a1", ParadigmKey: key, ParadigmVersion: 1,
		CalibrationID: calibration, ZoneKeys: zones, VideoVerified: true, ExperimentStatus: "RUNNING"}
}

// The orchestrator submits ready animals once, skips a failing one without
// losing the others and opens results the analysis service did not report.
func TestTickSubmitsReadyAnimalsAndOpensResults(t *testing.T) {
	store := &fakeStore{
		candidates: []domain.Candidate{
			ready("ready", "MWM", "cal-1"),
			ready("broken", "MWM", "cal-2"),
			ready("zones missing", "ZONED", "cal-3", "lanes"),
			ready("zoned", "ZONED", "cal-4", "lanes", "center"),
			ready("unknown", "MAZE", "cal-5"),
			ready("again", "MWM", "cal-6"),
		},
		existing: map[string]bool{"again": true},
		unopened: []domain.Link{{TestSubjectID: subjectID, JobID: "job-old"}},
	}
	analysis := &fakeAnalysis{fail: map[string]error{"broken": errors.New("source store down")}}
	report, err := New(store, analysis, fakeCatalog{}, func() time.Time { return now }).Tick(ctx)
	if err != nil || report.Submitted != 2 || report.Opened != 1 || len(report.Skipped) != 3 {
		t.Fatalf("tick: %+v %v", report, err)
	}
	if !slices.Equal(store.askedCalibrated, []string{"MWM@1", "ZONED@1"}) {
		t.Fatalf("calibrated versions asked: %v", store.askedCalibrated)
	}
	var subjects []string
	for _, r := range analysis.submitted {
		subjects = append(subjects, r.TestSubjectID)
		if r.Trigger != domain.Automatic || !strings.HasPrefix(r.IdempotencyKey, "test-subject:"+r.TestSubjectID) {
			t.Errorf("submitted: %+v", r)
		}
	}
	if !slices.Equal(subjects, []string{"ready", "broken", "zoned", "again"}) {
		t.Fatalf("submitted: %v", subjects)
	}
	if len(store.links) != 2 || store.links[0].CalibrationID != "cal-1" || store.links[0].Trigger != domain.Automatic {
		t.Fatalf("links: %+v", store.links)
	}
	if store.results[subjectID].JobID != "job-old" || store.results[subjectID].Status != domain.ResultPending {
		t.Fatalf("opened: %+v", store.results)
	}
	joined := ""
	for _, e := range report.Skipped {
		joined += e.Error() + "|"
	}
	for _, want := range []string{"test subject broken: source store down", "test subject zones missing", "test subject unknown"} {
		if !strings.Contains(joined, want) {
			t.Errorf("skipped %q missing from %s", want, joined)
		}
	}
}

func TestReanalysisNeedsAReadyAnimalAndNoPendingJob(t *testing.T) {
	technician := access.Actor{UserID: "tech", Role: access.Technician}
	for name, mutate := range map[string]func(*domain.Candidate){
		"no video":             func(c *domain.Candidate) { c.RecordingID = "" },
		"video not verified":   func(c *domain.Candidate) { c.VideoVerified = false },
		"test cancelled":       func(c *domain.Candidate) { c.TestCancelled = true },
		"excluded animal":      func(c *domain.Candidate) { c.Excluded = true },
		"experiment draft":     func(c *domain.Candidate) { c.ExperimentStatus = "DRAFT" },
		"no calibration":       func(c *domain.Candidate) { c.CalibrationID = "" },
		"paradigm not in code": func(c *domain.Candidate) { c.ParadigmKey = "MAZE" },
	} {
		c := ready(subjectID, "MWM", "cal-1")
		mutate(&c)
		analysis := &fakeAnalysis{}
		if _, err := New(&fakeStore{candidate: c}, analysis, fakeCatalog{}, time.Now).Reanalyze(ctx, technician, subjectID); !errors.Is(err, ErrNotReady) ||
			len(analysis.submitted) != 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	store := &fakeStore{candidate: ready(subjectID, "MWM", "cal-1"), existingLinks: []domain.Link{{JobID: "job-1", TestSubjectID: subjectID}}}
	analysis := &fakeAnalysis{jobs: map[string]domain.Job{"job-1": {ID: "job-1", Status: domain.JobSucceeded}}}
	run, err := New(store, analysis, fakeCatalog{}, time.Now).Reanalyze(ctx, technician, subjectID)
	if err != nil || run.Trigger != domain.Manual || run.TestSubjectID != subjectID || run.CreatedBy != "tech" || store.locked != subjectID ||
		analysis.submitted[0].IdempotencyKey != "" || run.Job.ID == "" {
		t.Fatalf("manual run: %+v %v", run, err)
	}
	analysis.jobs["job-1"] = domain.Job{ID: "job-1", Status: domain.JobRunning}
	if _, err := New(store, analysis, fakeCatalog{}, time.Now).Reanalyze(ctx, technician, subjectID); !errors.Is(err, ErrRunPending) {
		t.Fatalf("pending: %v", err)
	}
}

func newReviewStore() *fakeStore {
	return &fakeStore{
		experiment: "RUNNING",
		results: map[string]domain.SubjectResult{
			subjectID:    {TestSubjectID: subjectID, JobID: "job-1", Status: domain.ResultPending},
			otherSubject: {TestSubjectID: otherSubject, JobID: "job-9", Status: domain.ResultPending},
		},
		existingLinks: []domain.Link{{JobID: "job-1", TestSubjectID: subjectID}, {JobID: "job-2", TestSubjectID: subjectID},
			{JobID: "job-x", TestSubjectID: subjectID}, {JobID: "job-9", TestSubjectID: otherSubject}},
	}
}

func reviewAnalysis() *fakeAnalysis {
	first, second := now.Add(-time.Hour), now
	return &fakeAnalysis{jobs: map[string]domain.Job{
		"job-1": {ID: "job-1", Status: domain.JobSucceeded, FinishedAt: &first, ModelVersion: "tracker 1"},
		"job-2": {ID: "job-2", Status: domain.JobSucceeded, FinishedAt: &second, ModelVersion: "tracker 2"},
		"job-x": {ID: "job-x", Status: domain.JobFailed, FinishedAt: &second},
		"job-9": {ID: "job-9", Status: domain.JobSucceeded, FinishedAt: &first},
	}}
}

func TestReviewDecisionsAndBlinding(t *testing.T) {
	reviewer := access.Actor{UserID: "reviewer", Role: access.Technician}
	store := newReviewStore()
	s := New(store, reviewAnalysis(), fakeCatalog{}, func() time.Time { return now })

	item, err := s.Accept(ctx, reviewer, subjectID, "ok")
	if err != nil || item.Result.Status != domain.ResultAccepted || item.Result.ReviewedBy != "reviewer" || !item.Result.ReviewedAt.Equal(now) ||
		len(item.Metrics) != 1 || item.Group == nil || !item.NewerRunAvailable() || item.LatestRunID != "job-2" || item.Run.ModelVersion != "tracker 1" {
		t.Fatalf("accept: %+v %v", item, err)
	}
	if item, err = s.Reject(ctx, reviewer, subjectID, "WRONG_ANIMAL", ""); err != nil || item.Result.Reason != "WRONG_ANIMAL" {
		t.Fatalf("reject: %+v %v", item, err)
	}
	if _, err := s.Reject(ctx, reviewer, subjectID, "", ""); !errors.Is(err, domain.ErrInvalidRejection) {
		t.Fatalf("reject without reason: %v", err)
	}
	if item, err = s.ChooseRun(ctx, reviewer, subjectID, "job-2"); err != nil || item.Result.JobID != "job-2" || item.Result.Status != domain.ResultPending ||
		item.NewerRunAvailable() {
		t.Fatalf("choose newer job: %+v %v", item, err)
	}
	for name, tc := range map[string]struct {
		job  string
		want error
	}{
		"job of another animal": {"job-9", domain.ErrRunNotChosen},
		"failed job":            {"job-x", domain.ErrRunNotChosen},
		"unknown job":           {"missing", ErrRunNotFound},
	} {
		if _, err := s.ChooseRun(ctx, reviewer, subjectID, tc.job); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	detail, err := s.Result(ctx, reviewer, subjectID)
	if err != nil || len(detail.Runs) != 2 {
		t.Fatalf("detail lists succeeded jobs only: %+v %v", detail.Runs, err)
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
	if _, err := s.Results(ctx, reviewer, ReviewQuery{Status: "ALL", PageSize: 500}); err != nil || store.filters[len(store.filters)-1].Status != "" ||
		store.filters[len(store.filters)-1].Limit != 200 {
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
	s := New(store, reviewAnalysis(), fakeCatalog{}, func() time.Time { return now })
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

func TestSucceededJobsOpenTheResultOnce(t *testing.T) {
	store := &fakeStore{}
	s := New(store, &fakeAnalysis{}, fakeCatalog{}, time.Now)
	if err := s.JobSucceeded(ctx, "job-1", subjectID); err != nil {
		t.Fatal(err)
	}
	if err := s.JobSucceeded(ctx, "job-2", subjectID); err != nil {
		t.Fatal(err)
	}
	if store.results[subjectID].JobID != "job-1" {
		t.Fatalf("the first success stays under review: %+v", store.results[subjectID])
	}
}

// fakeCatalog: MWM needs a calibration, ZONED needs one with two zones, MAZE is unknown.
type fakeCatalog struct{}

func (fakeCatalog) Requirement(key string, version int) (domain.Requirement, error) {
	switch {
	case key == "MWM" && version == 1:
		return domain.Requirement{CalibrationRequired: true}, nil
	case key == "ZONED" && version == 1:
		return domain.Requirement{CalibrationRequired: true, RequiredZones: []string{"center", "lanes"}}, nil
	}
	return domain.Requirement{}, errors.New("unknown paradigm version")
}

func (fakeCatalog) Calibrated() []string { return []string{"MWM@1", "ZONED@1"} }

type fakeAnalysis struct {
	submitted []domain.JobRequest
	fail      map[string]error
	jobs      map[string]domain.Job
}

func (f *fakeAnalysis) Submit(_ context.Context, req domain.JobRequest) (domain.Job, error) {
	f.submitted = append(f.submitted, req)
	if err := f.fail[req.TestSubjectID]; err != nil {
		return domain.Job{}, err
	}
	return domain.Job{ID: "job-" + req.TestSubjectID, Status: domain.JobQueued, CreatedAt: now}, nil
}

func (f *fakeAnalysis) Jobs(_ context.Context, ids []string) ([]domain.Job, error) {
	var out []domain.Job
	for _, id := range ids {
		if j, ok := f.jobs[id]; ok {
			out = append(out, j)
		}
	}
	return out, nil
}

func (f *fakeAnalysis) Metrics(_ context.Context, ids []string) (map[string][]domain.Metric, error) {
	v := 12.5
	out := map[string][]domain.Metric{}
	for _, id := range ids {
		out[id] = []domain.Metric{{Key: "distance_cm", Unit: "cm", Value: &v}}
	}
	return out, nil
}

// fakeStore panics on calls a test did not expect through its embedded nil Store.
type fakeStore struct {
	Store
	candidates      []domain.Candidate
	candidate       domain.Candidate
	askedCalibrated []string
	existing        map[string]bool
	links           []domain.Link
	existingLinks   []domain.Link
	unopened        []domain.Link
	locked          string
	results         map[string]domain.SubjectResult
	blinded         bool
	experiment      string
	filters         []ReviewFilter
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	// Decisions apply only when the whole transaction succeeds.
	saved := maps.Clone(f.results)
	if err := fn(f); err != nil {
		f.results = saved
		return err
	}
	return nil
}

func (f *fakeStore) ReadyTestSubjects(_ context.Context, calibrated []string) ([]domain.Candidate, error) {
	f.askedCalibrated = calibrated
	return f.candidates, nil
}

func (f *fakeStore) Candidate(context.Context, string) (domain.Candidate, error) {
	return f.candidate, nil
}

func (f *fakeStore) LockTestSubject(_ context.Context, id string) error {
	f.locked = id
	return nil
}

func (f *fakeStore) CreateLink(_ context.Context, l domain.Link) (bool, error) {
	if f.existing[l.TestSubjectID] {
		return false, nil
	}
	f.links = append(f.links, l)
	return true, nil
}

func (f *fakeStore) Link(_ context.Context, jobID string) (domain.Link, error) {
	for _, l := range f.existingLinks {
		if l.JobID == jobID {
			return l, nil
		}
	}
	return domain.Link{}, ErrRunNotFound
}

func (f *fakeStore) Links(_ context.Context, subjects []string) ([]domain.Link, error) {
	var out []domain.Link
	for _, l := range f.existingLinks {
		if slices.Contains(subjects, l.TestSubjectID) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeStore) Unopened(context.Context, int) ([]domain.Link, error) { return f.unopened, nil }

func (f *fakeStore) OpenResult(_ context.Context, r domain.SubjectResult) error {
	if f.results == nil {
		f.results = map[string]domain.SubjectResult{}
	}
	if _, ok := f.results[r.TestSubjectID]; !ok {
		f.results[r.TestSubjectID] = r
	}
	return nil
}

func (f *fakeStore) LockResult(_ context.Context, id string) (domain.SubjectResult, string, error) {
	res, ok := f.results[id]
	if !ok {
		return domain.SubjectResult{}, "", ErrResultNotFound
	}
	return res, f.experiment, nil
}

func (f *fakeStore) UpdateResult(_ context.Context, res domain.SubjectResult) (domain.SubjectResult, error) {
	f.results[res.TestSubjectID] = res
	return res, nil
}

func (f *fakeStore) ReviewItems(_ context.Context, filter ReviewFilter) ([]ReviewItem, int, error) {
	f.filters = append(f.filters, filter)
	var out []ReviewItem
	for _, id := range slices.Sorted(maps.Keys(f.results)) {
		if filter.TestSubjectID != "" && id != filter.TestSubjectID {
			continue
		}
		res := f.results[id]
		out = append(out, ReviewItem{Result: res, Blinded: f.blinded, SubjectCode: "M-" + id[len(id)-2:], Run: ReviewRun{ID: res.JobID},
			Group: &ReviewGroup{ID: "g1", Code: "G1", Role: "TREATMENT", SubstanceName: "Compound X"}})
	}
	return out, len(out), nil
}
