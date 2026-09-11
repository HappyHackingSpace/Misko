package catalog

import (
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"slices"
	"testing"
)

func TestContractsComeFromThePublishedParadigmVersion(t *testing.T) {
	c, err := New().Contract("OPEN_FIELD", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !c.CalibrationRequired || !c.TrajectoryAnalyzable || c.MetricEngineVersion != 1 || c.ResultSchemaVersion != 1 || len(c.Metrics) != 9 || len(c.Events) != 3 {
		t.Fatalf("OPEN_FIELD contract: %+v", c)
	}
	entries := c.Metrics[slices.IndexFunc(c.Metrics, func(m domain.MetricSpec) bool { return m.Key == "center_entries_count" })]
	if !entries.Integer || entries.Unit != "count" {
		t.Fatalf("integer metric: %+v", entries)
	}
	if !slices.Contains(c.MissingReasons, "NO_VALID_INTERVALS") || !slices.ContainsFunc(c.Events, func(e domain.EventSpec) bool { return e.Type == "center_entry" && e.Kind == "POINT" }) {
		t.Fatalf("reasons and events: %+v", c)
	}
	if !slices.Contains(c.QC, domain.QCRule{Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.1}) {
		t.Fatalf("QC rules: %+v", c.QC)
	}
	// Scored observations and calibrated zones cannot come from a trajectory.
	for _, key := range []string{"ROTAROD", "NOVEL_OBJECT"} {
		other, err := New().Contract(key, 1)
		if err != nil || other.TrajectoryAnalyzable {
			t.Errorf("%s: %+v %v", key, other, err)
		}
	}
	rotarod, _ := New().Contract("ROTAROD", 1)
	if rotarod.CalibrationRequired {
		t.Fatalf("ROTAROD: %+v", rotarod)
	}
	for _, tc := range []struct {
		key     string
		version int
	}{{"MAZE", 1}, {"OPEN_FIELD", 9}} {
		if _, err := New().Contract(tc.key, tc.version); !errors.Is(err, application.ErrUnknownCapability) {
			t.Errorf("%s v%d: %v", tc.key, tc.version, err)
		}
	}
}

func TestTheEngineComputesMetricsFromTheTrajectory(t *testing.T) {
	params := map[string]float64{"arena_width_cm": 50, "arena_height_cm": 50, "center_fraction": 0.5, "max_sample_gap_s": 0.5}
	// Periphery to center in two 0.1 s steps of 5 cm, then a lost sample.
	tr := domain.Track{Samples: []domain.Sample{
		{TUs: 0, X: 5, Y: 25, Tracked: true, Confidence: 1},
		{TUs: 100_000, X: 10, Y: 25, Tracked: true, Confidence: 1},
		{TUs: 200_000, X: 15, Y: 25, Tracked: true, Confidence: 1},
		{TUs: 300_000},
	}}
	c, err := New().Evaluate("OPEN_FIELD", 1, params, tr, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	metric := func(key string) domain.MetricValue {
		return c.Metrics[slices.IndexFunc(c.Metrics, func(m domain.MetricValue) bool { return m.Key == key })]
	}
	if d := metric("distance_cm"); d.Value == nil || *d.Value != 10 || d.Unit != "cm" || d.MissingReason != "" || len(c.Metrics) != 9 {
		t.Fatalf("distance: %+v", c.Metrics)
	}
	if e := metric("center_entries_count"); *e.Value != 1 || len(c.Points) != 1 || c.Points[0] != (domain.Point{Type: "center_entry", AtUs: 200_000}) {
		t.Fatalf("entries: %+v %+v", e, c.Points)
	}

	empty, err := New().Evaluate("OPEN_FIELD", 1, params, domain.Track{}, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range empty.Metrics {
		if m.Value != nil || m.MissingReason != "NO_VALID_INTERVALS" {
			t.Fatalf("a metric without counted intervals must be missing: %+v", m)
		}
	}
	backwards := domain.Track{Samples: []domain.Sample{{TUs: 5}, {TUs: 1}}}
	if _, err := New().Evaluate("OPEN_FIELD", 1, params, backwards, 1_000_000); !errors.Is(err, domain.ErrInvalidTrajectory) {
		t.Fatalf("samples out of order: %v", err)
	}
	if _, err := New().Evaluate("OPEN_FIELD", 1, map[string]float64{"unknown": 1}, tr, 1_000_000); err == nil || errors.Is(err, domain.ErrInvalidTrajectory) {
		t.Fatalf("an unknown pinned parameter is a server fault, not a worker fault: %v", err)
	}
}
