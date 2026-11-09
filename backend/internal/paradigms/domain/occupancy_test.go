package domain

import (
	"errors"
	"testing"
)

// A 50 x 50 cm novel object arena in 10 cells of 5 cm. Counted intervals credit
// the earlier sample's cell:
//
//	0.0-0.2 s at (2, 2): 0.2 s in cell (0, 0).
//	0.2-0.3 s at (47, 48): 0.1 s in cell (9, 9).
//	0.3-0.4 s at (60, -5), past the wall: 0.1 s in the edge cell (9, 0).
//	0.4 s lost, then 2.0 s after a gap longer than max_sample_gap_s: not counted.
func TestOccupancyHandCalculated(t *testing.T) {
	trajectory := []Sample{sample(0, 2, 2), sample(0.1, 2, 2), sample(0.2, 47, 48), sample(0.3, 60, -5), sample(0.4, 60, -5), lost(0.5), sample(2, 25, 25), sample(3, 25, 25)}
	o, err := OccupancyOf("NOVEL_OBJECT", 1, nil, trajectory, 10)
	if err != nil {
		t.Fatal(err)
	}
	if o.Columns != 10 || o.Rows != 10 || o.CellCm != 5 || o.Bounds != (Bounds{0, 0, 50, 50}) {
		t.Fatalf("grid %+v", o)
	}
	want := map[int]int64{0: 200_000, 99: 100_000, 9: 100_000}
	for i, us := range o.Us {
		if us != want[i] {
			t.Errorf("cell %d: %d us, want %d", i, us, want[i])
		}
	}
}

// A rectangular arena keeps square cells along the shorter side.
func TestOccupancyKeepsCellsSquare(t *testing.T) {
	o, err := OccupancyOf("NOVEL_OBJECT", 1, map[string]float64{"arena_width_cm": 60, "arena_height_cm": 40}, nil, 12)
	if err != nil || o.CellCm != 5 || o.Columns != 12 || o.Rows != 8 || len(o.Us) != 96 {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestOccupancyNeedsAnArenaFloor(t *testing.T) {
	for _, key := range []string{"POLE", "TREADMILL"} {
		if _, err := OccupancyOf(key, 1, nil, nil, 10); !errors.Is(err, ErrNoArenaFloor) {
			t.Errorf("%s: %v", key, err)
		}
	}
	if _, err := OccupancyOf("MWM", 1, nil, []Sample{sample(1, 0, 0), sample(0.5, 0, 0)}, 10); !errors.Is(err, ErrInvalidSample) {
		t.Errorf("unordered samples: %v", err)
	}
}
