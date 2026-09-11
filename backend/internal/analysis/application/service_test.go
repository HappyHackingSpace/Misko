package application

import (
	"context"
	"encoding/json"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"slices"
	"strings"
	"testing"
	"time"
)

var (
	ctx      = context.Background()
	now      = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	settings = Settings{Lease: time.Minute, MaxAttempts: 3, UploadTTL: time.Hour, ReadTTL: 15 * time.Minute, MaxOutputBytes: 1 << 30, MaxTrajectoryBytes: 1 << 20}
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
	return New(store, objects, &fakeCatalog{}, fakeTokens{}, settings, func() time.Time { return now })
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
	for name, capability := range map[string]domain.Capability{
		"outside the catalog":              {ParadigmKey: "MAZE", ParadigmVersion: 1},
		"not computable from a trajectory": {ParadigmKey: "ROTAROD", ParadigmVersion: 1},
	} {
		store := &fakeStore{}
		_, err := newService(store, &fakeObjects{}).RegisterWorker(ctx, manager, WorkerInput{Name: "x", ModelVersion: "m", Capabilities: []domain.Capability{capability}})
		if !errors.Is(err, ErrUnknownCapability) || store.tokenHash != nil {
			t.Errorf("%s: %v", name, err)
		}
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

func TestClaimCarriesThePinnedCalibration(t *testing.T) {
	queued := domain.Run{ID: "run-1", TestID: "test", ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, CalibrationID: "cal-1", Status: domain.Queued, MaxAttempts: 3, AvailableAt: now}
	store := &fakeStore{next: &queued}
	job, err := newService(store, &fakeObjects{}).Claim(ctx, worker)
	if err != nil || job.Run.Attempt != 1 || job.Calibration == nil || job.Calibration.ID != "cal-1" || job.Calibration.Transform[8] != 1 || len(job.Contract.QC) == 0 {
		t.Fatalf("job: %+v %v", job, err)
	}
	queued.CalibrationID = ""
	job, err = newService(&fakeStore{next: &queued}, &fakeObjects{}).Claim(ctx, worker)
	if err != nil || job.Calibration != nil {
		t.Fatalf("paradigm without calibration: %+v %v", job, err)
	}
	if _, err := New(&fakeStore{next: &queued}, nil, &fakeCatalog{}, fakeTokens{}, settings, time.Now).Claim(ctx, worker); !errors.Is(err, ErrStorageNotConfigured) {
		t.Fatalf("claim without storage: %v", err)
	}
}

func running() domain.Run {
	end := int64(8_000_000)
	run := domain.Run{ID: "run-1", TestID: "test", RecordingID: "rec", ClipEndUs: &end, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1,
		Parameters: map[string]float64{"arena_width_cm": 50}, Status: domain.Queued, MaxAttempts: 3, AvailableAt: now.Add(-time.Minute)}
	run, _ = run.Claim("worker-a", now, time.Minute)
	return run
}

func submission(run domain.Run) domain.Submission {
	return domain.Submission{
		WorkerID: run.WorkerID, Attempt: run.Attempt, ModelVersion: "tracker 1.0",
		Artifacts: []domain.ArtifactInput{
			{Kind: domain.AnalyzedVideo, ObjectName: run.OutputPrefix() + "overlay.mp4"},
			{Kind: domain.Trajectory, ObjectName: run.OutputPrefix() + "trajectory.json"},
		},
		Pair: domain.PairInput{AnalyzedObjectName: run.OutputPrefix() + "overlay.mp4", TimeMappingVersion: "identity-v1"},
	}
}

// trajectory has four samples; tracked marks which of them were found.
func trajectory(t *testing.T, run domain.Run, tracked ...bool) []byte {
	t.Helper()
	xs, ys, confidence := []any{}, []any{}, []any{}
	for _, ok := range tracked {
		if ok {
			xs, ys, confidence = append(xs, 10.0), append(ys, 10.0), append(confidence, 0.5)
		} else {
			xs, ys, confidence = append(xs, nil), append(ys, nil), append(confidence, 0.0)
		}
	}
	data, err := json.Marshal(map[string]any{
		"schema": domain.TrajectorySchema, "runId": run.ID, "attempt": run.Attempt, "durationUs": 8_000_000,
		"samples": map[string]any{"tUs": []int{0, 1_000_000, 2_000_000, 3_000_000}, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSubmitComputesMetricsFromTheVerifiedTrajectory(t *testing.T) {
	run := running()
	overlay, path := run.OutputPrefix()+"overlay.mp4", run.OutputPrefix()+"trajectory.json"
	good := trajectory(t, run, true, true, true, false)
	video := OutputUpload{RunID: run.ID, Attempt: 1, ObjectName: overlay, Kind: domain.AnalyzedVideo, ContentType: "video/mp4", SizeBytes: 2048, CRC32C: 5}
	track := OutputUpload{RunID: run.ID, Attempt: 1, ObjectName: path, Kind: domain.Trajectory, ContentType: "application/json", SizeBytes: int64(len(good)), CRC32C: 9}
	uploads := []OutputUpload{video, track}
	stored := func(trajectoryBytes []byte, trajectoryCRC uint32) *fakeObjects {
		return &fakeObjects{
			attrs: map[string]ObjectAttrs{
				overlay: {Generation: 11, Size: 2048, CRC32C: 5, ContentType: "video/mp4"},
				path:    {Generation: 12, Size: int64(len(trajectoryBytes)), CRC32C: trajectoryCRC, ContentType: "application/json"},
			},
			data: map[string][]byte{path: trajectoryBytes},
		}
	}

	// An upload declared before trajectories had to be JSON is stored and verified, but not read.
	parquet := track
	parquet.ContentType = "application/vnd.apache.parquet"
	parquetObjects := stored(good, 9)
	parquetObjects.attrs[path] = ObjectAttrs{Generation: 12, Size: int64(len(good)), CRC32C: 9, ContentType: parquet.ContentType}
	oversized := track
	oversized.SizeBytes = settings.MaxTrajectoryBytes + 1
	otherRun := running()
	otherRun.ID = "run-2"
	for name, tc := range map[string]struct {
		objects *fakeObjects
		uploads []OutputUpload
		mutate  func(*domain.Submission)
		want    error
	}{
		"video output missing":      {&fakeObjects{}, uploads, func(*domain.Submission) {}, ErrOutputNotVerified},
		"trajectory corrupt":        {stored(good, 10), uploads, func(*domain.Submission) {}, ErrOutputNotVerified},
		"output never requested":    {stored(good, 9), uploads[:1], func(*domain.Submission) {}, ErrUnknownOutput},
		"late attempt":              {stored(good, 9), uploads, func(s *domain.Submission) { s.Attempt = 2 }, domain.ErrStaleAttempt},
		"no trajectory artifact":    {stored(good, 9), uploads, func(s *domain.Submission) { s.Artifacts = s.Artifacts[:1] }, domain.ErrMissingTrajectory},
		"trajectory not JSON":       {parquetObjects, []OutputUpload{video, parquet}, func(*domain.Submission) {}, domain.ErrMissingTrajectory},
		"trajectory too large":      {stored(good, 9), []OutputUpload{video, oversized}, func(*domain.Submission) {}, ErrOutputNotVerified},
		"trajectory of another run": {stored(trajectory(t, otherRun, true, true, true, false), 9), uploads, func(*domain.Submission) {}, domain.ErrInvalidTrajectory},
	} {
		store := &fakeStore{run: run, uploads: tc.uploads}
		s := submission(run)
		tc.mutate(&s)
		if _, err := newService(store, tc.objects).Submit(ctx, worker, run.ID, s); !errors.Is(err, tc.want) || store.published {
			t.Errorf("%s: err=%v published=%v want %v", name, err, store.published, tc.want)
		}
	}

	// A stored trajectory larger than the read limit is refused before it is read.
	big := stored(good, 9)
	big.attrs[path] = ObjectAttrs{Generation: 12, Size: settings.MaxTrajectoryBytes + 1, CRC32C: 9, ContentType: "application/json"}
	bigStore := &fakeStore{run: run, uploads: []OutputUpload{video, oversized}}
	if _, err := newService(bigStore, big).Submit(ctx, worker, run.ID, submission(run)); !errors.Is(err, domain.ErrInvalidTrajectory) || big.reads != 0 {
		t.Fatalf("oversized trajectory: %v reads=%d", err, big.reads)
	}

	catalog := &fakeCatalog{}
	store := &fakeStore{run: run, uploads: uploads}
	objects := stored(good, 9)
	done, err := New(store, objects, catalog, fakeTokens{}, settings, func() time.Time { return now }).Submit(ctx, worker, run.ID, submission(run))
	if err != nil || done.Status != domain.Succeeded || done.ModelVersion != "tracker 1.0" || !store.published || store.publishedOutputs[0].Generation != 11 {
		t.Fatalf("publish: %+v %v %+v", done, err, store.publishedOutputs)
	}
	// The engine received the pinned parameters, the recording duration and the stored samples.
	if catalog.duration != 8_000_000 || catalog.parameters["arena_width_cm"] != 50 || len(catalog.samples) != 4 || objects.lastGeneration != 12 {
		t.Fatalf("engine input: %+v generation %d", catalog, objects.lastGeneration)
	}
	r := store.publishedResult
	if len(r.Metrics) != 1 || r.Metrics[0].Key != "distance_cm" || *r.Metrics[0].Value != 3 ||
		len(r.Events) != 1 || r.Events[0] != (domain.Event{Type: "in_center", Kind: "INTERVAL", StartUs: 0, EndUs: 3_000_000, Confidence: 0.5}) {
		t.Fatalf("published result: %+v", r)
	}

	// The same attempt delivered again returns the stored run without publishing twice.
	again := &fakeStore{run: done, uploads: uploads}
	repeated, err := newService(again, stored(good, 9)).Submit(ctx, worker, run.ID, submission(run))
	if err != nil || repeated.Status != domain.Succeeded || again.published {
		t.Fatalf("duplicate delivery: %+v %v published=%v", repeated, err, again.published)
	}
}

func TestTrajectoriesFailingQualityFailTheRun(t *testing.T) {
	run := running()
	overlay, path := run.OutputPrefix()+"overlay.mp4", run.OutputPrefix()+"trajectory.json"
	lost := trajectory(t, run, true, false, false, false)
	objects := &fakeObjects{
		attrs: map[string]ObjectAttrs{
			overlay: {Generation: 11, Size: 2048, CRC32C: 5, ContentType: "video/mp4"},
			path:    {Generation: 12, Size: int64(len(lost)), CRC32C: 9, ContentType: "application/json"},
		},
		data: map[string][]byte{path: lost},
	}
	store := &fakeStore{run: run, uploads: []OutputUpload{
		{RunID: run.ID, Attempt: 1, ObjectName: overlay, Kind: domain.AnalyzedVideo, ContentType: "video/mp4", SizeBytes: 2048, CRC32C: 5},
		{RunID: run.ID, Attempt: 1, ObjectName: path, Kind: domain.Trajectory, ContentType: "application/json", SizeBytes: int64(len(lost)), CRC32C: 9},
	}}
	catalog := &fakeCatalog{}
	failed, err := New(store, objects, catalog, fakeTokens{}, settings, func() time.Time { return now }).Submit(ctx, worker, run.ID, submission(run))
	if err != nil || failed.Status != domain.Failed || !strings.HasPrefix(failed.FailureReason, "QC_FAILED: max_lost_frame_ratio 0.75") || store.published || catalog.samples != nil {
		t.Fatalf("QC failure: %+v %v published=%v", failed, err, store.published)
	}
	if len(store.updated) != 1 || store.updated[0].Status != domain.Failed || store.updated[0].FinishedAt == nil {
		t.Fatalf("stored failure: %+v", store.updated)
	}
}

type fakeCatalog struct {
	parameters map[string]float64
	samples    []domain.Sample
	duration   int64
}

func (*fakeCatalog) Contract(key string, version int) (domain.Contract, error) {
	if (key != "OPEN_FIELD" && key != "ROTAROD" && key != "EPM") || version != 1 {
		return domain.Contract{}, ErrUnknownCapability
	}
	return domain.Contract{
		MetricEngineVersion: 1, ResultSchemaVersion: 1, CalibrationRequired: key != "ROTAROD", TrajectoryAnalyzable: key != "ROTAROD",
		Metrics: []domain.MetricSpec{{Key: "distance_cm", Unit: "cm", Max: 1e6}},
		Events:  []domain.EventSpec{{Type: "in_center", Kind: "INTERVAL"}},
		QC:      []domain.QCRule{{Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.5}},
	}, nil
}

// Evaluate reports the tracked sample count as the distance and one interval over the trajectory.
func (f *fakeCatalog) Evaluate(_ string, _ int, parameters map[string]float64, t domain.Track, durationUs int64) (domain.Computed, error) {
	f.parameters, f.samples, f.duration = parameters, t.Samples, durationUs
	tracked := 0.0
	for _, s := range t.Samples {
		if s.Tracked {
			tracked++
		}
	}
	return domain.Computed{
		Metrics:   []domain.MetricValue{{Key: "distance_cm", Unit: "cm", Value: &tracked}},
		Intervals: []domain.Interval{{Type: "in_center", StartUs: 0, EndUs: t.Samples[len(t.Samples)-1].TUs}},
	}, nil
}

type fakeTokens struct{}

func (fakeTokens) New() (string, []byte, error) { return "token-1", []byte("hash:token-1"), nil }
func (fakeTokens) Hash(token string) []byte     { return []byte("hash:" + token) }

type fakeObjects struct {
	attrs          map[string]ObjectAttrs
	data           map[string][]byte
	reads          int
	lastGeneration int64
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

func (f *fakeObjects) Read(_ context.Context, object string, generation, limit int64) ([]byte, error) {
	f.reads++
	f.lastGeneration = generation
	data, ok := f.data[object]
	if !ok || f.attrs[object].Generation != generation {
		return nil, ErrObjectNotFound
	}
	if int64(len(data)) > limit {
		return nil, errors.New("too large")
	}
	return data, nil
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
	next             *domain.Run
	uploads          []OutputUpload
	published        bool
	publishedOutputs []VerifiedOutput
	publishedResult  domain.Result
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

func (f *fakeStore) ClaimNext(context.Context, []domain.Capability, time.Time) (domain.Run, error) {
	if f.next == nil {
		return domain.Run{}, ErrNoRun
	}
	return *f.next, nil
}

func (f *fakeStore) Calibration(_ context.Context, id string) (domain.CalibrationRef, error) {
	return domain.CalibrationRef{ID: id, FrameWidth: 1920, FrameHeight: 1080, CropWidth: 1920, CropHeight: 1080, Plane: "ARENA_FLOOR",
		Transform: [9]float64{0.1, 0, 0, 0, 0.1, 0, 0, 0, 1}}, nil
}

func (f *fakeStore) UpdateRun(_ context.Context, r domain.Run) (domain.Run, error) {
	f.updated = append(f.updated, r)
	return r, nil
}

func (f *fakeStore) Run(context.Context, string) (domain.Run, error)     { return f.run, nil }
func (f *fakeStore) LockRun(context.Context, string) (domain.Run, error) { return f.run, nil }

func (f *fakeStore) OutputUploads(context.Context, string, int) ([]OutputUpload, error) {
	return f.uploads, nil
}

func (f *fakeStore) Publish(_ context.Context, _ domain.Run, result domain.Result, outputs []VerifiedOutput) error {
	f.published, f.publishedOutputs, f.publishedResult = true, outputs, result
	return nil
}

func second[T any](_ T, err error) error { return err }
