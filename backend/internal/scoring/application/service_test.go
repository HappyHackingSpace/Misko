package application

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/domain"
)

var (
	ctx        = context.Background()
	now        = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	technician = access.Actor{UserID: "tech", Role: access.Technician}
	viewer     = access.Actor{UserID: "view", Role: access.Viewer}
)

func us(v int64) *int64 { return &v }

func scorable() Subject {
	return Subject{
		ExperimentID: "e", TestID: "t", TestSubjectID: "ts", ExperimentStatus: "RUNNING", TestStatus: "COMPLETED",
		ParadigmKey: "ROTAROD", ParadigmVersion: 1, Parameters: map[string]float64{"max_trial_duration_s": 60},
		Video: &domain.Video{Clip: domain.Clip{RecordingID: "rec", StartUs: 1_000_000, EndUs: us(61_000_000)}, AssetID: "asset", Generation: 7, CRC32C: 9},
	}
}

func TestOperationsRequireTheirPermission(t *testing.T) {
	s := New(&fakeStore{subject: scorable()}, fakeCatalog{}, func() time.Time { return now })
	for name, call := range map[string]func(access.Actor) error{
		"read": func(a access.Actor) error { _, err := s.Read(ctx, a, "t", "ts"); return err },
		"add": func(a access.Actor) error {
			_, err := s.AddEvent(ctx, a, "t", "ts", domain.EventInput{Type: "fall", StartUs: 1})
			return err
		},
		"change": func(a access.Actor) error {
			_, err := s.ChangeEvent(ctx, a, "t", "ts", "o1", domain.EventInput{Type: "fall", StartUs: 1})
			return err
		},
		"remove": func(a access.Actor) error { return s.RemoveEvent(ctx, a, "t", "ts", "o1") },
		"complete": func(a access.Actor) error {
			_, err := s.Complete(ctx, a, "t", "ts", domain.CompleteInput{})
			return err
		},
		"reopen": func(a access.Actor) error { _, err := s.Reopen(ctx, a, "t", "ts"); return err },
	} {
		if err := call(access.Actor{}); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("%s anonymous: %v", name, err)
		}
		if name == "read" {
			continue
		}
		if err := call(viewer); !errors.Is(err, access.ErrForbidden) {
			t.Errorf("%s as viewer: %v", name, err)
		}
	}
}

func TestOnlyScorableAnimalsCanBeScored(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*Subject)
		want   error
	}{
		"tracked paradigm":     {func(s *Subject) { s.ParadigmKey = "OPEN_FIELD" }, domain.ErrNotObserverScored},
		"planned test":         {func(s *Subject) { s.TestStatus = "PLANNED" }, ErrNotScorable},
		"cancelled test":       {func(s *Subject) { s.TestStatus = "CANCELLED" }, ErrNotScorable},
		"completed experiment": {func(s *Subject) { s.ExperimentStatus = "COMPLETED" }, ErrNotScorable},
		"excluded animal":      {func(s *Subject) { s.Excluded = true }, ErrNotScorable},
		"no video":             {func(s *Subject) { s.Video = nil }, ErrNoVideo},
	} {
		t.Run(name, func(t *testing.T) {
			subject := scorable()
			tc.mutate(&subject)
			store := &fakeStore{subject: subject}
			s := New(store, fakeCatalog{}, func() time.Time { return now })
			if _, err := s.AddEvent(ctx, technician, "t", "ts", domain.EventInput{Type: "fall", StartUs: 1}); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if len(store.sheets) != 0 {
				t.Fatalf("a sheet was started: %+v", store.sheets)
			}
		})
	}
}

