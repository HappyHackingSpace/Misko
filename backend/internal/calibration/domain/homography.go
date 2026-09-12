package domain

import "math"

// Homography maps pixel coordinates (x, y) to plane coordinates in centimeters:
// u = (h0 x + h1 y + h2) / w, v = (h3 x + h4 y + h5) / w, w = h6 x + h7 y + h8.
type Homography [9]float64

func (h Homography) Apply(x, y float64) (float64, float64, error) {
	w := h[6]*x + h[7]*y + h[8]
	if math.Abs(w) < 1e-12 {
		return 0, 0, ErrDegenerateFit
	}
	u, v := (h[0]*x+h[1]*y+h[2])/w, (h[3]*x+h[4]*y+h[5])/w
	if !finite(u) || !finite(v) {
		return 0, 0, ErrDegenerateFit
	}
	return u, v, nil
}

// fitHomography solves the normalized direct linear transform by least squares
// with h8 fixed to 1. Both point sets are first centered and scaled to a mean
// distance of sqrt(2) (Hartley normalization) so the system is well conditioned.
// Collinear or repeated points make the system singular and are rejected.
func fitHomography(points []Correspondence) (Homography, error) {
	pcx, pcy, ps, ok := normalization(points, func(c Correspondence) (float64, float64) { return c.PixelX, c.PixelY })
	if !ok {
		return Homography{}, ErrDegenerateFit
	}
	wcx, wcy, ws, ok := normalization(points, func(c Correspondence) (float64, float64) { return c.WorldX, c.WorldY })
	if !ok {
		return Homography{}, ErrDegenerateFit
	}
	var ata [8][9]float64 // normal equations with the right-hand side in column 8
	for _, c := range points {
		x, y := (c.PixelX-pcx)*ps, (c.PixelY-pcy)*ps
		u, v := (c.WorldX-wcx)*ws, (c.WorldY-wcy)*ws
		for _, row := range [2][9]float64{
			{x, y, 1, 0, 0, 0, -x * u, -y * u, u},
			{0, 0, 0, x, y, 1, -x * v, -y * v, v},
		} {
			for i := 0; i < 8; i++ {
				for j := 0; j < 9; j++ {
					ata[i][j] += row[i] * row[j]
				}
			}
		}
	}
	h, ok := solve(ata)
	if !ok {
		return Homography{}, ErrDegenerateFit
	}
	normalized := [9]float64{h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7], 1}
	toPixel := [9]float64{ps, 0, -ps * pcx, 0, ps, -ps * pcy, 0, 0, 1}
	fromWorld := [9]float64{1 / ws, 0, wcx, 0, 1 / ws, wcy, 0, 0, 1}
	m := multiply(fromWorld, multiply(normalized, toPixel))
	if math.Abs(m[8]) < 1e-12 {
		return Homography{}, ErrDegenerateFit
	}
	var out Homography
	for i := range m {
		out[i] = m[i] / m[8]
	}
	return out, nil
}

func normalization(points []Correspondence, coords func(Correspondence) (float64, float64)) (cx, cy, scale float64, ok bool) {
	for _, p := range points {
		x, y := coords(p)
		cx, cy = cx+x, cy+y
	}
	n := float64(len(points))
	cx, cy = cx/n, cy/n
	var mean float64
	for _, p := range points {
		x, y := coords(p)
		mean += math.Hypot(x-cx, y-cy)
	}
	mean /= n
	if mean < 1e-9 {
		return 0, 0, 0, false
	}
	return cx, cy, math.Sqrt2 / mean, true
}

// solve runs Gaussian elimination with partial pivoting on an 8x8 system and
// reports false when a pivot is negligible relative to the matrix scale.
func solve(a [8][9]float64) ([8]float64, bool) {
	var scale float64
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			scale = math.Max(scale, math.Abs(a[i][j]))
		}
	}
	if scale == 0 {
		return [8]float64{}, false
	}
	for col := 0; col < 8; col++ {
		pivot := col
		for r := col + 1; r < 8; r++ {
			if math.Abs(a[r][col]) > math.Abs(a[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(a[pivot][col]) < 1e-10*scale {
			return [8]float64{}, false
		}
		a[col], a[pivot] = a[pivot], a[col]
		for r := col + 1; r < 8; r++ {
			f := a[r][col] / a[col][col]
			for c := col; c < 9; c++ {
				a[r][c] -= f * a[col][c]
			}
		}
	}
	var x [8]float64
	for r := 7; r >= 0; r-- {
		sum := a[r][8]
		for c := r + 1; c < 8; c++ {
			sum -= a[r][c] * x[c]
		}
		x[r] = sum / a[r][r]
		if !finite(x[r]) {
			return [8]float64{}, false
		}
	}
	return x, true
}

func multiply(a, b [9]float64) [9]float64 {
	var out [9]float64
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			for k := 0; k < 3; k++ {
				out[3*r+c] += a[3*r+k] * b[3*k+c]
			}
		}
	}
	return out
}

// reprojection returns the root mean square and largest distance in
// centimeters between mapped pixels and their plane coordinates.
func reprojection(h Homography, points []Correspondence) (rms, largest float64, err error) {
	for _, p := range points {
		u, v, err := h.Apply(p.PixelX, p.PixelY)
		if err != nil {
			return 0, 0, err
		}
		d := math.Hypot(u-p.WorldX, v-p.WorldY)
		rms += d * d
		largest = math.Max(largest, d)
	}
	return math.Sqrt(rms / float64(len(points))), largest, nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
