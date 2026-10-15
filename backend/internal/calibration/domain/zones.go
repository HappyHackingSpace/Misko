package domain

import (
	"errors"
	"math"
	"slices"
	"sort"
)

// MaxShapesPerZone bounds the shapes of one zone key (Barnes maze holes are
// the largest set).
const MaxShapesPerZone = 100

const maxPolygonVertices = 100

var (
	ErrUnknownZone      = errors.New("zone keys must be calibrated zones of the paradigm version")
	ErrMissingZone      = errors.New("every required calibrated zone of the paradigm version needs a shape")
	ErrInvalidZone      = errors.New("a zone shape is a circle with a positive finite radius or a polygon of 3 to 100 finite vertices, at most 100 shapes per zone")
	ErrZoneOutsideFrame = errors.New("zone shapes must lie inside the camera frame")
	ErrZonesFrozen      = errors.New("zones are drawn with their calibration and never added later")
)

// ZoneRequirement is one calibrated zone of a paradigm version: its shapes are
// drawn on the camera frame with the calibration.
type ZoneRequirement struct {
	Key      string
	Required bool
}

type Vertex struct{ X, Y float64 }

type Circle struct{ X, Y, Radius float64 }

// Shape is either a circle or a polygon.
type Shape struct {
	Circle  *Circle
	Polygon []Vertex
}

// ZoneInput is one shape of a calibrated zone as drawn, in full-frame pixels.
type ZoneInput struct {
	Key string
	Shape
}

// Zone is one shape of a calibrated zone, as drawn in full-frame pixels and in
// the calibration's centimetres.
type Zone struct {
	Key         string
	Pixels      Shape
	Centimeters Shape
}

// circleSamples is the number of boundary points used to convert a circle.
const circleSamples = 32

// buildZones validates the drawn zones against the paradigm's calibrated zones
// and the frame, and converts them to centimetres with h. A homography does
// not keep circles round, so a circle keeps its mapped centre and gets the
// mean distance from it to its mapped boundary as the radius.
func buildZones(in []ZoneInput, req []ZoneRequirement, frameWidth, frameHeight int, h Homography) ([]Zone, error) {
	counts := map[string]int{}
	out := make([]Zone, 0, len(in))
	for _, z := range in {
		if !slices.ContainsFunc(req, func(r ZoneRequirement) bool { return r.Key == z.Key }) {
			return nil, ErrUnknownZone
		}
		counts[z.Key]++
		if counts[z.Key] > MaxShapesPerZone || !validShape(z.Shape) {
			return nil, ErrInvalidZone
		}
		if !insideFrame(z.Shape, float64(frameWidth), float64(frameHeight)) {
			return nil, ErrZoneOutsideFrame
		}
		cm, err := convert(z.Shape, h)
		if err != nil {
			return nil, err
		}
		out = append(out, Zone{Key: z.Key, Pixels: cloneShape(z.Shape), Centimeters: cm})
	}
	for _, r := range req {
		if r.Required && counts[r.Key] == 0 {
			return nil, ErrMissingZone
		}
	}
	// Shapes of one key keep their drawing order; keys are sorted so the
	// stored positions do not depend on the request order of other keys.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func validShape(s Shape) bool {
	if (s.Circle == nil) == (len(s.Polygon) == 0) {
		return false
	}
	if s.Circle != nil {
		return finite(s.Circle.X) && finite(s.Circle.Y) && finite(s.Circle.Radius) && s.Circle.Radius > 0
	}
	if len(s.Polygon) < 3 || len(s.Polygon) > maxPolygonVertices {
		return false
	}
	for _, v := range s.Polygon {
		if !finite(v.X) || !finite(v.Y) {
			return false
		}
	}
	return true
}

func insideFrame(s Shape, width, height float64) bool {
	if c := s.Circle; c != nil {
		return c.X-c.Radius >= 0 && c.Y-c.Radius >= 0 && c.X+c.Radius <= width && c.Y+c.Radius <= height
	}
	for _, v := range s.Polygon {
		if v.X < 0 || v.Y < 0 || v.X > width || v.Y > height {
			return false
		}
	}
	return true
}

func convert(s Shape, h Homography) (Shape, error) {
	if c := s.Circle; c != nil {
		cx, cy, err := h.Apply(c.X, c.Y)
		if err != nil {
			return Shape{}, ErrInvalidZone
		}
		var sum float64
		for i := range circleSamples {
			angle := 2 * math.Pi * float64(i) / circleSamples
			x, y, err := h.Apply(c.X+c.Radius*math.Cos(angle), c.Y+c.Radius*math.Sin(angle))
			if err != nil {
				return Shape{}, ErrInvalidZone
			}
			sum += math.Hypot(x-cx, y-cy)
		}
		radius := sum / circleSamples
		if !finite(radius) || radius <= 0 {
			return Shape{}, ErrInvalidZone
		}
		return Shape{Circle: &Circle{X: cx, Y: cy, Radius: radius}}, nil
	}
	out := make([]Vertex, 0, len(s.Polygon))
	for _, v := range s.Polygon {
		x, y, err := h.Apply(v.X, v.Y)
		if err != nil {
			return Shape{}, ErrInvalidZone
		}
		out = append(out, Vertex{X: x, Y: y})
	}
	return Shape{Polygon: out}, nil
}

func cloneShape(s Shape) Shape {
	out := Shape{Polygon: slices.Clone(s.Polygon)}
	if s.Circle != nil {
		c := *s.Circle
		out.Circle = &c
	}
	return out
}

// MissingZones lists the required calibrated zones the calibration has no
// shape for. A calibration with missing zones cannot be analysed: it was
// stored before its paradigm's zones were drawn with calibrations.
func MissingZones(c Calibration, req []ZoneRequirement) []string {
	var missing []string
	for _, r := range req {
		if r.Required && !slices.ContainsFunc(c.Zones, func(z Zone) bool { return z.Key == r.Key }) {
			missing = append(missing, r.Key)
		}
	}
	return missing
}
