package catalog

import (
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"math"
	"slices"
	"testing"
)

func TestContractsComeFromThePublishedParadigmVersion(t *testing.T) {
	c, err := New().Contract("MWM", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !c.CalibrationRequired || !c.TrajectoryAnalyzable || c.ObserverScored || c.MetricEngineVersion != 1 || c.ResultSchemaVersion != 1 || len(c.Metrics) != 8 || len(c.Events) != 2 {
		t.Fatalf("MWM contract: %+v", c)
	}
	crossings := c.Metrics[slices.IndexFunc(c.Metrics, func(m domain.MetricSpec) bool { return m.Key == "platform_crossings_count" })]
	if !crossings.Integer || crossings.Unit != "count" {
		t.Fatalf("integer metric: %+v", crossings)
	}
	if !slices.Contains(c.MissingReasons, "NO_VALID_INTERVALS") || !slices.ContainsFunc(c.Events, func(e domain.EventSpec) bool { return e.Type == "platform_entry" && e.Kind == "POINT" }) {
		t.Fatalf("reasons and events: %+v", c)
	}
	if !slices.Contains(c.QC, domain.QCRule{Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.1}) {
		t.Fatalf("QC rules: %+v", c.QC)
	}
	// Scored observations cannot come from a trajectory and need no calibration.
	for _, key := range []string{"NOVEL_OBJECT", "POLE", "TREADMILL"} {
		other, err := New().Contract(key, 1)
		if err != nil || other.TrajectoryAnalyzable || !other.ObserverScored || other.CalibrationRequired {
			t.Errorf("%s: %+v %v", key, other, err)
		}
	}
	for _, tc := range []struct {
		key     string
		version int
	}{{"MAZE", 1}, {"MWM", 9}} {
		if _, err := New().Contract(tc.key, tc.version); !errors.Is(err, application.ErrUnknownCapability) {
			t.Errorf("%s v%d: %v", tc.key, tc.version, err)
		}
	}
}

func TestTheEngineComputesMetricsFromTheTrajectory(t *testing.T) {
	params := map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30, "max_sample_gap_s": 0.5}
	// Onto the platform edge and its center in two 0.1 s steps of 5 cm, then a lost sample.
	tr := domain.Track{Samples: []domain.Sample{
		{TUs: 0, X: 20, Y: 30, Tracked: true, Confidence: 1},
		{TUs: 100_000, X: 25, Y: 30, Tracked: true, Confidence: 1},
		{TUs: 200_000, X: 30, Y: 30, Tracked: true, Confidence: 1},
		{TUs: 300_000},
	}}
	c, err := New().Evaluate("MWM", 1, params, tr, nil, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	metric := func(key string) domain.MetricValue {
		return c.Metrics[slices.IndexFunc(c.Metrics, func(m domain.MetricValue) bool { return m.Key == key })]
	}
	if d := metric("path_length_cm"); d.Value == nil || *d.Value != 10 || d.Unit != "cm" || d.MissingReason != "" || len(c.Metrics) != 8 {
		t.Fatalf("path length: %+v", c.Metrics)
	}
	if e := metric("platform_crossings_count"); *e.Value != 1 || len(c.Points) != 1 || c.Points[0] != (domain.Point{Type: "platform_entry", AtUs: 100_000}) {
		t.Fatalf("crossings: %+v %+v", e, c.Points)
	}
	if l := metric("escape_latency_s"); l.Value != nil || l.MissingReason != "EVENT_NOT_OBSERVED" {
		t.Fatalf("a 0.1 s touch is not an escape: %+v", l)
	}

	empty, err := New().Evaluate("MWM", 1, params, domain.Track{}, nil, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range empty.Metrics {
		if m.Value != nil || m.MissingReason != "NO_VALID_INTERVALS" {
			t.Fatalf("a metric without counted intervals must be missing: %+v", m)
		}
	}
	backwards := domain.Track{Samples: []domain.Sample{{TUs: 5}, {TUs: 1}}}
	if _, err := New().Evaluate("MWM", 1, params, backwards, nil, 1_000_000); !errors.Is(err, domain.ErrInvalidTrajectory) {
		t.Fatalf("samples out of order: %v", err)
	}
	if _, err := New().Evaluate("MWM", 1, map[string]float64{"unknown": 1}, tr, nil, 1_000_000); err == nil || errors.Is(err, domain.ErrInvalidTrajectory) {
		t.Fatalf("an unknown pinned parameter is a server fault, not a worker fault: %v", err)
	}
}

// No published paradigm declares calibrated zones: the water maze derives its
// zones from parameters, and scored-only paradigms are not analysed from a
// trajectory at all. Zone shapes the paradigm does not declare are refused.
func TestCalibratedZonesComeFromTheCalibration(t *testing.T) {
	for key, analyzable := range map[string]bool{"MWM": true, "NOVEL_OBJECT": false, "POLE": false, "TREADMILL": false} {
		c, err := New().Contract(key, 1)
		if err != nil || c.TrajectoryAnalyzable != analyzable || len(c.RequiredZones) != 0 {
			t.Errorf("%s: %+v %v", key, c, err)
		}
	}
	params := map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}
	tr := domain.Track{}
	for i := range 4 {
		tr.Samples = append(tr.Samples, domain.Sample{TUs: int64(i) * 100_000, X: 5 + float64(i)*5, Y: 5, Tracked: true, Confidence: 1})
	}
	c, err := New().Evaluate("MWM", 1, params, tr, nil, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range c.Metrics {
		if m.Key == "duration_s" && (m.Value == nil || math.Abs(*m.Value-0.3) > 1e-9) {
			t.Fatalf("duration: %+v", m)
		}
	}
	square := domain.Shape{Polygon: []domain.Vertex{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 10}, {X: 0, Y: 10}}}
	if _, err := New().Evaluate("MWM", 1, params, tr, []domain.Zone{{Key: "platform", Centimeters: square}}, 1_000_000); err == nil {
		t.Fatal("a zone shape the water maze does not declare must not be evaluated")
	}
}
