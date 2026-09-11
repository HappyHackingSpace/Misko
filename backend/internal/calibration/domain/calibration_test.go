package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

// truth maps 1920x1080 camera pixels to a 50 x 40 cm arena with a mild
// perspective tilt; points below are generated from it independently of Fit.
var truth = Homography{0.03, 0.002, -5, -0.001, 0.045, -3, 0.00001, 0.00002, 1}

func point(px, py float64) Correspondence {
	x, y, err := truth.Apply(px, py)
	if err != nil {
		panic(err)
	}
	return Correspondence{PixelX: px, PixelY: py, WorldX: x, WorldY: y}
}

func input() Input {
	return Input{
		CameraID: "cam-top-1", FrameWidth: 1920, FrameHeight: 1080, Crop: Rect{X: 100, Y: 50, Width: 1700, Height: 1000},
		ReferenceFrameUs: 2_000_000, Plane: ArenaFloor,
		Fit:   []Correspondence{point(200, 150), point(1700, 120), point(1750, 1000), point(250, 980)},
		Check: []Correspondence{point(900, 500), point(400, 700), point(1400, 300)},
	}
}

func arena() Requirement {
	return Requirement{Required: true, ToleranceCm: 2, Bounds: &Bounds{MinX: -10, MinY: -10, MaxX: 100, MaxY: 100}}
}

