package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

var (
	ctx      = context.Background()
	now      = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	settings = Settings{Lease: time.Minute, MaxAttempts: 3, UploadTTL: time.Hour, ReadTTL: 15 * time.Minute, MaxOutputBytes: 1 << 30, MaxTrajectoryBytes: 1 << 20}
	worker   = domain.Worker{ID: "worker-a", Name: "tracker", ModelVersion: "tracker 1.0", Capabilities: []domain.Capability{{ParadigmKey: "MWM", ParadigmVersion: 1}}}
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

type fixture struct {
	store        *fakeStore
	objects      *fakeObjects
	catalog      *fakeCatalog
	sources      *fakeSources
	calibrations *fakeCalibrations
	listener     *fakeListener
}

func newFixture() *fixture {
	return &fixture{store: &fakeStore{}, objects: &fakeObjects{}, catalog: &fakeCatalog{}, sources: &fakeSources{},
		calibrations: &fakeCalibrations{}, listener: &fakeListener{}}
}

func (f *fixture) service() *Service {
	return New(Dependencies{Store: f.store, Objects: f.objects, Sources: f.sources, Calibrations: f.calibrations, Catalog: f.catalog,
		Tokens: fakeTokens{}, Listener: f.listener}, settings, func() time.Time { return now })
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
		{"job detail", access.Read, func(s *Service, a access.Actor) error { return second(s.JobDetail(ctx, a, "job-1")) }},
		{"video pair", access.Read, func(s *Service, a access.Actor) error { return second(s.VideoPair(ctx, a, "job-1")) }},
		{"output url", access.Read, func(s *Service, a access.Actor) error { return second(s.OutputURL(ctx, a, "job-1", domain.Trajectory)) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error {
				return op.run(New(Dependencies{Store: &fakeStore{}, Objects: &fakeObjects{}, Catalog: &fakeCatalog{}, Tokens: fakeTokens{}}, settings, time.Now),
					access.Actor{UserID: "u", Role: role})
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
	f := newFixture()
	manager := access.Actor{UserID: "manager", Role: access.LabManager}
	registered, err := f.service().RegisterWorker(ctx, manager, WorkerInput{Name: "tracker", ModelVersion: "tracker 1.0", Capabilities: worker.Capabilities})
	if err != nil || registered.Token != "token-1" || string(f.store.tokenHash) != "hash:token-1" {
		t.Fatalf("registered: %+v %v", registered, err)
	}
	for name, capability := range map[string]domain.Capability{
		"outside the catalog":              {ParadigmKey: "MAZE", ParadigmVersion: 1},
		"not computable from a trajectory": {ParadigmKey: "UNTRACKED", ParadigmVersion: 1},
	} {
		f := newFixture()
		_, err := f.service().RegisterWorker(ctx, manager, WorkerInput{Name: "x", ModelVersion: "m", Capabilities: []domain.Capability{capability}})
		if !errors.Is(err, ErrUnknownCapability) || f.store.tokenHash != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if w, err := newFixture().service().AuthenticateWorker(ctx, "token-1"); err != nil || w.ID != "worker-a" {
		t.Fatalf("authenticate: %+v %v", w, err)
	}
	if _, err := newFixture().service().AuthenticateWorker(ctx, ""); !errors.Is(err, ErrWorkerUnauthenticated) {
		t.Fatalf("empty token: %v", err)
	}
}

func TestTickReturnsExpiredLeasesToTheQueue(t *testing.T) {
	past := now.Add(-time.Second)
	retry, last := running(), running()
	retry.LeaseExpiresAt, last.LeaseExpiresAt, last.Attempt = &past, &past, 3
	f := newFixture()
	f.store.expired = []domain.Job{retry, last}
	report, err := f.service().Tick(ctx)
	if err != nil || report.Requeued != 1 || report.Failed != 1 {
		t.Fatalf("tick: %+v %v", report, err)
	}
	if len(f.store.updated) != 2 || f.store.updated[0].Status != domain.Queued || f.store.updated[1].FailureReason != domain.ReasonLeaseExpired {
		t.Fatalf("updated: %+v", f.store.updated)
	}
}

func request(key string) domain.Request {
	end := int64(8_000_000)
	return domain.Request{Requester: domain.Requester{Kind: domain.RequesterTrial, ID: "trial-1"}, ParadigmKey: key, ParadigmVersion: 1,
		Parameters: map[string]float64{"tank_diameter_cm": 120}, SourceAssetID: "asset-1", ClipEndUs: &end, CalibrationID: "cal-1",
		Trigger: domain.Manual, IdempotencyKey: "trial-1:asset-1", CreatedBy: "user-1"}
}

// Submit snapshots what a requester names: the verified source generation and
// the calibration, which must carry every zone the paradigm requires.
func TestSubmitSnapshotsTheSourceAndTheCalibration(t *testing.T) {
	f := newFixture()
	job, err := f.service().Submit(ctx, request("ZONED"))
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != domain.Queued || job.Source.Generation != 42 || job.Source.CRC32C != 7 || job.CalibrationID() != "cal-1" ||
		len(job.Calibration.Zones) != 2 || job.MetricEngineVersion != 1 || job.IdempotencyKey != "trial-1:asset-1" || len(f.store.created) != 1 {
		t.Fatalf("job: %+v", job)
	}
	// A paradigm without calibration pins none, even when one is named.
	f = newFixture()
	if free, err := f.service().Submit(ctx, request("FREE")); err != nil || free.Calibration != nil || f.calibrations.reads != 0 {
		t.Fatalf("paradigm without calibration: %+v %v", free.Calibration, err)
	}

	rejected := errors.New("rejected calibration")
	for name, tc := range map[string]struct {
		req   domain.Request
		setup func(*fixture)
		want  error
	}{
		"invalid request":    {func() domain.Request { r := request("ZONED"); r.Requester.Kind = "EXPERIMENT"; return r }(), nil, domain.ErrInvalidRequest},
		"unknown paradigm":   {request("MAZE"), nil, domain.ErrNotAnalysable},
		"untracked paradigm": {request("UNTRACKED"), nil, domain.ErrNotAnalysable},
		"unknown source":     {request("ZONED"), func(f *fixture) { f.sources.missing = true }, ErrSourceNotFound},
		"unverified source":  {request("ZONED"), func(f *fixture) { f.sources.pending = true }, domain.ErrSourceNotVerified},
		"no calibration":     {func() domain.Request { r := request("ZONED"); r.CalibrationID = ""; return r }(), nil, domain.ErrCalibrationRequired},
		"calibration error":  {request("ZONED"), func(f *fixture) { f.calibrations.err = rejected }, rejected},
		"a zone is missing":  {request("ZONED"), func(f *fixture) { f.calibrations.oneZone = true }, domain.ErrMissingZones},
	} {
		f := newFixture()
		if tc.setup != nil {
			tc.setup(f)
		}
		if _, err := f.service().Submit(ctx, tc.req); !errors.Is(err, tc.want) || len(f.store.created) != 0 {
			t.Errorf("%s: err=%v want %v, created %d", name, err, tc.want, len(f.store.created))
		}
	}
}

func TestPortCallsAreBounded(t *testing.T) {
	ids := make([]string, MaxBatch+1)
	s := newFixture().service()
	if _, err := s.Jobs(ctx, ids); !errors.Is(err, ErrTooManyIDs) {
		t.Fatalf("jobs: %v", err)
	}
	if _, err := s.ByRequester(ctx, domain.RequesterTrial, ids); !errors.Is(err, ErrTooManyIDs) {
		t.Fatalf("by requester: %v", err)
	}
	if _, err := s.ByRequester(ctx, "EXPERIMENT", nil); !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("unknown requester kind: %v", err)
	}
	if jobs, err := s.Results(ctx, nil); err != nil || len(jobs) != 0 {
		t.Fatalf("no ids: %v %v", jobs, err)
	}
}

func TestClaimCarriesTheContract(t *testing.T) {
	queued := running()
	queued.Status, queued.Attempt, queued.WorkerID, queued.LeaseExpiresAt = domain.Queued, 0, "", nil
	f := newFixture()
	f.store.next = &queued
	claimed, err := f.service().Claim(ctx, worker)
	if err != nil || claimed.Job.Attempt != 1 || claimed.Job.Status != domain.Running || len(claimed.Contract.QC) == 0 {
		t.Fatalf("claimed: %+v %v", claimed, err)
	}
	if _, err := newFixture().service().Claim(ctx, worker); !errors.Is(err, ErrNoJob) {
		t.Fatalf("nothing queued: %v", err)
	}
	if _, err := New(Dependencies{Store: f.store, Catalog: f.catalog}, settings, time.Now).Claim(ctx, worker); !errors.Is(err, ErrStorageNotConfigured) {
		t.Fatalf("claim without storage: %v", err)
	}
}

func running() domain.Job {
	end := int64(8_000_000)
	job := domain.Job{ID: "job-1", Input: domain.Input{Requester: domain.Requester{Kind: domain.RequesterTestSubject, ID: "subject-1"},
		ParadigmKey: "MWM", ParadigmVersion: 1, Parameters: map[string]float64{"tank_diameter_cm": 120},
		Source: domain.Source{AssetID: "asset-1", Generation: 42, ClipEndUs: &end}},
		Status: domain.Queued, MaxAttempts: 3, AvailableAt: now.Add(-time.Minute)}
	job, _ = job.Claim("worker-a", now, time.Minute)
	return job
}

func submission(job domain.Job) domain.Submission {
	return domain.Submission{
		WorkerID: job.WorkerID, Attempt: job.Attempt, ModelVersion: "tracker 1.0",
		Artifacts: []domain.ArtifactInput{
			{Kind: domain.AnalyzedVideo, ObjectName: job.OutputPrefix() + "overlay.mp4"},
			{Kind: domain.Trajectory, ObjectName: job.OutputPrefix() + "trajectory.json"},
		},
		Pair: domain.PairInput{AnalyzedObjectName: job.OutputPrefix() + "overlay.mp4", TimeMappingVersion: "identity-v1"},
	}
}

// trajectory has four samples; tracked marks which of them were found.
func trajectory(t *testing.T, job domain.Job, tracked ...bool) []byte {
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
		"schema": domain.TrajectorySchema, "runId": job.ID, "attempt": job.Attempt, "durationUs": 8_000_000,
		"samples": map[string]any{"tUs": []int{0, 1_000_000, 2_000_000, 3_000_000}, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// outputs declares and stores the overlay and the trajectory of a job.
func outputs(job domain.Job, trajectoryBytes []byte, trajectoryCRC uint32) ([]OutputUpload, *fakeObjects) {
	overlay, path := job.OutputPrefix()+"overlay.mp4", job.OutputPrefix()+"trajectory.json"
	uploads := []OutputUpload{
		{JobID: job.ID, Attempt: 1, ObjectName: overlay, Kind: domain.AnalyzedVideo, ContentType: "video/mp4", SizeBytes: 2048, CRC32C: 5},
		{JobID: job.ID, Attempt: 1, ObjectName: path, Kind: domain.Trajectory, ContentType: "application/json", SizeBytes: int64(len(trajectoryBytes)), CRC32C: 9},
	}
	return uploads, &fakeObjects{
		attrs: map[string]ObjectAttrs{
			overlay: {Generation: 11, Size: 2048, CRC32C: 5, ContentType: "video/mp4"},
			path:    {Generation: 12, Size: int64(len(trajectoryBytes)), CRC32C: trajectoryCRC, ContentType: "application/json"},
		},
		data: map[string][]byte{path: trajectoryBytes},
	}
}

func TestSubmitResultRefusesUnverifiedOutputs(t *testing.T) {
	job := running()
	good := trajectory(t, job, true, true, true, false)
	other := job
	other.ID = "job-2"
	for name, tc := range map[string]struct {
		trajectory []byte
		crc        uint32
		uploads    func([]OutputUpload) []OutputUpload
		objects    func(*fakeObjects)
		mutate     func(*domain.Submission)
		want       error
	}{
		"video output missing":      {good, 9, nil, func(o *fakeObjects) { o.attrs = nil }, nil, ErrOutputNotVerified},
		"trajectory corrupt":        {good, 10, func(u []OutputUpload) []OutputUpload { return u }, nil, nil, ErrOutputNotVerified},
		"output never requested":    {good, 9, func(u []OutputUpload) []OutputUpload { return u[:1] }, nil, nil, ErrUnknownOutput},
		"late attempt":              {good, 9, nil, nil, func(s *domain.Submission) { s.Attempt = 2 }, domain.ErrStaleAttempt},
		"no trajectory artifact":    {good, 9, nil, nil, func(s *domain.Submission) { s.Artifacts = s.Artifacts[:1] }, domain.ErrMissingTrajectory},
		"trajectory of another job": {trajectory(t, other, true, true, true, false), 9, nil, nil, nil, domain.ErrInvalidTrajectory},
	} {
		uploads, objects := outputs(job, tc.trajectory, 9)
		if tc.crc != 9 {
			a := objects.attrs[job.OutputPrefix()+"trajectory.json"]
			a.CRC32C = tc.crc
			objects.attrs[job.OutputPrefix()+"trajectory.json"] = a
		}
		if tc.uploads != nil {
			uploads = tc.uploads(uploads)
		}
		if tc.objects != nil {
			tc.objects(objects)
		}
		f := newFixture()
		f.store.job, f.store.uploads, f.objects = job, uploads, objects
		s := submission(job)
		if tc.mutate != nil {
			tc.mutate(&s)
		}
		if _, err := f.service().SubmitResult(ctx, worker, job.ID, s); !errors.Is(err, tc.want) || f.store.published || f.listener.heard != 0 {
			t.Errorf("%s: err=%v published=%v want %v", name, err, f.store.published, tc.want)
		}
	}
}

func TestSubmitResultComputesMetricsFromTheVerifiedTrajectory(t *testing.T) {
	job := running()
	zoned := job
	zoned.Calibration = &domain.Calibration{ID: "cal-1", Zones: []domain.Zone{{Key: "lanes", Centimeters: domain.Shape{Circle: &domain.Circle{Radius: 5}}}}}
	uploads, objects := outputs(job, trajectory(t, job, true, true, true, false), 9)
	f := newFixture()
	f.store.job, f.store.uploads, f.objects = zoned, uploads, objects
	done, err := f.service().SubmitResult(ctx, worker, job.ID, submission(job))
	if err != nil || done.Status != domain.Succeeded || done.ModelVersion != "tracker 1.0" || !f.store.published || f.store.publishedOutputs[0].Generation != 11 {
		t.Fatalf("publish: %+v %v %+v", done, err, f.store.publishedOutputs)
	}
	// The engine received the pinned parameters, the snapshot's zones, the
	// recording duration and the stored samples.
	c := f.catalog
	if c.duration != 8_000_000 || c.parameters["tank_diameter_cm"] != 120 || len(c.samples) != 4 || len(c.zones) != 1 || objects.lastGeneration != 12 {
		t.Fatalf("engine input: %+v generation %d", c, objects.lastGeneration)
	}
	r := f.store.publishedResult
	if len(r.Metrics) != 1 || *r.Metrics[0].Value != 3 || len(r.Events) != 1 || f.store.publishedQuality.Samples != 4 || f.store.publishedQuality.Tracked != 3 {
		t.Fatalf("published: %+v quality %+v", r, f.store.publishedQuality)
	}
	if f.listener.heard != 1 || f.listener.last.ID != job.ID {
		t.Fatalf("listener: %+v", f.listener)
	}

	// The same attempt delivered again publishes nothing and tells the listener again.
	again := newFixture()
	again.store.job, again.store.uploads, again.objects = done, uploads, objects
	if repeated, err := again.service().SubmitResult(ctx, worker, job.ID, submission(job)); err != nil || repeated.Status != domain.Succeeded ||
		again.store.published || again.listener.heard != 1 {
		t.Fatalf("duplicate delivery: %+v %v", repeated, err)
	}
	// A listener failure is reported to the worker, which delivers again.
	failing := newFixture()
	failing.store.job, failing.store.uploads, failing.objects = job, uploads, objects
	failing.listener.err = errors.New("review store down")
	if _, err := failing.service().SubmitResult(ctx, worker, job.ID, submission(job)); err == nil || !failing.store.published {
		t.Fatalf("listener failure: %v", err)
	}
}

func TestTrajectoriesFailingQualityFailTheJob(t *testing.T) {
	job := running()
	uploads, objects := outputs(job, trajectory(t, job, true, false, false, false), 9)
	f := newFixture()
	f.store.job, f.store.uploads, f.objects = job, uploads, objects
	failed, err := f.service().SubmitResult(ctx, worker, job.ID, submission(job))
	if err != nil || failed.Status != domain.Failed || !strings.HasPrefix(failed.FailureReason, "QC_FAILED: max_lost_frame_ratio 0.75") ||
		f.store.published || f.catalog.samples != nil || f.listener.heard != 0 {
		t.Fatalf("QC failure: %+v %v", failed, err)
	}
	if f.store.qc == nil || f.store.qc.Passed || f.store.qc.LostRatio != 0.75 || len(f.store.updated) != 1 || f.store.updated[0].FinishedAt == nil {
		t.Fatalf("stored failure: %+v %+v", f.store.qc, f.store.updated)
	}
}

func TestSignedReadsNeedASucceededJob(t *testing.T) {
	viewer := access.Actor{UserID: "v", Role: access.Viewer}
	f := newFixture()
	f.store.job = running()
	if _, err := f.service().OutputURL(ctx, viewer, "job-1", domain.Trajectory); !errors.Is(err, ErrJobNotSucceeded) {
		t.Fatalf("running job: %v", err)
	}
	done := running()
	done.Status = domain.Succeeded
	f.store.job = done
	f.store.stored = map[string]Published{"job-1": {Artifacts: []VerifiedOutput{{OutputUpload: OutputUpload{ObjectName: "jobs/job-1/attempts/1/t.json", Kind: domain.Trajectory}, Generation: 3}},
		Pair: &Pair{AnalyzedObjectName: "jobs/job-1/attempts/1/o.mp4", AnalyzedGeneration: 4}}}
	link, err := f.service().OutputURL(ctx, viewer, "job-1", domain.Trajectory)
	if err != nil || link.URL != "read:jobs/job-1/attempts/1/t.json" {
		t.Fatalf("trajectory link: %+v %v", link, err)
	}
	if _, err := f.service().OutputURL(ctx, viewer, "job-1", domain.Thumbnail); !errors.Is(err, ErrOutputNotFound) {
		t.Fatalf("no thumbnail: %v", err)
	}
	pair, err := f.service().VideoPair(ctx, viewer, "job-1")
	if err != nil || pair.OriginalURL != "read:videos/asset-1" || pair.AnalyzedURL != "read:jobs/job-1/attempts/1/o.mp4" {
		t.Fatalf("pair: %+v %v", pair, err)
	}
}

// fakeCatalog knows MWM and FREE (tracked, FREE without calibration),
// UNTRACKED (a hypothetical version no trajectory can measure) and ZONED, a
// tracked paradigm with two required calibrated zones.
type fakeCatalog struct {
	parameters map[string]float64
	samples    []domain.Sample
	zones      []domain.Zone
	duration   int64
}

func (*fakeCatalog) Contract(key string, version int) (domain.Contract, error) {
	known := map[string]bool{"MWM": true, "FREE": true, "UNTRACKED": true, "ZONED": true}
	if !known[key] || version != 1 {
		return domain.Contract{}, ErrUnknownCapability
	}
	var zones []string
	if key == "ZONED" {
		zones = []string{"center", "lanes"}
	}
	return domain.Contract{
		MetricEngineVersion: 1, ResultSchemaVersion: 1, CalibrationRequired: key == "MWM" || key == "ZONED", TrajectoryAnalyzable: key != "UNTRACKED",
		RequiredZones: zones,
		Metrics:       []domain.MetricSpec{{Key: "path_length_cm", Unit: "cm", Max: 1e6}},
		Events:        []domain.EventSpec{{Type: "in_platform", Kind: "INTERVAL"}},
		QC:            []domain.QCRule{{Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.5}},
	}, nil
}

// Evaluate reports the tracked sample count as the distance and one interval over the trajectory.
func (f *fakeCatalog) Evaluate(_ string, _ int, parameters map[string]float64, t domain.Track, zones []domain.Zone, durationUs int64) (domain.Computed, error) {
	f.parameters, f.samples, f.zones, f.duration = parameters, t.Samples, zones, durationUs
	tracked := 0.0
	for _, s := range t.Samples {
		if s.Tracked {
			tracked++
		}
	}
	return domain.Computed{
		Metrics:   []domain.MetricValue{{Key: "path_length_cm", Unit: "cm", Value: &tracked}},
		Intervals: []domain.Interval{{Type: "in_platform", StartUs: 0, EndUs: t.Samples[len(t.Samples)-1].TUs}},
	}, nil
}

type fakeSources struct{ missing, pending bool }

func (f *fakeSources) Source(_ context.Context, id string) (domain.SourceVideo, error) {
	if f.missing {
		return domain.SourceVideo{}, ErrSourceNotFound
	}
	return domain.SourceVideo{ID: id, ObjectName: "videos/" + id, Verified: !f.pending, Generation: 42, CRC32C: 7}, nil
}

type fakeCalibrations struct {
	reads   int
	err     error
	oneZone bool
}

func (f *fakeCalibrations) Calibration(_ context.Context, id string) (domain.Calibration, error) {
	f.reads++
	if f.err != nil {
		return domain.Calibration{}, f.err
	}
	c := domain.Calibration{ID: id, FrameWidth: 1920, FrameHeight: 1080, Crop: domain.Rect{Width: 1920, Height: 1080}, Plane: "ARENA_FLOOR",
		Transform: [9]float64{0.1, 0, 0, 0, 0.1, 0, 0, 0, 1},
		Zones:     []domain.Zone{{Key: "lanes"}, {Key: "center"}}}
	if f.oneZone {
		c.Zones = c.Zones[:1]
	}
	return c, nil
}

type fakeListener struct {
	heard int
	last  domain.Job
	err   error
}

func (f *fakeListener) JobSucceeded(_ context.Context, j domain.Job) error {
	f.heard++
	f.last = j
	return f.err
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

func (f *fakeObjects) UploadURL(_ context.Context, object, _ string, expires time.Time) (SignedRequest, error) {
	return SignedRequest{Method: "POST", URL: "signed:" + object, ExpiresAt: expires}, nil
}

func (f *fakeObjects) ReadURL(_ context.Context, object string, _ int64, _ time.Time) (string, error) {
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

// fakeStore panics on calls a test did not expect through its embedded nil Store.
type fakeStore struct {
	Store
	tokenHash        []byte
	expired          []domain.Job
	created          []domain.Job
	updated          []domain.Job
	job              domain.Job
	next             *domain.Job
	uploads          []OutputUpload
	published        bool
	publishedOutputs []VerifiedOutput
	publishedResult  domain.Result
	publishedQuality domain.Quality
	qc               *QC
	stored           map[string]Published
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

func (f *fakeStore) CreateJob(_ context.Context, j domain.Job) (domain.Job, error) {
	j.ID = "job-new"
	f.created = append(f.created, j)
	return j, nil
}

func (f *fakeStore) LockExpiredJobs(context.Context, time.Time) ([]domain.Job, error) {
	return f.expired, nil
}

func (f *fakeStore) ClaimNext(context.Context, []domain.Capability, time.Time) (domain.Job, error) {
	if f.next == nil {
		return domain.Job{}, ErrNoJob
	}
	return *f.next, nil
}

func (f *fakeStore) UpdateJob(_ context.Context, j domain.Job) (domain.Job, error) {
	f.updated = append(f.updated, j)
	return j, nil
}

func (f *fakeStore) Job(context.Context, string) (domain.Job, error)     { return f.job, nil }
func (f *fakeStore) LockJob(context.Context, string) (domain.Job, error) { return f.job, nil }

func (f *fakeStore) Jobs(context.Context, []string) ([]domain.Job, error) {
	return []domain.Job{f.job}, nil
}

func (f *fakeStore) OutputUploads(context.Context, string, int) ([]OutputUpload, error) {
	return f.uploads, nil
}

func (f *fakeStore) Publish(_ context.Context, _ domain.Job, result domain.Result, outputs []VerifiedOutput, quality domain.Quality) error {
	f.published, f.publishedOutputs, f.publishedResult, f.publishedQuality = true, outputs, result, quality
	return nil
}

func (f *fakeStore) RecordQC(_ context.Context, _ domain.Job, qc QC) error {
	f.qc = &qc
	return nil
}

func (f *fakeStore) Published(context.Context, []string) (map[string]Published, error) {
	if f.stored == nil {
		return map[string]Published{}, nil
	}
	return f.stored, nil
}

func second[T any](_ T, err error) error { return err }
