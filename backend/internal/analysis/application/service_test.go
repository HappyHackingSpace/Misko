package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"slices"
	"testing"
	"time"
)

var (
	ctx      = context.Background()
	now      = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	settings = Settings{Lease: time.Minute, MaxAttempts: 3, UploadTTL: time.Hour, ReadTTL: 15 * time.Minute, MaxOutputBytes: 1 << 30}
	worker   = domain.Worker{ID: "worker-a", Name: "tracker", ModelVersion: "tracker 1.0", Capabilities: []domain.Capability{{ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1}}}
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

func newService(store *fakeStore, objects *fakeObjects) *Service {
	return New(store, objects, fakeCatalog{}, fakeTokens{}, settings, func() time.Time { return now })
}

func TestUserOperationsRequireTheirPermission(t *testing.T) {
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"register worker", access.LabConfigure, func(s *Service, a access.Actor) error {
			return second(s.RegisterWorker(ctx, a, WorkerInput{Name: "w", ModelVersion: "m", Capabilities: worker.Capabilities}))
		}},
		{"list workers", access.LabConfigure, func(s *Service, a access.Actor) error { return second(s.Workers(ctx, a)) }},
		{"disable worker", access.LabConfigure, func(s *Service, a access.Actor) error { return second(s.DisableWorker(ctx, a, "worker-a")) }},
		{"capabilities", access.Read, func(s *Service, a access.Actor) error { return second(s.Capabilities(ctx, a)) }},
		{"reanalyze", access.TestRun, func(s *Service, a access.Actor) error { return second(s.Reanalyze(ctx, a, "test", "rec")) }},
		{"list runs", access.Read, func(s *Service, a access.Actor) error { return second(s.Runs(ctx, a, "test")) }},
		{"run detail", access.Read, func(s *Service, a access.Actor) error { return second(s.RunDetail(ctx, a, "run-1")) }},
		{"video pair", access.Read, func(s *Service, a access.Actor) error { return second(s.VideoPair(ctx, a, "run-1")) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error {
				return op.run(newService(&fakeStore{}, &fakeObjects{}), access.Actor{UserID: "u", Role: role})
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

func TestRegisteredWorkersGetATokenOnce(t *testing.T) {
	store := &fakeStore{}
	manager := access.Actor{UserID: "manager", Role: access.LabManager}
	registered, err := newService(store, &fakeObjects{}).RegisterWorker(ctx, manager, WorkerInput{Name: "tracker", ModelVersion: "tracker 1.0", Capabilities: worker.Capabilities})
	if err != nil || registered.Token != "token-1" || string(store.tokenHash) != "hash:token-1" {
		t.Fatalf("registered: %+v %v", registered, err)
	}
	if _, err := newService(&fakeStore{}, &fakeObjects{}).RegisterWorker(ctx, manager, WorkerInput{Name: "x", ModelVersion: "m",
		Capabilities: []domain.Capability{{ParadigmKey: "MAZE", ParadigmVersion: 1}}}); !errors.Is(err, ErrUnknownCapability) {
		t.Fatalf("capability outside the catalog: %v", err)
	}
	if w, err := newService(&fakeStore{}, &fakeObjects{}).AuthenticateWorker(ctx, "token-1"); err != nil || w.ID != "worker-a" {
		t.Fatalf("authenticate: %+v %v", w, err)
	}
	if _, err := newService(&fakeStore{}, &fakeObjects{}).AuthenticateWorker(ctx, ""); !errors.Is(err, ErrWorkerUnauthenticated) {
		t.Fatalf("empty token: %v", err)
	}
}

// Tick enqueues ready recordings once and returns expired leases to the queue.
func TestTickEnqueuesOnlyReadyRecordings(t *testing.T) {
	expired := running()
	past := now.Add(-time.Second)
	expired.LeaseExpiresAt = &past
	store := &fakeStore{
		candidates: []Candidate{
			{RecordingID: "ready", TestID: "t1", SourceAssetID: "a1", SourceGeneration: 7, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, CalibrationID: "cal-1", VideoVerified: true},
			{RecordingID: "uncalibrated", TestID: "t2", SourceGeneration: 7, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, VideoVerified: true},
			{RecordingID: "observed", TestID: "t3", SourceGeneration: 7, ParadigmKey: "ROTAROD", ParadigmVersion: 1, VideoVerified: true},
			{RecordingID: "no worker", TestID: "t4", SourceGeneration: 7, ParadigmKey: "EPM", ParadigmVersion: 1, CalibrationID: "cal-2", VideoVerified: true},
		},
		capabilities: []domain.Capability{{ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1}, {ParadigmKey: "ROTAROD", ParadigmVersion: 1}},
		expired:      []domain.Run{expired},
	}
	report, err := newService(store, &fakeObjects{}).Tick(ctx)
	if err != nil || report.Enqueued != 2 || report.Requeued != 1 {
		t.Fatalf("tick: %+v %v", report, err)
	}
	var recordings []string
	for _, r := range store.created {
		recordings = append(recordings, r.RecordingID)
		if r.Trigger != domain.Automatic || r.Status != domain.Queued || r.MaxAttempts != 3 || r.MetricEngineVersion != 1 {
			t.Errorf("created run: %+v", r)
		}
	}
	if !slices.Equal(recordings, []string{"ready", "observed"}) {
		t.Fatalf("enqueued: %v", recordings)
	}
	if store.created[0].CalibrationID != "cal-1" || store.created[1].CalibrationID != "" {
		t.Fatalf("pinned calibrations: %+v", store.created)
	}
	if len(store.updated) != 1 || store.updated[0].Status != domain.Queued {
		t.Fatalf("expired lease: %+v", store.updated)
	}
}

func running() domain.Run {
	end := int64(8_000_000)
	run := domain.Run{ID: "run-1", TestID: "test", RecordingID: "rec", ClipEndUs: &end, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1,
		Status: domain.Queued, MaxAttempts: 3, AvailableAt: now.Add(-time.Minute)}
	run, _ = run.Claim("worker-a", now, time.Minute)
	return run
}

func submission(run domain.Run) domain.Submission {
	v := 12.5
	return domain.Submission{
		WorkerID: run.WorkerID, Attempt: run.Attempt, ModelVersion: "tracker 1.0",
		Metrics:   []domain.MetricInput{{Key: "distance_cm", Value: &v}},
		Events:    []domain.EventInput{{Type: "in_center", StartUs: 0, EndUs: 1_000_000, Confidence: 1}},
		Artifacts: []domain.ArtifactInput{{Kind: domain.AnalyzedVideo, ObjectName: run.OutputPrefix() + "overlay.mp4"}},
		Pair:      domain.PairInput{AnalyzedObjectName: run.OutputPrefix() + "overlay.mp4", TimeMappingVersion: "identity-v1"},
	}
}

func TestSubmitPublishesOnlyVerifiedOutputs(t *testing.T) {
	run := running()
	overlay := run.OutputPrefix() + "overlay.mp4"
	upload := OutputUpload{RunID: run.ID, Attempt: 1, ObjectName: overlay, Kind: domain.AnalyzedVideo, ContentType: "video/mp4", SizeBytes: 2048, CRC32C: 5}
	good := &fakeObjects{attrs: map[string]ObjectAttrs{overlay: {Generation: 11, Size: 2048, CRC32C: 5, ContentType: "video/mp4"}}}

	for name, tc := range map[string]struct {
		objects *fakeObjects
		uploads []OutputUpload
		mutate  func(*domain.Submission)
		want    error
	}{
		"video output missing":   {&fakeObjects{}, []OutputUpload{upload}, func(*domain.Submission) {}, ErrOutputNotVerified},
		"video output corrupt":   {&fakeObjects{attrs: map[string]ObjectAttrs{overlay: {Generation: 11, Size: 2048, CRC32C: 6, ContentType: "video/mp4"}}}, []OutputUpload{upload}, func(*domain.Submission) {}, ErrOutputNotVerified},
		"output never requested": {good, nil, func(*domain.Submission) {}, ErrUnknownOutput},
		"late attempt":           {good, []OutputUpload{upload}, func(s *domain.Submission) { s.Attempt = 2 }, domain.ErrStaleAttempt},
		"invalid metric":         {good, []OutputUpload{upload}, func(s *domain.Submission) { s.Metrics = nil }, domain.ErrMissingMetric},
	} {
		store := &fakeStore{run: run, uploads: tc.uploads}
		s := submission(run)
		tc.mutate(&s)
		if _, err := newService(store, tc.objects).Submit(ctx, worker, run.ID, s); !errors.Is(err, tc.want) || store.published {
			t.Errorf("%s: err=%v published=%v want %v", name, err, store.published, tc.want)
		}
	}

	store := &fakeStore{run: run, uploads: []OutputUpload{upload}}
	done, err := newService(store, good).Submit(ctx, worker, run.ID, submission(run))
	if err != nil || done.Status != domain.Succeeded || done.ModelVersion != "tracker 1.0" || !store.published || store.publishedOutputs[0].Generation != 11 {
		t.Fatalf("publish: %+v %v %+v", done, err, store.publishedOutputs)
	}

	// The same attempt delivered again returns the stored run without publishing twice.
	again := &fakeStore{run: done, uploads: []OutputUpload{upload}}
	repeated, err := newService(again, good).Submit(ctx, worker, run.ID, submission(run))
	if err != nil || repeated.Status != domain.Succeeded || again.published {
		t.Fatalf("duplicate delivery: %+v %v published=%v", repeated, err, again.published)
	}
}

type fakeCatalog struct{}

func (fakeCatalog) Contract(key string, version int) (domain.Contract, error) {
	if (key != "OPEN_FIELD" && key != "ROTAROD" && key != "EPM") || version != 1 {
		return domain.Contract{}, ErrUnknownCapability
	}
	return domain.Contract{
		MetricEngineVersion: 1, ResultSchemaVersion: 1, CalibrationRequired: key != "ROTAROD",
		Metrics: []domain.MetricSpec{{Key: "distance_cm", Unit: "cm", Max: 1e6}},
		Events:  []domain.EventSpec{{Type: "in_center", Kind: "INTERVAL"}},
	}, nil
}

type fakeTokens struct{}

func (fakeTokens) New() (string, []byte, error) { return "token-1", []byte("hash:token-1"), nil }
func (fakeTokens) Hash(token string) []byte     { return []byte("hash:" + token) }

type fakeObjects struct {
	attrs map[string]ObjectAttrs
}

func (f *fakeObjects) Bucket() string { return "misko-videos" }

func (f *fakeObjects) UploadURL(_ context.Context, object, contentType string, expires time.Time) (SignedRequest, error) {
	return SignedRequest{Method: "POST", URL: "signed:" + object, ExpiresAt: expires}, nil
}

func (f *fakeObjects) ReadURL(_ context.Context, object string, generation int64, _ time.Time) (string, error) {
	return "read:" + object, nil
}

func (f *fakeObjects) Attrs(_ context.Context, object string) (ObjectAttrs, error) {
	if a, ok := f.attrs[object]; ok {
		return a, nil
	}
	return ObjectAttrs{}, ErrObjectNotFound
}

type fakeStore struct {
	Store
	tokenHash        []byte
	candidates       []Candidate
	capabilities     []domain.Capability
	expired          []domain.Run
	created          []domain.Run
	updated          []domain.Run
	run              domain.Run
	uploads          []OutputUpload
	published        bool
	publishedOutputs []VerifiedOutput
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error { return fn(f) }

func (f *fakeStore) CreateWorker(_ context.Context, w domain.Worker, hash []byte) (domain.Worker, error) {
	f.tokenHash = hash
	w.ID = "worker-a"
	return w, nil
}

func (f *fakeStore) WorkerByToken(_ context.Context, hash []byte) (domain.Worker, error) {
	if string(hash) != "hash:token-1" {
		return domain.Worker{}, ErrWorkerUnauthenticated
	}
	return worker, nil
}

func (f *fakeStore) ActiveCapabilities(context.Context) ([]domain.Capability, error) {
	return f.capabilities, nil
}

func (f *fakeStore) ReadyRecordings(context.Context) ([]Candidate, error) { return f.candidates, nil }

func (f *fakeStore) CreateAutomaticRun(_ context.Context, r domain.Run) (bool, error) {
	f.created = append(f.created, r)
	return true, nil
}

func (f *fakeStore) LockExpiredRuns(context.Context, time.Time) ([]domain.Run, error) {
	return f.expired, nil
}

func (f *fakeStore) UpdateRun(_ context.Context, r domain.Run) (domain.Run, error) {
	f.updated = append(f.updated, r)
	return r, nil
}

func (f *fakeStore) Run(context.Context, string) (domain.Run, error)     { return f.run, nil }
func (f *fakeStore) LockRun(context.Context, string) (domain.Run, error) { return f.run, nil }

func (f *fakeStore) TrialIDs(context.Context, string) ([]string, error) { return nil, nil }

func (f *fakeStore) OutputUploads(context.Context, string, int) ([]OutputUpload, error) {
	return f.uploads, nil
}

func (f *fakeStore) Publish(_ context.Context, _ domain.Run, _ domain.Result, outputs []VerifiedOutput) error {
	f.published, f.publishedOutputs = true, outputs
	return nil
}

func second[T any](_ T, err error) error { return err }
