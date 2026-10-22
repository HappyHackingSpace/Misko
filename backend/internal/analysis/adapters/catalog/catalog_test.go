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
	if !c.CalibrationRequired || !c.TrajectoryAnalyzable || c.MetricEngineVersion != 1 || c.ResultSchemaVersion != 1 || len(c.Metrics) != 8 || len(c.Events) != 2 {
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
	// Every paradigm is analysed from a calibrated trajectory.
	for key, events := range map[string][]domain.EventSpec{
		"NOVEL_OBJECT": {{Type: "exploration", Kind: "INTERVAL"}},
		"POLE":         {{Type: "turn_complete", Kind: "POINT"}, {Type: "base_reached", Kind: "POINT"}, {Type: "fall", Kind: "POINT"}},
		"TREADMILL":    {{Type: "exhaustion", Kind: "POINT"}, {Type: "shock", Kind: "POINT"}},
	} {
		other, err := New().Contract(key, 1)
		if err != nil || !other.TrajectoryAnalyzable || !other.CalibrationRequired || !slices.Equal(other.Events, events) ||
			slices.Contains(other.MissingReasons, "NOT_SCORED") {
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

// The water maze derives its zones from parameters; the other paradigms need
// the zones drawn with their calibration, and the engine evaluates the
// centimetre shapes of the pinned calibration. Zone shapes the paradigm does
// not declare are refused.
func TestCalibratedZonesComeFromTheCalibration(t *testing.T) {
	for key, zones := range map[string][]string{"MWM": nil, "NOVEL_OBJECT": {"novel_object", "familiar_object"}, "POLE": {"pole", "base"}, "TREADMILL": {"shock_grid"}} {
		c, err := New().Contract(key, 1)
		if err != nil || !c.TrajectoryAnalyzable || !slices.Equal(c.RequiredZones, zones) {
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

	// Novel object at (10, 10) r = 4 and a familiar square: 0.0-0.4 s at the
	// novel object, out at 0.5 s; the bout [0, 0.5) carries its label.
	novel := domain.Shape{Circle: &domain.Circle{X: 10, Y: 10, Radius: 4}}
	familiar := domain.Shape{Polygon: []domain.Vertex{{X: 36, Y: 6}, {X: 44, Y: 6}, {X: 44, Y: 14}, {X: 36, Y: 14}}}
	objects := []domain.Zone{{Key: "novel_object", Centimeters: novel}, {Key: "familiar_object", Centimeters: familiar}}
	track := domain.Track{}
	for i := range 6 {
		x := 10.0
		if i == 5 {
			x = 30
		}
		track.Samples = append(track.Samples, domain.Sample{TUs: int64(i) * 100_000, X: x, Y: 10, Tracked: true, Confidence: 0.9})
	}
	nor, err := New().Evaluate("NOVEL_OBJECT", 1, map[string]float64{"arena_width_cm": 50, "arena_height_cm": 50}, track, objects, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(nor.Intervals) != 1 || nor.Intervals[0] != (domain.Interval{Type: "exploration", Label: "novel", StartUs: 0, EndUs: 500_000}) {
		t.Fatalf("exploration: %+v", nor.Intervals)
	}
	if events := track.Events(nor); len(events) != 1 || events[0].Label != "novel" || events[0].Kind != "INTERVAL" {
		t.Fatalf("published events: %+v", events)
	}
	if _, err := New().Evaluate("NOVEL_OBJECT", 1, map[string]float64{"arena_width_cm": 50, "arena_height_cm": 50}, track, objects[:1], 1_000_000); err == nil {
		t.Fatal("a missing required zone must not be evaluated")
	}
}