func TestScoringCompletesIntoAScoredRunAndReopensAsANewVersion(t *testing.T) {
	store := &fakeStore{subject: scorable()}
	catalog := &recordingCatalog{}
	s := New(store, catalog, func() time.Time { return now })

	view, err := s.Read(ctx, viewer, "t", "ts")
	if err != nil || view.Sheet != nil || len(view.Events) != 0 || len(view.Paradigm.Events) != 1 {
		t.Fatalf("empty view: %+v %v", view, err)
	}
	fall, err := s.AddEvent(ctx, technician, "t", "ts", domain.EventInput{Type: "fall", StartUs: 30_000_000})
	if err != nil || fall.SheetID != "sheet-1" || fall.ScoredBy != "tech" || len(store.sheets) != 1 || store.sheets[0].Version != 1 {
		t.Fatalf("first event: %+v %v %+v", fall, err, store.sheets)
	}
	if _, err := s.AddEvent(ctx, technician, "t", "ts", domain.EventInput{Type: "fall", StartUs: 31_000_000}); !errors.Is(err, domain.ErrSingleEvent) {
		t.Fatalf("second fall: %v", err)
	}
	if _, err := s.ChangeEvent(ctx, technician, "t", "ts", "missing", domain.EventInput{Type: "fall", StartUs: 1}); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("change a missing event: %v", err)
	}
	moved, err := s.ChangeEvent(ctx, technician, "t", "ts", fall.ID, domain.EventInput{Type: "fall", StartUs: 20_000_000})
	if err != nil || moved.StartUs != 20_000_000 || moved.ID != fall.ID {
		t.Fatalf("change the fall: %+v %v", moved, err)
	}

	done, err := s.Complete(ctx, technician, "t", "ts", domain.CompleteInput{})
	if err != nil || done.RunID != "run-1" || done.Sheet.Status != domain.Completed || done.Sheet.RecordingID != "rec" || *done.Sheet.DurationUs != 60_000_000 {
		t.Fatalf("complete: %+v %v", done, err)
	}
	if !slices.Equal(catalog.scoring.ScoredTypes, []string{"fall"}) || catalog.scoring.Events[0].StartUs != 20_000_000 || catalog.scoring.DurationUs != 60_000_000 {
		t.Fatalf("engine input: %+v", catalog.scoring)
	}
	run := store.runs[0]
	if run.ScoredBy != "tech" || run.Video.AssetID != "asset" || run.ParadigmKey != "ROTAROD" || run.Parameters["max_trial_duration_s"] != 60 || len(run.Computed.Metrics) != 1 {
		t.Fatalf("run: %+v", run)
	}
	if !slices.Equal(store.opened, []string{"ts/run-1"}) {
		t.Fatalf("opened results: %v", store.opened)
	}
	if _, err := s.AddEvent(ctx, technician, "t", "ts", domain.EventInput{Type: "fall", StartUs: 1}); !errors.Is(err, domain.ErrNotDraft) {
		t.Fatalf("add after completion: %v", err)
	}
	if _, err := s.Complete(ctx, technician, "t", "ts", domain.CompleteInput{}); !errors.Is(err, domain.ErrNotDraft) {
		t.Fatalf("complete twice: %v", err)
	}

	reopened, err := s.Reopen(ctx, technician, "t", "ts")
	if err != nil || reopened.Sheet.Version != 2 || reopened.Sheet.Status != domain.Draft || len(reopened.Events) != 1 || reopened.Events[0].StartUs != 20_000_000 {
		t.Fatalf("reopen: %+v %v", reopened, err)
	}
	if _, err := s.Reopen(ctx, technician, "t", "ts"); !errors.Is(err, domain.ErrNotCompleted) {
		t.Fatalf("reopen a draft: %v", err)
	}
	if err := s.RemoveEvent(ctx, technician, "t", "ts", reopened.Events[0].ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := s.Complete(ctx, technician, "t", "ts", domain.CompleteInput{DurationUs: us(70_000_000)}); !errors.Is(err, domain.ErrInvalidDuration) {
		t.Fatalf("length beyond the clip: %v", err)
	}
	catalog.reject = true
	if _, err := s.Complete(ctx, technician, "t", "ts", domain.CompleteInput{}); !errors.Is(err, ErrRejectedByEngine) {
		t.Fatalf("engine rejection: %v", err)
	}
}

func TestCompletingWithoutEventsStartsAndCompletesTheFirstSheet(t *testing.T) {
	store := &fakeStore{subject: scorable()}
	s := New(store, &recordingCatalog{}, func() time.Time { return now })
	done, err := s.Complete(ctx, technician, "t", "ts", domain.CompleteInput{})
	if err != nil || done.Sheet.Version != 1 || done.Sheet.Status != domain.Completed || len(done.Events) != 0 {
		t.Fatalf("complete without events: %+v %v", done, err)
	}
	if reopened, err := s.Reopen(ctx, technician, "t", "ts"); err != nil || reopened.Sheet.Version != 2 || len(reopened.Events) != 0 {
		t.Fatalf("reopen an empty sheet: %+v %v", reopened, err)
	}
}

