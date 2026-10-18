package catalog

import (
	"encoding/json"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"math"
	"os"
	"testing"
)

// workerFixture is written by worker/tests/contract_fixture.py: the trajectory
// the Python worker produced from a synthetic swim, and the true positions of
// that swim.
type workerFixture struct {
	Algorithm string `json:"algorithm"`
	Paradigm  struct {
		Key     string `json:"key"`
		Version int    `json:"version"`
	} `json:"paradigm"`
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
// the true positions. Tolerances for this synthetic water maze swim (7 s, 25
// fps, tank-centred centimetres): path length and mean distance to the platform
// within 2%, the escape latency and the quadrant and wall zone times within
// 0.08 s (two frames), the same platform crossings, and event boundaries within
// 0.08 s. Measured when generated: path length +0.25%, mean distance +0.04%,
// times and boundaries equal. This is not evidence of accuracy on real animals.
func TestWorkerTrajectoriesMatchTheEngineContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/worker_mwm_swim.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture workerFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Paradigm.Key != "MWM" || fixture.Paradigm.Version != 1 {
		t.Fatalf("fixture paradigm: %+v", fixture.Paradigm)
	}
	var header struct {
		RunID      string `json:"runId"`
		Attempt    int    `json:"attempt"`
		DurationUs int64  `json:"durationUs"`
	}
	if err := json.Unmarshal(fixture.Trajectory, &header); err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: header.RunID, Attempt: header.Attempt}
	measured, err := domain.ParseTrajectory(fixture.Trajectory, job, header.DurationUs)
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

	got, err := New().Evaluate("MWM", 1, fixture.Parameters, measured, nil, header.DurationUs)
	if err != nil {
		t.Fatal(err)
	}
	want, err := New().Evaluate("MWM", 1, fixture.Parameters, truth, nil, header.DurationUs)
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
	duration := value(want, "duration_s")
	for key, tolerance := range map[string]float64{"duration_s": 1e-9, "escape_latency_s": 0.08,
		"target_quadrant_time_ratio": 0.08 / duration, "thigmotaxis_time_ratio": 0.08 / duration, "platform_crossings_count": 0} {
		g, w := value(got, key), value(want, key)
		t.Logf("%s: worker %.4f, truth %.4f", key, g, w)
		if math.Abs(g-w) > tolerance {
			t.Errorf("%s: worker %.4f, truth %.4f, tolerance %.4f", key, g, w, tolerance)
		}
	}
	for _, key := range []string{"path_length_cm", "mean_distance_to_platform_cm"} {
		g, w := value(got, key), value(want, key)
		t.Logf("%s: worker %.4f, truth %.4f", key, g, w)
		if math.Abs(g-w) > 0.02*w {
			t.Errorf("%s: worker %.4f, truth %.4f", key, g, w)
		}
	}

	// Platform visits: each platform interval of the truth has a counterpart
	// starting and ending within 0.08 s.
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
