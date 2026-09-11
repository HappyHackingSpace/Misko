package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func trajectoryJSON(t *testing.T, run Run, duration int64, mutate func(map[string]any)) []byte {
	t.Helper()
	x, y := 10.0, 20.0
	doc := map[string]any{
		"schema": TrajectorySchema, "runId": run.ID, "attempt": run.Attempt, "durationUs": duration,
		"samples": map[string]any{
			"tUs":        []any{0, 100_000, 200_000, 300_000},
			"xCm":        []any{x, 11.0, nil, 13.0},
			"yCm":        []any{y, 20.0, nil, 20.0},
			"tracked":    []any{true, true, false, true},
			"confidence": []any{0.9, 0.8, 0.0, 1.0},
		},
		"worker": map[string]any{"algorithm": "background-subtraction", "pixels": []any{1, 2}},
	}
	if mutate != nil {
		mutate(doc)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func samples(doc map[string]any) map[string]any { return doc["samples"].(map[string]any) }

func TestTrajectoriesBelongToTheirRunAndRecording(t *testing.T) {
	run, _ := queued().Claim("worker-a", now, time.Minute)
	tr, err := ParseTrajectory(trajectoryJSON(t, run, 8_000_000, nil), run, 8_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Samples) != 4 || tr.Samples[1] != (Sample{TUs: 100_000, X: 11, Y: 20, Tracked: true, Confidence: 0.8}) || tr.Samples[2].Tracked {
		t.Fatalf("samples: %+v", tr.Samples)
	}
	for name, mutate := range map[string]func(map[string]any){
		"other schema":          func(d map[string]any) { d["schema"] = "misko.trajectory.v2" },
		"other run":             func(d map[string]any) { d["runId"] = "run-2" },
		"late attempt":          func(d map[string]any) { d["attempt"] = 2 },
		"other duration":        func(d map[string]any) { d["durationUs"] = 7_000_000 },
		"short array":           func(d map[string]any) { samples(d)["confidence"] = []any{1, 1, 1} },
		"decreasing time":       func(d map[string]any) { samples(d)["tUs"] = []any{0, 200_000, 100_000, 300_000} },
		"repeated time":         func(d map[string]any) { samples(d)["tUs"] = []any{0, 100_000, 100_000, 300_000} },
		"negative time":         func(d map[string]any) { samples(d)["tUs"] = []any{-1, 100_000, 200_000, 300_000} },
		"after the recording":   func(d map[string]any) { samples(d)["tUs"] = []any{0, 100_000, 200_000, 8_000_001} },
		"confidence above one":  func(d map[string]any) { samples(d)["confidence"] = []any{0.9, 1.5, 0, 1} },
		"tracked without place": func(d map[string]any) { samples(d)["xCm"] = []any{10.0, nil, nil, 13.0} },
		"not JSON":              nil,
	} {
		data := []byte("{not json")
		if mutate != nil {
			data = trajectoryJSON(t, run, 8_000_000, mutate)
		}
		if _, err := ParseTrajectory(data, run, 8_000_000); !errors.Is(err, ErrInvalidTrajectory) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The last sample may sit exactly at the end of the recording.
	atEnd := trajectoryJSON(t, run, 8_000_000, func(d map[string]any) { samples(d)["tUs"] = []any{0, 100_000, 200_000, 8_000_000} })
	if _, err := ParseTrajectory(atEnd, run, 8_000_000); err != nil {
		t.Fatalf("sample at the end: %v", err)
	}
}

func TestQualityRulesUseTrackedSamples(t *testing.T) {
	rules := []QCRule{{Key: "min_tracking_confidence", Operator: ">=", Value: 0.7}, {Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.25},
		{Key: "max_calibration_error_cm", Operator: "<=", Value: 2}}
	tr := Track{Samples: []Sample{
		{TUs: 0, Tracked: true, Confidence: 0.9}, {TUs: 1, Tracked: true, Confidence: 0.6}, {TUs: 2, Confidence: 0}, {TUs: 3, Tracked: true, Confidence: 0.9},
	}}
	// Mean tracked confidence 0.8 and one lost sample in four both pass; the
	// untracked sample's confidence does not lower the mean.
	if reason := CheckQC(rules, tr); reason != "" {
		t.Fatalf("passing trajectory: %s", reason)
	}
	lost := tr
	lost.Samples = append(append([]Sample(nil), tr.Samples...), Sample{TUs: 4})
	if reason := CheckQC(rules, lost); !strings.HasPrefix(reason, "QC_FAILED: max_lost_frame_ratio 0.4 <= 0.25") {
		t.Fatalf("too many lost samples: %q", reason)
	}
	unsure := Track{Samples: []Sample{{TUs: 0, Tracked: true, Confidence: 0.69}}}
	if reason := CheckQC(rules, unsure); !strings.HasPrefix(reason, "QC_FAILED: min_tracking_confidence 0.69 >= 0.7") {
		t.Fatalf("low confidence: %q", reason)
	}
	if reason := CheckQC(rules, Track{}); reason == "" {
		t.Fatal("an empty trajectory passed QC")
	}
	if reason := CheckQC(nil, Track{}); reason != "" {
		t.Fatalf("no rules: %q", reason)
	}
	if reason := CheckQC([]QCRule{{Key: "max_lost_frame_ratio", Operator: "<", Value: 1}}, tr); reason == "" {
		t.Fatal("an unknown operator passed")
	}
}

func TestComputedEventsTakeTheConfidenceOfTheirSamples(t *testing.T) {
	tr := Track{Samples: []Sample{
		{TUs: 0, Tracked: true, Confidence: 1}, {TUs: 100, Tracked: true, Confidence: 0.5}, {TUs: 200, Confidence: 0.9}, {TUs: 300, Tracked: true, Confidence: 0.7},
	}}
	events := tr.Events(Computed{
		Intervals: []Interval{{Type: "in_center", StartUs: 100, EndUs: 300}, {Type: "immobile", StartUs: 0, EndUs: 100}},
		Points:    []Point{{Type: "center_entry", AtUs: 100}, {Type: "center_entry", AtUs: 200}},
	})
	want := []Event{
		{Type: "immobile", Kind: "INTERVAL", StartUs: 0, EndUs: 100, Confidence: 0.75},
		{Type: "center_entry", Kind: "POINT", StartUs: 100, EndUs: 100, Confidence: 0.5},
		{Type: "in_center", Kind: "INTERVAL", StartUs: 100, EndUs: 300, Confidence: 0.6},
		{Type: "center_entry", Kind: "POINT", StartUs: 200, EndUs: 200, Confidence: 0},
	}
	if len(events) != len(want) {
		t.Fatalf("events: %+v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d: %+v want %+v", i, events[i], want[i])
		}
	}
}
