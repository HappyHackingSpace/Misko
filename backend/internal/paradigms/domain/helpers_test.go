package domain

import (
	"errors"
	"math"
	"slices"
	"testing"
)

func run(t *testing.T, key string, in Input) Result {
	t.Helper()
	r, err := Evaluate(key, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func expectMissing(t *testing.T, r Result, key, reason string) {
	t.Helper()
	m, ok := r.Metric(key)
	if !ok || !m.Missing || m.Reason != reason || m.Value != 0 {
		t.Errorf("%s = %+v, want missing %s", key, m, reason)
	}
}

func expectError(t *testing.T, key string, in Input, want error) {
	t.Helper()
	if _, err := Evaluate(key, 1, in); !errors.Is(err, want) {
		t.Errorf("%s: err=%v want %v", key, err, want)
	}
}

func rect(x0, y0, x1, y1 float64) Shape {
	return Shape{Polygon: []Vertex{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}}
}

func circle(x, y, r float64) Shape { return Shape{Circle: &Circle{X: x, Y: y, Radius: r}} }

// steady returns tracked samples every step seconds from `from` to `to`
// (both included) at one position.
func steady(from, to, step, x, y float64) []Sample {
	n := int(math.Round((to - from) / step))
	out := make([]Sample, 0, n+1)
	for i := 0; i <= n; i++ {
		out = append(out, sample(from+float64(i)*step, x, y))
	}
	return out
}

func samples(parts ...[]Sample) []Sample { return slices.Concat(parts...) }

// Calibrated zones of the hand calculations.
var (
	// Novel object: a disk of radius 4 cm at (10, 10); familiar object: an
	// 8 cm square centred on (40, 10).
	objectZones = map[string][]Shape{"novel_object": {circle(10, 10, 4)}, "familiar_object": {rect(36, 6, 44, 14)}}
	// Pole: a 4 cm wide band from the base (y = 0) to the top (y = 50); base:
	// the floor strip y <= 1 cm, 60 cm wide.
	poleZones = map[string][]Shape{"pole": {rect(-2, 0, 2, 50)}, "base": {rect(-30, -5, 30, 1)}}
	// Treadmill: the shock grid covers the first 5 cm of the lane.
	gridZones = map[string][]Shape{"shock_grid": {rect(0, -2, 5, 10)}}
)
