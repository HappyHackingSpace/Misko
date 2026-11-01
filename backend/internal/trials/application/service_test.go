package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
)

var (
	ctx        = context.Background()
	now        = time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	technician = access.Actor{UserID: "tech", Role: access.Technician}
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

type fixture struct {
	store        *fakeStore
	analysis     *fakeAnalysis
	videos       *fakeVideos
	calibrations *fakeCalibrations
}

func newFixture() *fixture {
	return &fixture{store: &fakeStore{trials: map[string]domain.Trial{}}, analysis: &fakeAnalysis{}, videos: &fakeVideos{status: domain.VideoVerified},
		calibrations: &fakeCalibrations{id: "cal-1"}}
}

func (f *fixture) service() *Service {
	return New(Dependencies{Store: f.store, Analysis: f.analysis, Videos: f.videos, Calibrations: f.calibrations, Environments: fakeEnvironments{},
		Catalog: fakeCatalog{}}, func() time.Time { return now })
}

func TestOperationsRequireTheirPermission(t *testing.T) {
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"create", access.TrialRun, func(s *Service, a access.Actor) error { return second(s.Create(ctx, a, CreateInput{Name: "x"})) }},
		{"list", access.Read, func(s *Service, a access.Actor) error { return second(s.List(ctx, a, ListQuery{})) }},
		{"get", access.Read, func(s *Service, a access.Actor) error { return second(s.Get(ctx, a, "t")) }},
		{"upload", access.TrialRun, func(s *Service, a access.Actor) error {
			return second(s.StartUpload(ctx, a, "t", domain.UploadRequest{}))
		}},
		{"finalize", access.TrialRun, func(s *Service, a access.Actor) error { return second(s.FinalizeUpload(ctx, a, "t")) }},
		{"video url", access.Read, func(s *Service, a access.Actor) error { _, _, err := s.VideoURL(ctx, a, "t"); return err }},
		{"analyse", access.TrialRun, func(s *Service, a access.Actor) error { return second(s.Analyse(ctx, a, "t", nil)) }},
		{"delete", access.TrialRun, func(s *Service, a access.Actor) error { return s.Delete(ctx, a, "t") }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error {
				return op.run(New(Dependencies{Store: &fakeStore{}, Environments: fakeEnvironments{}}, time.Now), access.Actor{UserID: "u", Role: role})
			})
			if role.Allows(op.permission) {
				if errors.Is(err, access.ErrForbidden) {
					t.Errorf("%s %s: denied", role, op.name)
				}
				continue
			}
			if !errors.Is(err, access.ErrForbidden) {
				t.Errorf("%s %s: err=%v want forbidden before any call", role, op.name, err)
			}
		}
	}
}

