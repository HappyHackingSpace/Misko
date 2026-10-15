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
	// Scored observations cannot come from a trajectory.
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
	c, err := New().Evaluate("OPEN_FIELD", 1, params, tr, nil, 1_000_000)
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

	empty, err := New().Evaluate("OPEN_FIELD", 1, params, domain.Track{}, nil, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range empty.Metrics {
		if m.Value != nil || m.MissingReason != "NO_VALID_INTERVALS" {
			t.Fatalf("a metric without counted intervals must be missing: %+v", m)
		}
	}
	backwards := domain.Track{Samples: []domain.Sample{{TUs: 5}, {TUs: 1}}}
	if _, err := New().Evaluate("OPEN_FIELD", 1, params, backwards, nil, 1_000_000); !errors.Is(err, domain.ErrInvalidTrajectory) {
		t.Fatalf("samples out of order: %v", err)
	}
	if _, err := New().Evaluate("OPEN_FIELD", 1, map[string]float64{"unknown": 1}, tr, nil, 1_000_000); err == nil || errors.Is(err, domain.ErrInvalidTrajectory) {
		t.Fatalf("an unknown pinned parameter is a server fault, not a worker fault: %v", err)
	}
}

// Tracked paradigms with calibrated zones are analysed from a trajectory and
// the zone shapes of the pinned calibration; scored-only paradigms are not.
func TestCalibratedZonesComeFromTheCalibration(t *testing.T) {
	for key, zones := range map[string][]string{
		"EPM":           {"center", "open_arms", "closed_arms"},
		"Y_MAZE":        {"arm_a", "arm_b", "arm_c"},
		"BARNES_MAZE":   {"target_hole", "holes"},
		"THREE_CHAMBER": {"social_chamber", "object_chamber", "center_chamber"},
		"MWM":           nil,
		"LIGHT_DARK":    nil,
	} {
		c, err := New().Contract(key, 1)
		if err != nil || !c.TrajectoryAnalyzable || !slices.Equal(c.RequiredZones, zones) {
			t.Errorf("%s: %+v %v", key, c, err)
		}
	}
	for _, key := range []string{"ROTAROD", "POLE", "TREADMILL", "NOVEL_OBJECT"} {
		if c, err := New().Contract(key, 1); err != nil || c.TrajectoryAnalyzable {
			t.Errorf("%s: %+v %v", key, c, err)
		}
	}

	// A 10 cm walk along the open arm (x from 5 to 15), then into the center.
	square := func(x0, y0, x1, y1 float64) domain.Shape {
		return domain.Shape{Polygon: []domain.Vertex{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}}
	}
	zones := []domain.Zone{
		{Key: "open_arms", Centimeters: square(0, 0, 20, 10)},
		{Key: "open_arms", Centimeters: square(30, 0, 50, 10)},
		{Key: "center", Centimeters: square(20, 0, 30, 10)},
		{Key: "closed_arms", Centimeters: domain.Shape{Circle: &domain.Circle{X: 25, Y: 30, Radius: 5}}},
	}
	tr := domain.Track{}
	for i := range 4 {
		tr.Samples = append(tr.Samples, domain.Sample{TUs: int64(i) * 100_000, X: 5 + float64(i)*5, Y: 5, Tracked: true, Confidence: 1})
	}
	tr.Samples = append(tr.Samples, domain.Sample{TUs: 400_000, X: 25, Y: 5, Tracked: true, Confidence: 1})
	c, err := New().Evaluate("EPM", 1, nil, tr, zones, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]domain.MetricValue{}
	for _, m := range c.Metrics {
		values[m.Key] = m
	}
	if v := values["open_arm_time_s"]; v.Value == nil || math.Abs(*v.Value-0.4) > 1e-9 {
		t.Fatalf("open arm time: %+v", c.Metrics)
	}
	if _, err := New().Evaluate("EPM", 1, nil, tr, zones[:1], 1_000_000); err == nil {
		t.Fatal("EPM without its center and closed arms must not be evaluated")
	}
}
