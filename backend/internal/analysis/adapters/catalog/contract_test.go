package catalog

import (
	"encoding/json"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"math"
	"os"
	"testing"
)

// workerFixture is written by worker/tests/contract_fixture.py: the trajectory
// the Python worker produced from a synthetic walk, and the true positions of
// that walk.
type workerFixture struct {
	Algorithm  string             `json:"algorithm"`
	Parameters map[string]float64 `json:"parameters"`
	Truth      struct {
		TUs []int64   `json:"tUs"`
		X   []float64 `json:"xCm"`
		Y   []float64 `json:"yCm"`
	} `json:"truth"`
	Trajectory json.RawMessage `json:"trajectory"`
}

// TestWorkerTrajectoriesMatchTheEngineContract parses the worker's output with
// the API's parser and compares the engine's results on it with the results on
// the true positions. Tolerances for this synthetic fixture (6 s, 25 fps, 320 x
// 240, about 4.4 px/cm): distance within 2%, zone and immobility times within
// 0.08 s (two frames), the same center entries, and event boundaries within
// 0.08 s. Measured when generated: distance +0.12%, times and boundaries equal.
// This is not evidence of accuracy on real animals.
func TestWorkerTrajectoriesMatchTheEngineContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/worker_open_field_walk.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture workerFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var header struct {
		RunID      string `json:"runId"`
		Attempt    int    `json:"attempt"`
		DurationUs int64  `json:"durationUs"`
	}
	if err := json.Unmarshal(fixture.Trajectory, &header); err != nil {
		t.Fatal(err)
	}
	run := domain.Run{ID: header.RunID, Attempt: header.Attempt}
	measured, err := domain.ParseTrajectory(fixture.Trajectory, run, header.DurationUs)
	if err != nil {
		t.Fatalf("the API rejects the worker's trajectory: %v", err)
	}
	if reason := domain.CheckQC([]domain.QCRule{{Key: "min_tracking_confidence", Operator: ">=", Value: 0.7}, {Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.1}}, measured); reason != "" {
		t.Fatalf("the fixture fails QC: %s", reason)
	}
	truth := domain.Track{}
	for i, tUs := range fixture.Truth.TUs {
		truth.Samples = append(truth.Samples, domain.Sample{TUs: tUs, X: fixture.Truth.X[i], Y: fixture.Truth.Y[i], Tracked: true, Confidence: 1})
	}

	got, err := New().Evaluate("OPEN_FIELD", 1, fixture.Parameters, measured, header.DurationUs)
	if err != nil {
		t.Fatal(err)
	}
	want, err := New().Evaluate("OPEN_FIELD", 1, fixture.Parameters, truth, header.DurationUs)
	if err != nil {
		t.Fatal(err)
	}
	value := func(c domain.Computed, key string) float64 {
		for _, m := range c.Metrics {
			if m.Key == key && m.Value != nil {
				return *m.Value
			}
		}
		t.Fatalf("metric %s missing", key)
		return 0
	}
	for key, tolerance := range map[string]float64{"center_time_s": 0.08, "periphery_time_s": 0.08, "immobility_s": 0.08, "duration_s": 1e-9, "center_entries_count": 0} {
		g, w := value(got, key), value(want, key)
		t.Logf("%s: worker %.4f, truth %.4f", key, g, w)
		if math.Abs(g-w) > tolerance {
			t.Errorf("%s: worker %.4f, truth %.4f, tolerance %.2f", key, g, w, tolerance)
		}
	}
	g, w := value(got, "distance_cm"), value(want, "distance_cm")
	t.Logf("distance_cm: worker %.4f, truth %.4f", g, w)
	if math.Abs(g-w) > 0.02*w {
		t.Errorf("distance_cm: worker %.4f, truth %.4f", g, w)
	}

	// Movement boundaries: each immobile interval and center interval of the
	// truth has a counterpart starting and ending within 0.08 s.
	const boundary = 80_000
	for _, wantInterval := range want.Intervals {
		matched := false
		for _, gotInterval := range got.Intervals {
			if gotInterval.Type == wantInterval.Type && abs(gotInterval.StartUs-wantInterval.StartUs) <= boundary && abs(gotInterval.EndUs-wantInterval.EndUs) <= boundary {
				matched = true
			}
		}
		t.Logf("%s [%d, %d) matched=%v", wantInterval.Type, wantInterval.StartUs, wantInterval.EndUs, matched)
		if !matched {
			t.Errorf("no worker interval for %+v in %+v", wantInterval, got.Intervals)
		}
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