type fakeCatalog struct{}

func (fakeCatalog) Paradigm(key string, version int) (domain.Paradigm, error) {
	if key == "OPEN_FIELD" {
		return domain.Paradigm{Key: key, Version: version}, nil
	}
	return domain.Paradigm{Key: key, Version: version, ObserverScored: true, Events: []domain.EventDefinition{{Type: "fall", Kind: domain.Point, Single: true}}}, nil
}

func (fakeCatalog) Evaluate(domain.Paradigm, map[string]float64, domain.Scoring) (domain.Computed, error) {
	return domain.Computed{}, nil
}

type recordingCatalog struct {
	fakeCatalog
	scoring domain.Scoring
	reject  bool
}

func (c *recordingCatalog) Evaluate(_ domain.Paradigm, _ map[string]float64, s domain.Scoring) (domain.Computed, error) {
	if c.reject {
		return domain.Computed{}, ErrRejectedByEngine
	}
	c.scoring = s
	v := 20.0
	return domain.Computed{MetricEngineVersion: 1, ResultSchemaVersion: 1, Metrics: []domain.MetricValue{{Key: "latency_to_fall_s", Unit: "s", Value: &v}}}, nil
}

type fakeStore struct {
	subject      Subject
	sheets       []domain.Sheet
	observations []domain.Observation
	runs         []domain.Run
	opened       []string
	ids          int
}

func (f *fakeStore) id(prefix string) string {
	f.ids++
	return prefix + "-" + strconv.Itoa(f.ids)
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	sheets, observations, runs, opened := slices.Clone(f.sheets), slices.Clone(f.observations), slices.Clone(f.runs), slices.Clone(f.opened)
	if err := fn(f); err != nil {
		f.sheets, f.observations, f.runs, f.opened = sheets, observations, runs, opened
		return err
	}
	return nil
}

func (f *fakeStore) LockSubject(context.Context, string) error { return nil }

func (f *fakeStore) Subject(context.Context, string, string) (Subject, error) { return f.subject, nil }

func (f *fakeStore) LatestSheet(context.Context, string) (*domain.Sheet, error) {
	if len(f.sheets) == 0 {
		return nil, nil
	}
	s := f.sheets[len(f.sheets)-1]
	return &s, nil
}

func (f *fakeStore) CreateSheet(_ context.Context, s domain.Sheet) (domain.Sheet, error) {
	s.ID = "sheet-" + strconv.Itoa(len(f.sheets)+1)
	f.sheets = append(f.sheets, s)
	return s, nil
}

func (f *fakeStore) CompleteSheet(_ context.Context, s domain.Sheet) (domain.Sheet, error) {
	f.sheets[len(f.sheets)-1] = s
	return s, nil
}

func (f *fakeStore) Observations(_ context.Context, sheetID string) ([]domain.Observation, error) {
	out := []domain.Observation{}
	for _, o := range f.observations {
		if o.SheetID == sheetID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeStore) CreateObservation(_ context.Context, o domain.Observation) (domain.Observation, error) {
	o.ID = f.id("obs")
	f.observations = append(f.observations, o)
	return o, nil
}

func (f *fakeStore) UpdateObservation(_ context.Context, o domain.Observation) (domain.Observation, error) {
	for i := range f.observations {
		if f.observations[i].ID == o.ID {
			f.observations[i] = o
			return o, nil
		}
	}
	return domain.Observation{}, ErrEventNotFound
}

func (f *fakeStore) DeleteObservation(_ context.Context, _, id string) error {
	f.observations = slices.DeleteFunc(f.observations, func(o domain.Observation) bool { return o.ID == id })
	return nil
}

func (f *fakeStore) CopyObservations(_ context.Context, from, to domain.Sheet) error {
	for _, o := range f.observations {
		if o.SheetID == from.ID {
			o.ID, o.SheetID = f.id("obs"), to.ID
			f.observations = append(f.observations, o)
		}
	}
	return nil
}

func (f *fakeStore) CreateRun(_ context.Context, r domain.Run) (string, error) {
	r.ID = "run-" + strconv.Itoa(len(f.runs)+1)
	f.runs = append(f.runs, r)
	return r.ID, nil
}

func (f *fakeStore) OpenResult(_ context.Context, testSubjectID, runID string) error {
	f.opened = append(f.opened, testSubjectID+"/"+runID)
	return nil
}
