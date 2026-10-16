package domain

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
