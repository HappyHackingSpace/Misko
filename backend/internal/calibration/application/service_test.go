package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"slices"
	"testing"
	"time"
)

var (
	ctx   = context.Background()
	now   = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock = func() time.Time { return now }
	truth = domain.Homography{0.03, 0.002, -5, -0.001, 0.045, -3, 0.00001, 0.00002, 1}
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

func point(px, py float64) domain.Correspondence {
	x, y, _ := truth.Apply(px, py)
	return domain.Correspondence{PixelX: px, PixelY: py, WorldX: x, WorldY: y}
}

func input() domain.Input {
	return domain.Input{
		CameraID: "cam-top-1", FrameWidth: 1920, FrameHeight: 1080, Crop: domain.Rect{Width: 1920, Height: 1080}, Plane: domain.ArenaFloor,
		Fit:   []domain.Correspondence{point(200, 150), point(1700, 120), point(1750, 1000), point(250, 980)},
		Check: []domain.Correspondence{point(900, 500), point(400, 700), point(1400, 300)},
	}
}

func TestEveryOperationRequiresItsPermission(t *testing.T) {
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"calibrate", access.TestRun, func(s *Service, a access.Actor) error { return second(s.Calibrate(ctx, a, "test", "subject", input())) }},
		{"calibrate environment", access.ApparatusWrite, func(s *Service, a access.Actor) error {
			return second(s.CalibrateEnvironment(ctx, a, "env", 1, input()))
		}},
		{"list overrides", access.Read, func(s *Service, a access.Actor) error { return second(s.Overrides(ctx, a, "test", "subject")) }},
		{"list environment", access.Read, func(s *Service, a access.Actor) error { return second(s.EnvironmentCalibrations(ctx, a, "env", 1)) }},
		{"subject status", access.Read, func(s *Service, a access.Actor) error { return second(s.SubjectStatus(ctx, a, "test", "subject")) }},
		{"test status", access.Read, func(s *Service, a access.Actor) error { return second(s.TestStatus(ctx, a, "test")) }},
		{"environment status", access.Read, func(s *Service, a access.Actor) error { return second(s.EnvironmentStatus(ctx, a, "env", 1)) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error {
				return op.run(New(&fakeStore{}, fakeCatalog{}, clock), access.Actor{UserID: "u", Role: role})
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

func TestCalibrateOverridesTheArenaOnTheCurrentVideo(t *testing.T) {
	technician := access.Actor{UserID: "tech", Role: access.Technician}
	store := &fakeStore{}
	c, err := New(store, fakeCatalog{}, clock).Calibrate(ctx, technician, "test", "subject", input())
	if err != nil || c.Status != domain.Valid || c.TestSubjectID != "subject" || c.RecordingID != "rec" || c.EnvironmentRevisionID != "rev" ||
		c.CreatedBy != "tech" || c.Scope() != domain.ScopeOverride {
		t.Fatalf("calibration: %+v %v", c, err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockTestSubject", "CurrentVideo", "Overrides", "CreateCalibration"}) {
		t.Fatalf("store calls: %v", store.calls)
	}

	previous := func() domain.Calibration {
		p, err := domain.New(input(), domain.Requirement{Required: true, ToleranceCm: 2}, "tech", now)
		if err != nil {
			t.Fatal(err)
		}
		p.ID, p.TestSubjectID, p.RecordingID = "c1", "subject", "rec"
		return p
	}()
	onOldVideo := previous
	onOldVideo.RecordingID = "old"
	for name, tc := range map[string]struct {
		store  *fakeStore
		mutate func(*domain.Input)
		want   error
	}{
		"no video yet":                    {&fakeStore{video: "NONE"}, func(*domain.Input) {}, ErrNoVideo},
		"unverified video":                {&fakeStore{video: "PENDING"}, func(*domain.Input) {}, ErrVideoNotVerified},
		"paradigm without calibration":    {&fakeStore{paradigm: "POLE"}, func(*domain.Input) {}, domain.ErrNotRequired},
		"animal of another test":          {&fakeStore{missing: true}, func(*domain.Input) {}, ErrTestSubjectNotFound},
		"second first calibration":        {&fakeStore{chain: []domain.Calibration{previous}}, func(*domain.Input) {}, domain.ErrStaleCorrection},
		"a new video continues the chain": {&fakeStore{chain: []domain.Calibration{onOldVideo}}, func(*domain.Input) {}, domain.ErrStaleCorrection},
		"correction from another camera": {&fakeStore{chain: []domain.Calibration{previous}}, func(in *domain.Input) {
			in.SupersedesID, in.CameraID = "c1", "cam-side-2"
		}, domain.ErrCameraMismatch},
		"degenerate points": {&fakeStore{}, func(in *domain.Input) { in.Fit[3] = in.Fit[0] }, domain.ErrDegenerateFit},
	} {
		in := input()
		tc.mutate(&in)
		if _, err := New(tc.store, fakeCatalog{}, clock).Calibrate(ctx, technician, "test", "subject", in); !errors.Is(err, tc.want) || slices.Contains(tc.store.calls, "CreateCalibration") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, tc.store.calls, tc.want)
		}
	}
	corrected := input()
	corrected.SupersedesID = "c1"
	chained := &fakeStore{chain: []domain.Calibration{previous}}
	if c, err := New(chained, fakeCatalog{}, clock).Calibrate(ctx, technician, "test", "subject", corrected); err != nil || c.SupersedesID != "c1" {
		t.Fatalf("correction: %+v %v", c, err)
	}
	// A replacement video may come from another camera; the chain continues.
	otherCamera := input()
	otherCamera.SupersedesID, otherCamera.CameraID = "c1", "cam-side-2"
	if c, err := New(&fakeStore{chain: []domain.Calibration{onOldVideo}}, fakeCatalog{}, clock).Calibrate(ctx, technician, "test", "subject", otherCamera); err != nil || c.RecordingID != "rec" {
		t.Fatalf("override on a replacement video: %+v %v", c, err)
	}
}

// Analysis waits until the animal's effective calibration on a verified video is valid.
func TestSubjectStatusFollowsTheEffectiveCalibration(t *testing.T) {
	viewer := access.Actor{UserID: "viewer", Role: access.Viewer}
	valid, _ := domain.New(input(), domain.Requirement{Required: true, ToleranceCm: 2}, "tech", now)
	valid.ID = "c1"
	withZones := valid
	withZones.Zones = []domain.Zone{{Key: "lanes"}}
	for name, tc := range map[string]struct {
		store   *fakeStore
		status  CalibrationStatus
		source  domain.Source
		current string
	}{
		"nothing calibrated":        {&fakeStore{}, Waiting, "", ""},
		"arena default":             {&fakeStore{source: domain.SourceEnvironment, calibrationID: "c1", stored: valid}, Calibrated, domain.SourceEnvironment, "c1"},
		"override":                  {&fakeStore{source: domain.SourceOverride, calibrationID: "c1", stored: valid}, Calibrated, domain.SourceOverride, "c1"},
		"rejected override waits":   {&fakeStore{source: domain.SourceOverride}, Waiting, domain.SourceOverride, ""},
		"no video yet":              {&fakeStore{video: "NONE", source: domain.SourceEnvironment, calibrationID: "c1", stored: valid}, WaitingForVideo, "", ""},
		"unverified video":          {&fakeStore{video: "PENDING", source: domain.SourceEnvironment, calibrationID: "c1", stored: valid}, WaitingForVideo, "", ""},
		"observation-only paradigm": {&fakeStore{paradigm: "POLE"}, NotRequired, "", ""},
		// A calibration stored before its paradigm's zones were drawn cannot be analysed.
		"required zone missing": {&fakeStore{paradigm: "ZONED", source: domain.SourceEnvironment, calibrationID: "c1", stored: valid}, Waiting, domain.SourceEnvironment, ""},
		"zones drawn":           {&fakeStore{paradigm: "ZONED", source: domain.SourceOverride, calibrationID: "c1", stored: withZones}, Calibrated, domain.SourceOverride, "c1"},
	} {
		view, err := New(tc.store, fakeCatalog{}, clock).SubjectStatus(ctx, viewer, "test", "subject")
		got := ""
		if view.Current != nil {
			got = view.Current.ID
		}
		if err != nil || view.Status != tc.status || view.Source != tc.source || got != tc.current || view.TestSubjectID != "subject" {
			t.Errorf("%s: %+v %v", name, view, err)
		}
		if (tc.status == Calibrated) != (view.Drift == DriftUnchecked) {
			t.Errorf("%s: drift %q", name, view.Drift)
		}
	}
	if _, err := New(&fakeStore{missing: true}, fakeCatalog{}, clock).SubjectStatus(ctx, viewer, "test", "subject"); !errors.Is(err, ErrTestSubjectNotFound) {
		t.Fatalf("unknown animal: %v", err)
	}
	list, err := New(&fakeStore{}, fakeCatalog{}, clock).TestStatus(ctx, viewer, "test")
	if err != nil || len(list) != 1 || list[0].RecordingID != "rec" || list[0].EnvironmentRevisionID != "rev" {
		t.Fatalf("test status: %+v %v", list, err)
	}
	if _, err := New(&fakeStore{missing: true}, fakeCatalog{}, clock).TestStatus(ctx, viewer, "test"); !errors.Is(err, ErrTestNotFound) {
		t.Fatalf("unknown test: %v", err)
	}
}

// fakeCatalog answers for MWM (bounded by its pool), POLE (no calibration)
// and ZONED, a hypothetical paradigm with calibrated zones: no published
// paradigm declares any, but the generic support for them is kept.
type fakeCatalog struct{}

func (fakeCatalog) Requirement(key string, _ int, _ map[string]float64) (domain.Requirement, error) {
	if key == "POLE" {
		return domain.Requirement{}, nil
	}
	if key == "ZONED" {
		return domain.Requirement{Required: true, ToleranceCm: 2, Zones: []domain.ZoneRequirement{{Key: "lanes", Required: true}, {Key: "extra", Required: false}}}, nil
	}
	return domain.Requirement{Required: true, ToleranceCm: 2, Bounds: &domain.Bounds{MinX: -10, MinY: -10, MaxX: 100, MaxY: 100}}, nil
}

type fakeStore struct {
	Store
	calls         []string
	video         string
	paradigm      string
	missing       bool
	chain         []domain.Calibration
	envChain      []domain.Calibration
	noRevision    bool
	source        domain.Source
	calibrationID string
	stored        domain.Calibration
}

func (f *fakeStore) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	f.record("Transaction")
	return fn(f)
}

func (f *fakeStore) subject() (TestSubjectRef, error) {
	if f.missing {
		return TestSubjectRef{}, ErrTestSubjectNotFound
	}
	return TestSubjectRef{ID: "subject", TestID: "test", EnvironmentRevisionID: "rev", ParadigmKey: withDefault(f.paradigm, "MWM"), ParadigmVersion: 1}, nil
}

func (f *fakeStore) TestExists(context.Context, string) (bool, error) {
	f.record("TestExists")
	return !f.missing, nil
}

func (f *fakeStore) TestSubject(context.Context, string, string) (TestSubjectRef, error) {
	f.record("TestSubject")
	return f.subject()
}

func (f *fakeStore) LockTestSubject(context.Context, string, string) (TestSubjectRef, error) {
	f.record("LockTestSubject")
	return f.subject()
}

func (f *fakeStore) CurrentVideo(context.Context, string) (VideoRef, error) {
	f.record("CurrentVideo")
	if f.video == "NONE" {
		return VideoRef{}, ErrNoVideo
	}
	return VideoRef{RecordingID: "rec", Status: withDefault(f.video, VideoVerified)}, nil
}

func (f *fakeStore) Overrides(context.Context, string) ([]domain.Calibration, error) {
	f.record("Overrides")
	return f.chain, nil
}

func (f *fakeStore) Calibration(_ context.Context, id string) (domain.Calibration, error) {
	f.record("Calibration")
	if id != f.stored.ID {
		return domain.Calibration{}, errors.New("unknown calibration")
	}
	return f.stored, nil
}

func (f *fakeStore) Effective(context.Context, string) ([]Effective, error) {
	f.record("Effective")
	subject, err := f.subject()
	if err != nil {
		return nil, nil
	}
	row := Effective{Subject: subject, RecordingID: "rec", VideoStatus: withDefault(f.video, VideoVerified), Source: f.source, CalibrationID: f.calibrationID}
	if f.video == "NONE" {
		row.RecordingID, row.VideoStatus, row.Source, row.CalibrationID = "", "", "", ""
	}
	return []Effective{row}, nil
}

func withDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (f *fakeStore) revision() (RevisionRef, error) {
	if f.noRevision {
		return RevisionRef{}, ErrRevisionNotFound
	}
	return RevisionRef{ID: "rev", EnvironmentID: "env", ParadigmKey: withDefault(f.paradigm, "MWM"), ParadigmVersion: 1}, nil
}

func (f *fakeStore) Revision(context.Context, string, int) (RevisionRef, error) {
	f.record("Revision")
	return f.revision()
}

func (f *fakeStore) LockRevision(context.Context, string, int) (RevisionRef, error) {
	f.record("LockRevision")
	return f.revision()
}

func (f *fakeStore) EnvironmentCalibrations(context.Context, string) ([]domain.Calibration, error) {
	f.record("EnvironmentCalibrations")
	return f.envChain, nil
}

func (f *fakeStore) CreateCalibration(_ context.Context, c domain.Calibration) (domain.Calibration, error) {
	f.record("CreateCalibration")
	c.ID = "new"
	return c, nil
}

func second[T any](_ T, err error) error { return err }

func TestCalibrateEnvironmentStoresTheDefaultOfARevision(t *testing.T) {
	admin := access.Actor{UserID: "lab", Role: access.LabManager}
	store := &fakeStore{}
	in := input()
	in.ReferenceFrameUs = 5_000_000
	c, err := New(store, fakeCatalog{}, clock).CalibrateEnvironment(ctx, admin, "env", 1, in)
	if err != nil || c.Status != domain.Valid || c.RecordingID != "" || c.TestSubjectID != "" || c.EnvironmentRevisionID != "rev" || c.ReferenceFrameUs != 0 || c.Scope() != domain.ScopeEnvironment {
		t.Fatalf("environment calibration: %+v %v", c, err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockRevision", "EnvironmentCalibrations", "CreateCalibration"}) {
		t.Fatalf("store calls: %v", store.calls)
	}

	previous := c
	previous.ID = "e1"
	for name, tc := range map[string]struct {
		store  *fakeStore
		mutate func(*domain.Input)
		want   error
	}{
		"unknown revision":             {&fakeStore{noRevision: true}, func(*domain.Input) {}, ErrRevisionNotFound},
		"paradigm without calibration": {&fakeStore{paradigm: "POLE"}, func(*domain.Input) {}, domain.ErrNotRequired},
		"second first calibration":     {&fakeStore{envChain: []domain.Calibration{previous}}, func(*domain.Input) {}, domain.ErrStaleCorrection},
		"correction from another camera": {&fakeStore{envChain: []domain.Calibration{previous}}, func(in *domain.Input) {
			in.SupersedesID, in.CameraID = "e1", "cam-side-2"
		}, domain.ErrCameraMismatch},
	} {
		in := input()
		tc.mutate(&in)
		if _, err := New(tc.store, fakeCatalog{}, clock).CalibrateEnvironment(ctx, admin, "env", 1, in); !errors.Is(err, tc.want) || slices.Contains(tc.store.calls, "CreateCalibration") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, tc.store.calls, tc.want)
		}
	}
	corrected := input()
	corrected.SupersedesID = "e1"
	chained := &fakeStore{envChain: []domain.Calibration{previous}}
	if c, err := New(chained, fakeCatalog{}, clock).CalibrateEnvironment(ctx, admin, "env", 1, corrected); err != nil || c.SupersedesID != "e1" {
		t.Fatalf("correction: %+v %v", c, err)
	}
}

func TestEnvironmentStatusUsesTheLatestDefault(t *testing.T) {
	viewer := access.Actor{UserID: "viewer", Role: access.Viewer}
	env, _ := domain.New(input(), domain.Requirement{Required: true, ToleranceCm: 2}, "lab", now)
	env.ID, env.EnvironmentRevisionID = "e1", "rev"
	rejected := env
	rejected.ID, rejected.SupersedesID, rejected.Status, rejected.RejectionReason = "e2", "e1", domain.Rejected, domain.RejectedCheckError

	view, err := New(&fakeStore{envChain: []domain.Calibration{env}}, fakeCatalog{}, clock).EnvironmentStatus(ctx, viewer, "env", 1)
	if err != nil || view.Status != Calibrated || view.Current.ID != "e1" || view.Source != domain.SourceEnvironment || view.Drift != DriftUnchecked {
		t.Fatalf("environment status: %+v %v", view, err)
	}
	view, err = New(&fakeStore{envChain: []domain.Calibration{env, rejected}}, fakeCatalog{}, clock).EnvironmentStatus(ctx, viewer, "env", 1)
	if err != nil || view.Status != Waiting || view.Source != domain.SourceEnvironment || view.Current != nil {
		t.Fatalf("rejected correction: %+v %v", view, err)
	}
	view, err = New(&fakeStore{}, fakeCatalog{}, clock).EnvironmentStatus(ctx, viewer, "env", 1)
	if err != nil || view.Status != Waiting || view.Source != "" || view.Drift != "" {
		t.Fatalf("uncalibrated: %+v %v", view, err)
	}
	view, err = New(&fakeStore{paradigm: "ZONED", envChain: []domain.Calibration{env}}, fakeCatalog{}, clock).EnvironmentStatus(ctx, viewer, "env", 1)
	if err != nil || view.Status != Waiting || view.Source != domain.SourceEnvironment || view.Current != nil {
		t.Fatalf("environment without its zones: %+v %v", view, err)
	}
	view, err = New(&fakeStore{paradigm: "POLE"}, fakeCatalog{}, clock).EnvironmentStatus(ctx, viewer, "env", 1)
	if err != nil || view.Status != NotRequired {
		t.Fatalf("observation-only environment: %+v %v", view, err)
	}
}
