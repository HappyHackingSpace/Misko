package domain

import (
	"errors"
	"math"
)

// ErrNoArenaFloor means the paradigm has no fixed floor to map occupancy on,
// as in the side views of the pole and the treadmill.
var ErrNoArenaFloor = errors.New("the paradigm has no arena floor to map occupancy on")

// Occupancy is the tracked time spent in each square cell of the arena floor,
// row by row from MinY. Time is counted as the engine counts zone time, so
// the cells of a zone add up to its time in the zone.
type Occupancy struct {
	Bounds        Bounds
	CellCm        float64
	Columns, Rows int
	Us            []int64
}

// OccupancyOf maps a trajectory onto a grid of cells cells along the longer
// side of the arena. Samples past the walls count in the edge cells.
func OccupancyOf(key string, version int, parameters map[string]float64, samples []Sample, cells int) (Occupancy, error) {
	d, err := lookup(key, version)
	if err != nil {
		return Occupancy{}, err
	}
	params, err := resolveParameters(d.paradigm, parameters)
	if err != nil {
		return Occupancy{}, err
	}
	if err := validateSamples(samples); err != nil {
		return Occupancy{}, err
	}
	needs, err := CalibrationRequirement(key, version, params)
	if err != nil {
		return Occupancy{}, err
	}
	b := needs.Bounds
	if b == nil || b.MaxX <= b.MinX || b.MaxY <= b.MinY || cells < 1 {
		return Occupancy{}, ErrNoArenaFloor
	}
	width, height := b.MaxX-b.MinX, b.MaxY-b.MinY
	cell := math.Max(width, height) / float64(cells)
	o := Occupancy{Bounds: *b, CellCm: cell, Columns: int(math.Ceil(width / cell)), Rows: int(math.Ceil(height / cell))}
	o.Us = make([]int64, o.Columns*o.Rows)
	// Clamped as a float: converting an out-of-range float to int is undefined.
	index := func(v, from float64, n int) int { return int(max(0, min(float64(n-1), math.Floor((v-from)/cell)))) }
	walk(samples, validOnly, nil, gapUs(params), func(a, _ Sample, dt int64) {
		o.Us[index(a.Y, b.MinY, o.Rows)*o.Columns+index(a.X, b.MinX, o.Columns)] += dt
	})
	return o, nil
}
