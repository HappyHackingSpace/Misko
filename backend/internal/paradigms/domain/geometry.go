package domain

import "math"

const edgeTolerance = 1e-9

func (s Shape) valid() bool {
	if (s.Circle == nil) == (len(s.Polygon) == 0) {
		return false
	}
	if s.Circle != nil {
		return finite(s.Circle.X) && finite(s.Circle.Y) && finite(s.Circle.Radius) && s.Circle.Radius > 0
	}
	if len(s.Polygon) < 3 {
		return false
	}
	for _, v := range s.Polygon {
		if !finite(v.X) || !finite(v.Y) {
			return false
		}
	}
	return true
}

// contains includes the boundary: a closed disk or a closed polygon, which may
// be concave.
func (s Shape) contains(x, y float64) bool {
	if s.Circle != nil {
		return math.Hypot(x-s.Circle.X, y-s.Circle.Y) <= s.Circle.Radius
	}
	inside := false
	for i, j := 0, len(s.Polygon)-1; i < len(s.Polygon); j, i = i, i+1 {
		a, b := s.Polygon[i], s.Polygon[j]
		if onSegment(a, b, x, y) {
			return true
		}
		if (a.Y > y) != (b.Y > y) && x < (b.X-a.X)*(y-a.Y)/(b.Y-a.Y)+a.X {
			inside = !inside
		}
	}
	return inside
}

func onSegment(a, b Vertex, x, y float64) bool {
	cross := (b.X-a.X)*(y-a.Y) - (b.Y-a.Y)*(x-a.X)
	if math.Abs(cross) > edgeTolerance*math.Max(1, math.Hypot(b.X-a.X, b.Y-a.Y)) {
		return false
	}
	return x >= math.Min(a.X, b.X)-edgeTolerance && x <= math.Max(a.X, b.X)+edgeTolerance &&
		y >= math.Min(a.Y, b.Y)-edgeTolerance && y <= math.Max(a.Y, b.Y)+edgeTolerance
}

func inAny(shapes []Shape, x, y float64) bool {
	for _, s := range shapes {
		if s.contains(x, y) {
			return true
		}
	}
	return false
}
