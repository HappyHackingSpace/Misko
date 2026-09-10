package domain

import (
	"errors"
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

func mark(typ string, seconds float64) ObservedEvent {
	return ObservedEvent{Type: typ, StartUs: at(seconds), EndUs: at(seconds)}
}

func span(typ, label string, from, to float64) ObservedEvent {
	return ObservedEvent{Type: typ, Label: label, StartUs: at(from), EndUs: at(to)}
}
