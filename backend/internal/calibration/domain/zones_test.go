package domain

import (
	"errors"
	"math"
	"testing"
)

func maze() Requirement {
	req := arena()
	req.Bounds = nil
	req.Zones = []ZoneRequirement{{Key: "center", Required: true}, {Key: "open_arms", Required: true}, {Key: "interaction", Required: false}}
	return req
}

func square(x0, y0, x1, y1 float64) Shape {
	return Shape{Polygon: []Vertex{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}}
}

func zoned(zones ...ZoneInput) Input {
	in := input()
	in.Zones = zones
	return in
}

func TestZonesAreConvertedWithTheFittedTransform(t *testing.T) {
	in := zoned(
		ZoneInput{Key: "open_arms", Shape: square(200, 400, 600, 500)},
		ZoneInput{Key: "center", Shape: Shape{Circle: &Circle{X: 960, Y: 540, Radius: 40}}},
		ZoneInput{Key: "open_arms", Shape: square(1200, 400, 1700, 500)},
	)
	c, err := New(in, maze(), "tech", now)
	if err != nil {
		t.Fatal(err)
	}
	// Keys are sorted; shapes of one key keep their drawing order.
	if len(c.Zones) != 3 || c.Zones[0].Key != "center" || c.Zones[1].Key != "open_arms" || c.Zones[1].Pixels.Polygon[0] != (Vertex{200, 400}) ||
		c.Zones[2].Pixels.Polygon[0] != (Vertex{1200, 400}) {
		t.Fatalf("zones: %+v", c.Zones)
	}
	for i, v := range c.Zones[1].Centimeters.Polygon {
		p := c.Zones[1].Pixels.Polygon[i]
		x, y, _ := truth.Apply(p.X, p.Y)
		if math.Abs(v.X-x) > 1e-6 || math.Abs(v.Y-y) > 1e-6 {
			t.Fatalf("vertex %d: %v want %v %v", i, v, x, y)
		}
	}
	circle := c.Zones[0].Centimeters.Circle
	cx, cy, _ := truth.Apply(960, 540)
	// About 0.03-0.045 cm per pixel here, so 40 px is between 1.1 and 1.9 cm.
	if circle == nil || math.Abs(circle.X-cx) > 1e-6 || math.Abs(circle.Y-cy) > 1e-6 || circle.Radius < 1.1 || circle.Radius > 1.9 {
		t.Fatalf("circle: %+v want centre %v %v", circle, cx, cy)
	}
	if missing := MissingZones(c, maze().Zones); len(missing) != 0 {
		t.Fatalf("missing: %v", missing)
	}
	if missing := MissingZones(Calibration{}, maze().Zones); len(missing) != 2 || missing[0] != "center" || missing[1] != "open_arms" {
		t.Fatalf("missing without zones: %v", missing)
	}
}

func TestZonesAreValidated(t *testing.T) {
	center := ZoneInput{Key: "center", Shape: Shape{Circle: &Circle{X: 960, Y: 540, Radius: 40}}}
	arm := ZoneInput{Key: "open_arms", Shape: square(200, 400, 600, 500)}
	many := []ZoneInput{center}
	for range MaxShapesPerZone + 1 {
		many = append(many, arm)
	}
	for name, tc := range map[string]struct {
		in   Input
		req  Requirement
		want error
	}{
		"unknown key":          {zoned(center, arm, ZoneInput{Key: "closed_arms", Shape: arm.Shape}), maze(), ErrUnknownZone},
		"zones for a box":      {zoned(center), arena(), ErrUnknownZone},
		"required zone absent": {zoned(arm), maze(), ErrMissingZone},
		"no zones at all":      {input(), maze(), ErrMissingZone},
		"zero radius":          {zoned(arm, ZoneInput{Key: "center", Shape: Shape{Circle: &Circle{X: 960, Y: 540}}}), maze(), ErrInvalidZone},
		"two vertices":         {zoned(center, ZoneInput{Key: "open_arms", Shape: Shape{Polygon: []Vertex{{1, 1}, {2, 2}}}}), maze(), ErrInvalidZone},
		"circle and polygon":   {zoned(center, ZoneInput{Key: "open_arms", Shape: Shape{Circle: &Circle{X: 9, Y: 9, Radius: 1}, Polygon: arm.Polygon}}), maze(), ErrInvalidZone},
		"empty shape":          {zoned(center, ZoneInput{Key: "open_arms"}), maze(), ErrInvalidZone},
		"not finite":           {zoned(center, ZoneInput{Key: "open_arms", Shape: square(math.NaN(), 0, 10, 10)}), maze(), ErrInvalidZone},
		"too many shapes":      {zoned(many...), maze(), ErrInvalidZone},
		"polygon off frame":    {zoned(center, ZoneInput{Key: "open_arms", Shape: square(1800, 400, 1921, 500)}), maze(), ErrZoneOutsideFrame},
		"circle off frame":     {zoned(arm, ZoneInput{Key: "center", Shape: Shape{Circle: &Circle{X: 20, Y: 540, Radius: 40}}}), maze(), ErrZoneOutsideFrame},
	} {
		if _, err := New(tc.in, tc.req, "tech", now); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v want %v", name, err, tc.want)
		}
	}
	// An optional zone may be left out; a zone may touch the frame edge.
	if _, err := New(zoned(center, ZoneInput{Key: "open_arms", Shape: square(0, 0, 1920, 1080)}), maze(), "tech", now); err != nil {
		t.Fatalf("zones on the frame edge: %v", err)
	}
}
