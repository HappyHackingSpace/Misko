// Package domain holds calibrations: a pixel-to-centimeter homography fitted
// from FIT points and validated against independent CHECK points. A
// calibration is either the default of an environment revision or a manual
// override for one recording. Calibrations never change; a correction is a new
// calibration that supersedes the latest one of the same chain.
package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// AlgorithmVersion identifies the fitting method stored with every calibration.
const AlgorithmVersion = "homography-dlt-normalized-v1"

const maxPoints = 100

var (
	ErrNotRequired             = errors.New("this paradigm does not use video calibration")
	ErrInvalidCamera           = errors.New("camera id must contain 1 to 120 printable characters")
	ErrInvalidFrame            = errors.New("frame width and height must be between 1 and 16384 pixels")
	ErrInvalidCrop             = errors.New("crop must be a non-empty rectangle inside the frame")
	ErrInvalidPlane            = errors.New("measurement plane must be ARENA_FLOOR, WATER_SURFACE or APPARATUS_TOP")
	ErrInvalidReferenceFrame   = errors.New("reference frame time must not be negative")
	ErrInsufficientFitPoints   = errors.New("a calibration needs 4 to 100 FIT points")
	ErrInsufficientCheckPoints = errors.New("a calibration needs 3 to 100 independent CHECK points")
	ErrInvalidPoint            = errors.New("calibration points need finite pixel and plane coordinates")
	ErrPointOutsideCrop        = errors.New("calibration points must lie inside the crop")
	ErrPointOutsideEnvironment = errors.New("plane coordinates must lie inside the environment")
	ErrCheckPointReused        = errors.New("CHECK points must differ from FIT points")
	ErrDegenerateFit           = errors.New("FIT points must include four points with no three on a line")
	ErrStaleCorrection         = errors.New("a correction must supersede the latest calibration of its chain, and only the first calibration may have none")
	ErrCameraMismatch          = errors.New("a correction must use the same camera and frame size")
)

type Plane string

const (
	ArenaFloor   Plane = "ARENA_FLOOR"
	WaterSurface Plane = "WATER_SURFACE"
	ApparatusTop Plane = "APPARATUS_TOP"
)

// Scope tells what a calibration applies to.
type Scope string

const (
	// ScopeEnvironment is the default of an environment revision, entered once
	// when the rig is set up and used by every recording of that revision.
	ScopeEnvironment Scope = "ENVIRONMENT"
	// ScopeRecording is a manual override for one recording, used when its
	// camera or arena moved.
	ScopeRecording Scope = "RECORDING"
)

type Status string

const (
	Valid    Status = "VALID"
	Rejected Status = "REJECTED"
)

// RejectedCheckError means CHECK points deviate more than the tolerance.
const RejectedCheckError = "EXCESSIVE_CHECK_ERROR"

// Correspondence pairs a pixel in the full frame with plane coordinates in centimeters.
type Correspondence struct {
	PixelX, PixelY float64
	WorldX, WorldY float64
}

type Rect struct {
	X, Y, Width, Height int
}

type Bounds struct {
	MinX, MinY, MaxX, MaxY float64
}

// Requirement is what the paradigm asks of a calibration.
type Requirement struct {
	Required    bool
	ToleranceCm float64
	Bounds      *Bounds
}

type Input struct {
	CameraID                string
	FrameWidth, FrameHeight int
	Crop                    Rect
	// ReferenceFrameUs is the video time of the frame the points were read from.
	// Environment calibrations have no video and leave it zero.
	ReferenceFrameUs int64
	Plane            Plane
	Fit, Check       []Correspondence
	SupersedesID     string
}

type Calibration struct {
	ID string
	// RecordingID is empty for an environment calibration.
	RecordingID string
	// EnvironmentRevisionID is the revision whose measurements the points were
	// validated against.
	EnvironmentRevisionID   string
	SupersedesID            string
	CameraID                string
	FrameWidth, FrameHeight int
	Crop                    Rect
	ReferenceFrameUs        int64
	Plane                   Plane
	Fit, Check              []Correspondence
	Transform               Homography
	FitRMSErrorCm           float64
	CheckRMSErrorCm         float64
	CheckMaxErrorCm         float64
	ToleranceCm             float64
	AlgorithmVersion        string
	Status                  Status
	RejectionReason         string
	CreatedBy               string
	CreatedAt               time.Time
}