func TestTrialsRunTheirArenasParadigm(t *testing.T) {
	for name, tc := range map[string]struct {
		in   CreateInput
		want error
	}{
		"other paradigm": {CreateInput{Name: "x", ParadigmKey: "MWM", ParadigmVersion: 1, EnvironmentRevisionID: "rev-nor"}, ErrParadigmMismatch},
		"other version":  {CreateInput{Name: "x", ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 2, EnvironmentRevisionID: "rev-nor"}, ErrParadigmMismatch},
		"unknown arena":  {CreateInput{Name: "x", ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1, EnvironmentRevisionID: "missing"}, ErrRevisionNotFound},
		"no name":        {CreateInput{Name: "", ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1, EnvironmentRevisionID: "rev-nor"}, domain.ErrInvalidName},
	} {
		if _, err := newFixture().service().Create(ctx, technician, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	f := newFixture()
	trial, err := f.service().Create(ctx, technician, CreateInput{Name: " NOR ", ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1, EnvironmentRevisionID: "rev-nor"})
	if err != nil || trial.Name != "NOR" || trial.CreatedBy != "tech" {
		t.Fatalf("create: %+v %v", trial, err)
	}
}

// A verified video is analysed at once when the trial is ready, later by a
// tick when it waits for its calibration, and again on demand with overrides.
func TestTrialVideosAreAnalysedAutomaticallyAndOnDemand(t *testing.T) {
	f := newFixture()
	f.store.trials["t1"] = domain.Trial{ID: "t1", ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1, EnvironmentRevisionID: "rev-nor"}
	s := f.service()
	if _, err := s.FinalizeUpload(ctx, technician, "t1"); !errors.Is(err, ErrNoVideo) {
		t.Fatalf("finalize without a video: %v", err)
	}
	if _, err := s.StartUpload(ctx, technician, "t1", domain.UploadRequest{Clip: domain.Clip{StartUs: 5, EndUs: ptr(5)}}); !errors.Is(err, domain.ErrInvalidClip) {
		t.Fatalf("bad clip: %v", err)
	}
	end := int64(3_000_000)
	upload, err := s.StartUpload(ctx, technician, "t1", domain.UploadRequest{FileName: "a.mp4", Clip: domain.Clip{EndUs: &end}})
	if err != nil || upload.Trial.VideoAssetID != "video-1" || f.videos.folder != "trials/t1" || *upload.Trial.Clip.EndUs != end {
		t.Fatalf("upload: %+v %v", upload, err)
	}

	// Waiting for a calibration: finalize verifies and submits nothing.
	f.analysis.refuse = true
	detail, err := s.FinalizeUpload(ctx, technician, "t1")
	if err != nil || detail.Video == nil || len(f.store.links) != 0 {
		t.Fatalf("finalize while waiting: %+v %v", detail, err)
	}
	report, err := s.Tick(ctx)
	if err != nil || report.Submitted != 0 || len(report.Skipped) != 0 {
		t.Fatalf("tick while waiting: %+v %v", report, err)
	}
	f.analysis.refuse = false
	if report, err = s.Tick(ctx); err != nil || report.Submitted != 1 {
		t.Fatalf("tick once ready: %+v %v", report, err)
	}
	link, req := f.store.links[0], f.analysis.submitted[len(f.analysis.submitted)-1]
	if link.Trigger != domain.Automatic || link.CalibrationID != "cal-1" || req.IdempotencyKey != "trial:t1:video:video-1" || req.Parameters["arena_width_cm"] != 40 ||
		*req.Clip.EndUs != end {
		t.Fatalf("automatic job: %+v %+v", link, req)
	}

	if _, err := s.Analyse(ctx, technician, "t1", map[string]float64{"arena_width_cm": 60}); !errors.Is(err, domain.ErrInvalidOverrides) {
		t.Fatalf("apparatus override: %v", err)
	}
	analysis, err := s.Analyse(ctx, technician, "t1", map[string]float64{"min_bout_s": 1})
	req = f.analysis.submitted[len(f.analysis.submitted)-1]
	if err != nil || analysis.Trigger != domain.Manual || req.IdempotencyKey != "" || req.Parameters["min_bout_s"] != 1 || req.Parameters["arena_width_cm"] != 40 ||
		analysis.Overrides["min_bout_s"] != 1 || analysis.CreatedBy != "tech" {
		t.Fatalf("manual analysis: %+v %+v %v", analysis, req, err)
	}
	f.analysis.refuse = true
	if _, err := s.Analyse(ctx, technician, "t1", nil); !errors.Is(err, ErrNotReady) {
		t.Fatalf("refused analysis: %v", err)
	}
}

func TestDeletedTrialsAreHiddenAndFrozen(t *testing.T) {
	f := newFixture()
	f.store.trials["t1"] = domain.Trial{ID: "t1", ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1, EnvironmentRevisionID: "rev-nor", VideoAssetID: "v"}
	s := f.service()
	if err := s.Delete(ctx, technician, "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, technician, "t1"); !errors.Is(err, ErrTrialNotFound) {
		t.Fatalf("get a deleted trial: %v", err)
	}
	if _, err := s.Analyse(ctx, technician, "t1", nil); !errors.Is(err, ErrTrialNotFound) {
		t.Fatalf("analyse a deleted trial: %v", err)
	}
}

func TestListQueriesAreBounded(t *testing.T) {
	s := newFixture().service()
	for _, q := range []ListQuery{{Page: -1}, {PageSize: -1}, {Search: string(make([]byte, 121))}} {
		if _, err := s.List(ctx, technician, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}
	page, err := s.List(ctx, technician, ListQuery{PageSize: 500})
	if err != nil || page.PageSize != 200 || page.Page != 1 {
		t.Fatalf("page: %+v %v", page, err)
	}
}

func ptr(v int64) *int64 { return &v }

type fakeEnvironments struct{}

func (fakeEnvironments) Revision(_ context.Context, id string) (domain.Revision, error) {
	if id != "rev-nor" {
		return domain.Revision{}, ErrRevisionNotFound
	}
	return domain.Revision{ID: id, ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1, Parameters: map[string]float64{"arena_width_cm": 40, "arena_height_cm": 40}}, nil
}

// fakeCatalog accepts min_bout_s only.
type fakeCatalog struct{}

func (fakeCatalog) CheckOverrides(_ string, _ int, _, overrides map[string]float64) error {
	for k := range overrides {
		if k != "min_bout_s" {
			return fmt.Errorf("%w: %s", domain.ErrInvalidOverrides, k)
		}
	}
	return nil
}

type fakeAnalysis struct {
	submitted []domain.JobRequest
	refuse    bool
}

func (f *fakeAnalysis) Submit(_ context.Context, r domain.JobRequest) (domain.Job, error) {
	f.submitted = append(f.submitted, r)
	if f.refuse {
		return domain.Job{}, fmt.Errorf("%w: no calibration", ErrNotReady)
	}
	return domain.Job{ID: fmt.Sprintf("job-%d", len(f.submitted)), Status: "QUEUED", CalibrationID: r.CalibrationID, Parameters: r.Parameters}, nil
}

func (f *fakeAnalysis) Results(context.Context, []string) ([]domain.Result, error) { return nil, nil }

func (f *fakeAnalysis) Latest(context.Context, []string) (map[string]domain.Job, error) {
	return map[string]domain.Job{}, nil
}

type fakeVideos struct {
	folder string
	status string
}

func (f *fakeVideos) StartUpload(_ context.Context, folder string, r domain.UploadRequest, _ string) (domain.Video, domain.SignedUpload, error) {
	f.folder = folder
	return domain.Video{ID: "video-1", FileName: r.FileName, Status: "PENDING"}, domain.SignedUpload{URL: "signed"}, nil
}

func (f *fakeVideos) Finalize(_ context.Context, id string) (domain.Video, error) {
	return domain.Video{ID: id, Status: f.status}, nil
}

func (f *fakeVideos) Video(_ context.Context, id string) (domain.Video, error) {
	return domain.Video{ID: id, Status: f.status}, nil
}

func (f *fakeVideos) ReadURL(context.Context, string) (string, time.Time, error) {
	return "read", now, nil
}

type fakeCalibrations struct{ id string }

func (f *fakeCalibrations) TrialCalibration(context.Context, string) (string, error) {
	return f.id, nil
}

// fakeStore panics on calls a test did not expect through its embedded nil Store.
type fakeStore struct {
	Store
	trials map[string]domain.Trial
	links  []domain.Link
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error { return fn(f) }

func (f *fakeStore) Create(_ context.Context, t domain.Trial) (domain.Trial, error) {
	t.ID = "new"
	return t, nil
}

func (f *fakeStore) Trial(_ context.Context, id string) (domain.Trial, error) {
	t, ok := f.trials[id]
	if !ok || t.DeletedAt != nil {
		return domain.Trial{}, ErrTrialNotFound
	}
	return t, nil
}

func (f *fakeStore) LockTrial(c context.Context, id string) (domain.Trial, error) {
	return f.Trial(c, id)
}

func (f *fakeStore) SetVideo(_ context.Context, id, video string, clip domain.Clip) (domain.Trial, error) {
	t := f.trials[id]
	t.VideoAssetID, t.Clip = video, clip
	f.trials[id] = t
	return t, nil
}

func (f *fakeStore) Delete(_ context.Context, id string, at time.Time) error {
	t := f.trials[id]
	t.DeletedAt = &at
	f.trials[id] = t
	return nil
}

func (f *fakeStore) List(context.Context, ListFilter) ([]ListItem, int, error) { return nil, 0, nil }

// Candidates are the trials with a video and no automatic link of it.
func (f *fakeStore) Candidates(context.Context, int) ([]domain.Trial, error) {
	var out []domain.Trial
	for _, t := range f.trials {
		if t.VideoAssetID != "" && len(f.links) == 0 {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeStore) CreateLink(_ context.Context, l domain.Link) (bool, error) {
	f.links = append(f.links, l)
	return true, nil
}

func (f *fakeStore) Links(context.Context, string) ([]domain.Link, error) { return f.links, nil }
