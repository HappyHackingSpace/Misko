package domain

import "testing"

func TestShapesIncludeTheirBoundary(t *testing.T) {
	square := rect(0, 0, 10, 10)
	lShape := Shape{Polygon: []Vertex{{0, 0}, {10, 0}, {10, 4}, {4, 4}, {4, 10}, {0, 10}}}
	disk := circle(0, 0, 5)
	for _, tc := range []struct {
		name  string
		shape Shape
		x, y  float64
		want  bool
	}{
		{"square inside", square, 5, 5, true},
		{"square vertex", square, 0, 0, true},
		{"square edge", square, 10, 5, true},
		{"square top edge", square, 5, 10, true},
		{"square just outside", square, 10.001, 5, false},
		{"square below", square, 5, -0.001, false},
		{"concave notch is outside", lShape, 7, 7, false},
		{"concave upper arm", lShape, 2, 8, true},
		{"concave lower arm", lShape, 7, 2, true},
		{"concave inner edge", lShape, 4, 7, true},
		{"disk boundary", disk, 3, 4, true},
		{"disk just outside", disk, 3.001, 4, false},
	} {
		if got := tc.shape.contains(tc.x, tc.y); got != tc.want {
			t.Errorf("%s: contains(%v, %v) = %v", tc.name, tc.x, tc.y, got)
		}
	}
}