// New fits the transform and validates it against the CHECK points. Invalid
// input is an error; a fit whose CHECK error exceeds the tolerance is returned
// as REJECTED so the attempt is kept.
func New(in Input, req Requirement, createdBy string, now time.Time) (Calibration, error) {
	if !req.Required {
		return Calibration{}, ErrNotRequired
	}
	camera := strings.TrimSpace(in.CameraID)
	switch {
	case camera == "" || !utf8.ValidString(camera) || utf8.RuneCountInString(camera) > 120 || strings.IndexFunc(camera, unicode.IsControl) >= 0:
		return Calibration{}, ErrInvalidCamera
	case in.FrameWidth < 1 || in.FrameWidth > 16384 || in.FrameHeight < 1 || in.FrameHeight > 16384:
		return Calibration{}, ErrInvalidFrame
	case in.Crop.X < 0 || in.Crop.Y < 0 || in.Crop.Width < 1 || in.Crop.Height < 1 ||
		in.Crop.X+in.Crop.Width > in.FrameWidth || in.Crop.Y+in.Crop.Height > in.FrameHeight:
		return Calibration{}, ErrInvalidCrop
	case in.Plane != ArenaFloor && in.Plane != WaterSurface && in.Plane != ApparatusTop:
		return Calibration{}, ErrInvalidPlane
	case in.ReferenceFrameUs < 0:
		return Calibration{}, ErrInvalidReferenceFrame
	case len(in.Fit) < 4 || len(in.Fit) > maxPoints:
		return Calibration{}, ErrInsufficientFitPoints
	case len(in.Check) < 3 || len(in.Check) > maxPoints:
		return Calibration{}, ErrInsufficientCheckPoints
	}
	all := append(append([]Correspondence{}, in.Fit...), in.Check...)
	margin := req.ToleranceCm
	for _, p := range all {
		switch {
		case !finite(p.PixelX) || !finite(p.PixelY) || !finite(p.WorldX) || !finite(p.WorldY):
			return Calibration{}, ErrInvalidPoint
		case p.PixelX < float64(in.Crop.X) || p.PixelY < float64(in.Crop.Y) ||
			p.PixelX > float64(in.Crop.X+in.Crop.Width) || p.PixelY > float64(in.Crop.Y+in.Crop.Height):
			return Calibration{}, ErrPointOutsideCrop
		case req.Bounds != nil && (p.WorldX < req.Bounds.MinX-margin || p.WorldY < req.Bounds.MinY-margin ||
			p.WorldX > req.Bounds.MaxX+margin || p.WorldY > req.Bounds.MaxY+margin):
			return Calibration{}, ErrPointOutsideEnvironment
		}
	}
	for _, c := range in.Check {
		for _, f := range in.Fit {
			if math.Hypot(c.PixelX-f.PixelX, c.PixelY-f.PixelY) < 1 {
				return Calibration{}, ErrCheckPointReused
			}
		}
	}
	h, err := fitHomography(in.Fit)
	if err != nil {
		return Calibration{}, err
	}
	fitRMS, _, err := reprojection(h, in.Fit)
	if err != nil {
		return Calibration{}, err
	}
	checkRMS, checkMax, err := reprojection(h, in.Check)
	if err != nil {
		return Calibration{}, err
	}
	c := Calibration{
		SupersedesID: in.SupersedesID, CameraID: camera, FrameWidth: in.FrameWidth, FrameHeight: in.FrameHeight, Crop: in.Crop,
		ReferenceFrameUs: in.ReferenceFrameUs, Plane: in.Plane, Fit: append([]Correspondence{}, in.Fit...), Check: append([]Correspondence{}, in.Check...),
		Transform: h, FitRMSErrorCm: fitRMS, CheckRMSErrorCm: checkRMS, CheckMaxErrorCm: checkMax, ToleranceCm: req.ToleranceCm,
		AlgorithmVersion: AlgorithmVersion, Status: Valid, CreatedBy: createdBy, CreatedAt: now,
	}
	// A small epsilon keeps an error exactly at the tolerance valid despite rounding.
	if checkMax > req.ToleranceCm+1e-9 {
		c.Status, c.RejectionReason = Rejected, RejectedCheckError
	}
	return c, nil
}

// Scope is ScopeRecording when the calibration belongs to a recording.
func (c Calibration) Scope() Scope {
	if c.RecordingID != "" {
		return ScopeRecording
	}
	return ScopeEnvironment
}

// Latest returns the calibration no other calibration supersedes, or nil.
func Latest(chain []Calibration) *Calibration {
	superseded := map[string]bool{}
	for _, c := range chain {
		if c.SupersedesID != "" {
			superseded[c.SupersedesID] = true
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if !superseded[chain[i].ID] {
			return &chain[i]
		}
	}
	return nil
}

// Current is the latest calibration when it is valid. Otherwise the recording
// waits for calibration and no physical metric may be produced.
func Current(chain []Calibration) *Calibration {
	if latest := Latest(chain); latest != nil && latest.Status == Valid {
		return latest
	}
	return nil
}

// CanCorrect allows one first calibration and then only corrections of the latest.
func CanCorrect(chain []Calibration, supersedesID string) error {
	latest := Latest(chain)
	if (latest == nil && supersedesID != "") || (latest != nil && latest.ID != supersedesID) {
		return ErrStaleCorrection
	}
	return nil
}

// SameSetup requires a correction to use the camera and frame size of the
// calibration it replaces; the crop may change.
func SameSetup(previous Calibration, in Input) error {
	if strings.TrimSpace(in.CameraID) != previous.CameraID || in.FrameWidth != previous.FrameWidth || in.FrameHeight != previous.FrameHeight {
		return ErrCameraMismatch
	}
	return nil
}

// Source names where the effective calibration of a recording comes from.
type Source string

const (
	SourceEnvironment Source = "ENVIRONMENT"
	SourceRecording   Source = "RECORDING"
)

// Resolution is the calibration a recording is analyzed with. A nil
// Calibration means the recording waits for calibration; Source then names the
// chain that is blocking it, or is empty when nothing has been calibrated.
type Resolution struct {
	Calibration *Calibration
	Source      Source
}

// Effective decides which calibration a recording uses. A calibration entered
// for the recording itself always decides: when it is REJECTED the recording
// waits instead of silently falling back to the environment, because someone
// entered it for a reason. Without one, the environment default applies when
// it is VALID. Every reader of calibration state must use this function so the
// status endpoint, the analysis scheduler and the dashboard cannot disagree.
func Effective(recording, environment []Calibration) Resolution {
	if latest := Latest(recording); latest != nil {
		if latest.Status == Valid {
			return Resolution{Calibration: latest, Source: SourceRecording}
		}
		return Resolution{Source: SourceRecording}
	}
	if current := Current(environment); current != nil {
		return Resolution{Calibration: current, Source: SourceEnvironment}
	}
	if Latest(environment) != nil {
		return Resolution{Source: SourceEnvironment}
	}
	return Resolution{}
}
