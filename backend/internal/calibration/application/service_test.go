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
		{"calibrate", access.TestRun, func(s *Service, a access.Actor) error { return second(s.Calibrate(ctx, a, "test", "rec", input())) }},
		{"list", access.Read, func(s *Service, a access.Actor) error { return second(s.Calibrations(ctx, a, "test", "rec")) }},
		{"status", access.Read, func(s *Service, a access.Actor) error { return second(s.Status(ctx, a, "test", "rec")) }},
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

func TestCalibrateChecksTheRecordingAndChain(t *testing.T) {
	technician := access.Actor{UserID: "tech", Role: access.Technician}
	store := &fakeStore{}
	c, err := New(store, fakeCatalog{}, clock).Calibrate(ctx, technician, "test", "rec", input())
	if err != nil || c.Status != domain.Valid || c.RecordingID != "rec" || c.CreatedBy != "tech" {
		t.Fatalf("calibration: %+v %v", c, err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockRecording", "Calibrations", "CreateCalibration"}) {
		t.Fatalf("store calls: %v", store.calls)
	}

	previous := func() domain.Calibration {
		p, err := domain.New(input(), domain.Requirement{Required: true, ToleranceCm: 2}, "tech", now)
		if err != nil {
			t.Fatal(err)
		}
		p.ID, p.RecordingID = "c1", "rec"
		return p
	}()
	for name, tc := range map[string]struct {
		store  *fakeStore
		mutate func(*domain.Input)
		want   error
	}{
		"unverified video":             {&fakeStore{video: "PENDING"}, func(*domain.Input) {}, ErrVideoNotVerified},
		"paradigm without calibration": {&fakeStore{paradigm: "ROTAROD"}, func(*domain.Input) {}, domain.ErrNotRequired},
		"recording of another test":    {&fakeStore{missing: true}, func(*domain.Input) {}, ErrRecordingNotFound},
		"second first calibration":     {&fakeStore{chain: []domain.Calibration{previous}}, func(*domain.Input) {}, domain.ErrStaleCorrection},
		"correction from another camera": {&fakeStore{chain: []domain.Calibration{previous}}, func(in *domain.Input) {
			in.SupersedesID, in.CameraID = "c1", "cam-side-2"
		}, domain.ErrCameraMismatch},
		"degenerate points": {&fakeStore{}, func(in *domain.Input) { in.Fit[3] = in.Fit[0] }, domain.ErrDegenerateFit},
	} {
		in := input()
		tc.mutate(&in)
		if _, err := New(tc.store, fakeCatalog{}, clock).Calibrate(ctx, technician, "test", "rec", in); !errors.Is(err, tc.want) || slices.Contains(tc.store.calls, "CreateCalibration") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, tc.store.calls, tc.want)
		}
	}
	corrected := input()
	corrected.SupersedesID = "c1"
	chained := &fakeStore{chain: []domain.Calibration{previous}}
	if c, err := New(chained, fakeCatalog{}, clock).Calibrate(ctx, technician, "test", "rec", corrected); err != nil || c.SupersedesID != "c1" {
		t.Fatalf("correction: %+v %v", c, err)
	}
}

// Analysis waits until the latest calibration of a video that needs one is valid.
func TestStatusShowsWaitingForCalibration(t *testing.T) {
	viewer := access.Actor{UserID: "viewer", Role: access.Viewer}
	valid, _ := domain.New(input(), domain.Requirement{Required: true, ToleranceCm: 2}, "tech", now)
	valid.ID = "c1"
	rejected := valid
	rejected.ID, rejected.SupersedesID, rejected.Status, rejected.RejectionReason = "c2", "c1", domain.Rejected, domain.RejectedCheckError
	for name, tc := range map[string]struct {
		store   *fakeStore
		status  CalibrationStatus
		current string
	}{
		"no calibration yet":         {&fakeStore{}, Waiting, ""},
		"valid calibration":          {&fakeStore{chain: []domain.Calibration{valid}}, Calibrated, "c1"},
		"latest correction rejected": {&fakeStore{chain: []domain.Calibration{valid, rejected}}, Waiting, ""},
		"unverified video":           {&fakeStore{video: "PENDING"}, Waiting, ""},
		"observation-only paradigm":  {&fakeStore{paradigm: "ROTAROD"}, NotRequired, ""},
	} {
		view, err := New(tc.store, fakeCatalog{}, clock).Status(ctx, viewer, "test", "rec")
		got := ""
		if view.Current != nil {
			got = view.Current.ID
		}
		if err != nil || view.Status != tc.status || got != tc.current {
			t.Errorf("%s: %+v %v", name, view, err)
		}
	}
}

type fakeCatalog struct{}

func (fakeCatalog) Requirement(key string, _ int, _ map[string]float64) (domain.Requirement, error) {
	if key == "ROTAROD" {
		return domain.Requirement{}, nil
	}
	return domain.Requirement{Required: true, ToleranceCm: 2, Bounds: &domain.Bounds{MinX: -10, MinY: -10, MaxX: 100, MaxY: 100}}, nil
}

type fakeStore struct {
	Store
	calls    []string
	video    string
	paradigm string
	missing  bool
	chain    []domain.Calibration
}

func (f *fakeStore) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	f.record("Transaction")
	return fn(f)
}

func (f *fakeStore) ref() (RecordingRef, error) {
	if f.missing {
		return RecordingRef{}, ErrRecordingNotFound
	}
	video, paradigm := f.video, f.paradigm
	if video == "" {
		video = "VERIFIED"
	}
	if paradigm == "" {
		paradigm = "OPEN_FIELD"
	}
	return RecordingRef{ID: "rec", TestID: "test", VideoStatus: video, ParadigmKey: paradigm, ParadigmVersion: 1}, nil
}

func (f *fakeStore) Recording(context.Context, string, string) (RecordingRef, error) {
	f.record("Recording")
	return f.ref()
}

func (f *fakeStore) LockRecording(context.Context, string, string) (RecordingRef, error) {
	f.record("LockRecording")
	return f.ref()
}

func (f *fakeStore) Calibrations(context.Context, string) ([]domain.Calibration, error) {
	f.record("Calibrations")
	return f.chain, nil
}

func (f *fakeStore) CreateCalibration(_ context.Context, c domain.Calibration) (domain.Calibration, error) {
	f.record("CreateCalibration")
	c.ID = "new"
	return c, nil
}

func second[T any](_ T, err error) error { return err }