func TestFitRecoversAKnownTransform(t *testing.T) {
	c, err := New(input(), arena(), "tech", now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != Valid || c.RejectionReason != "" || c.AlgorithmVersion != AlgorithmVersion || c.CreatedBy != "tech" || c.ToleranceCm != 2 {
		t.Fatalf("calibration: %+v", c)
	}
	for i := range truth {
		if math.Abs(c.Transform[i]-truth[i]) > 1e-9*math.Max(1, math.Abs(truth[i])) {
			t.Fatalf("transform %d: %v want %v", i, c.Transform[i], truth[i])
		}
	}
	if c.FitRMSErrorCm > 1e-9 || c.CheckMaxErrorCm > 1e-9 || c.CheckRMSErrorCm > 1e-9 {
		t.Fatalf("errors: fit %v check max %v rms %v", c.FitRMSErrorCm, c.CheckMaxErrorCm, c.CheckRMSErrorCm)
	}

	// More than four fit points use least squares; noise shows up as fit error.
	noisy := input()
	extra := point(960, 540)
	extra.WorldX += 0.5
	noisy.Fit = append(noisy.Fit, extra)
	c, err = New(noisy, arena(), "tech", now)
	if err != nil || c.Status != Valid || c.FitRMSErrorCm <= 0 || c.FitRMSErrorCm > 0.5 {
		t.Fatalf("least squares: %+v %v", c, err)
	}
}

// A good fit is not enough: independent check points must agree within tolerance.
func TestCheckPointsValidateIndependently(t *testing.T) {
	far := input()
	far.Check[1].WorldX += 3 // 3 cm off with a 2 cm tolerance
	c, err := New(far, arena(), "tech", now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != Rejected || c.RejectionReason != RejectedCheckError || math.Abs(c.CheckMaxErrorCm-3) > 1e-6 || c.FitRMSErrorCm > 1e-9 {
		t.Fatalf("excessive check error: %+v", c)
	}
	just := input()
	just.Check[0].WorldX += 2.5 // between the tolerance and the next centimeter
	if c, err := New(just, arena(), "tech", now); err != nil || c.Status != Rejected {
		t.Fatalf("2.5 cm CHECK error: %+v %v", c, err)
	}
	edge := input()
	edge.Check[1].WorldY += 2 // exactly at the tolerance
	if c, err := New(edge, arena(), "tech", now); err != nil || c.Status != Valid {
		t.Fatalf("error at the tolerance: %+v %v", c, err)
	}
}

func TestInvalidCalibrationsAreRefused(t *testing.T) {
	with := func(mutate func(*Input)) Input { in := input(); mutate(&in); return in }
	for name, tc := range map[string]struct {
		in   Input
		req  Requirement
		want error
	}{
		"three fit points": {with(func(in *Input) { in.Fit = in.Fit[:3] }), arena(), ErrInsufficientFitPoints},
		"two check points": {with(func(in *Input) { in.Check = in.Check[:2] }), arena(), ErrInsufficientCheckPoints},
		"no check points":  {with(func(in *Input) { in.Check = nil }), arena(), ErrInsufficientCheckPoints},
		"collinear fit points": {with(func(in *Input) {
			in.Fit = []Correspondence{point(200, 200), point(500, 400), point(800, 600), point(1100, 800)}
		}), arena(), ErrDegenerateFit},
		"repeated fit points":             {with(func(in *Input) { in.Fit[3] = in.Fit[0] }), arena(), ErrDegenerateFit},
		"check reuses a fit point":        {with(func(in *Input) { in.Check[0] = in.Fit[2] }), arena(), ErrCheckPointReused},
		"pixel outside the crop":          {with(func(in *Input) { in.Check[0] = point(60, 500) }), arena(), ErrPointOutsideCrop},
		"crop outside the frame":          {with(func(in *Input) { in.Crop.Width = 1900 }), arena(), ErrInvalidCrop},
		"empty frame":                     {with(func(in *Input) { in.FrameWidth = 0 }), arena(), ErrInvalidFrame},
		"world point off the environment": {with(func(in *Input) { in.Check[2].WorldX = 200 }), arena(), ErrPointOutsideEnvironment},
		"not a number":                    {with(func(in *Input) { in.Fit[1].WorldY = math.NaN() }), arena(), ErrInvalidPoint},
		"no camera":                       {with(func(in *Input) { in.CameraID = " " }), arena(), ErrInvalidCamera},
		"unknown plane":                   {with(func(in *Input) { in.Plane = "CEILING" }), arena(), ErrInvalidPlane},
		"negative reference":              {with(func(in *Input) { in.ReferenceFrameUs = -1 }), arena(), ErrInvalidReferenceFrame},
		"paradigm without video":          {input(), Requirement{Required: false}, ErrNotRequired},
	} {
		if _, err := New(tc.in, tc.req, "tech", now); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	// A point measured on the wall may sit up to the tolerance outside the bounds;
	// the FIT point (1750, 1000) maps to x = 47.7 cm, 1.7 cm past a 46 cm edge.
	tight := arena()
	tight.Bounds = &Bounds{MinX: 0, MinY: 0, MaxX: 46, MaxY: 40}
	if c, err := New(input(), tight, "tech", now); err != nil || c.Status != Valid {
		t.Fatalf("point within the tolerance of the edge: %+v %v", c, err)
	}
	tight.Bounds.MaxX = 45
	if _, err := New(input(), tight, "tech", now); !errors.Is(err, ErrPointOutsideEnvironment) {
		t.Fatalf("point beyond the tolerance of the edge: %v", err)
	}
	// Without environment bounds any world coordinates are accepted.
	free := arena()
	free.Bounds = nil
	if c, err := New(with(func(in *Input) {
		for i := range in.Fit {
			in.Fit[i].WorldX += 500
		}
		for i := range in.Check {
			in.Check[i].WorldX += 500
		}
	}), free, "tech", now); err != nil || c.Status != Valid {
		t.Fatalf("calibrated-zone frame: %+v %v", c, err)
	}
}

// Corrections form one chain per recording and keep the same camera setup.
func TestCorrectionsExtendTheChain(t *testing.T) {
	first, _ := New(input(), arena(), "tech", now)
	first.ID = "c1"
	rejected := first
	rejected.Status, rejected.RejectionReason = Rejected, RejectedCheckError

	if err := CanCorrect(nil, ""); err != nil {
		t.Fatalf("first calibration: %v", err)
	}
	if err := CanCorrect(nil, "c1"); !errors.Is(err, ErrStaleCorrection) {
		t.Fatalf("correction without a calibration: %v", err)
	}
	chain := []Calibration{first}
	if err := CanCorrect(chain, ""); !errors.Is(err, ErrStaleCorrection) {
		t.Fatalf("second root: %v", err)
	}
	if err := CanCorrect(chain, "c1"); err != nil {
		t.Fatalf("correction of the latest: %v", err)
	}
	second := first
	second.ID, second.SupersedesID = "c2", "c1"
	chain = append(chain, second)
	if err := CanCorrect(chain, "c1"); !errors.Is(err, ErrStaleCorrection) {
		t.Fatalf("correction of a superseded calibration: %v", err)
	}
	if cur := Current(chain); cur == nil || cur.ID != "c2" {
		t.Fatalf("current: %+v", cur)
	}
	// The latest is found by the supersedes links, not by position.
	if latest := Latest([]Calibration{second, first}); latest == nil || latest.ID != "c2" {
		t.Fatalf("latest of an unordered chain: %+v", latest)
	}
	if cur := Current([]Calibration{rejected}); cur != nil {
		t.Fatalf("a rejected latest calibration leaves the video waiting: %+v", cur)
	}

	moved := input()
	moved.CameraID = "cam-side-2"
	if err := SameSetup(first, moved); !errors.Is(err, ErrCameraMismatch) {
		t.Fatalf("other camera: %v", err)
	}
	resized := input()
	resized.FrameWidth = 1280
	if err := SameSetup(first, resized); !errors.Is(err, ErrCameraMismatch) {
		t.Fatalf("other frame size: %v", err)
	}
	recropped := input()
	recropped.Crop.X = 0
	if err := SameSetup(first, recropped); err != nil {
		t.Fatalf("a correction may change the crop: %v", err)
	}
}
