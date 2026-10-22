package domain

import (
	"errors"
	"maps"
	"math"
	"slices"
	"testing"
)

const second = int64(1_000_000)

func at(seconds float64) int64 { return int64(math.Round(seconds * 1_000_000)) }

func sample(t, x, y float64) Sample { return Sample{TUs: at(t), X: x, Y: y, Valid: true} }

func lost(t float64) Sample { return Sample{TUs: at(t), X: math.NaN(), Y: math.NaN()} }

func expectValue(t *testing.T, r Result, key string, want float64) {
	t.Helper()
	m, ok := r.Metric(key)
	if !ok {
		t.Fatalf("%s not produced", key)
	}
	def, _ := metricDefinition(r, key)
	if m.Missing || math.Abs(m.Value-want) > def.Tolerance {
		t.Errorf("%s = %v (missing=%v %s), want %v within %v", key, m.Value, m.Missing, m.Reason, want, def.Tolerance)
	}
}

func metricDefinition(r Result, key string) (MetricDefinition, bool) {
	p, err := Version(r.Paradigm, r.ParadigmVersion)
	if err != nil {
		return MetricDefinition{}, false
	}
	for _, m := range p.Metrics {
		if m.Key == key {
			return m, true
		}
	}
	return MetricDefinition{}, false
}

// tank is a 120 cm pool with a 10 cm platform at (30, 30) and the wall zone from r = 48 cm.
var tank = map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30, "wall_annulus_width_cm": 12, "min_platform_dwell_s": 1}

// Hand calculation (pairs count when both samples are tracked and 0 < dt <= 0.5 s):
//
//	pairs 0-0.5, 1.5-2, 2-2.5, 2.5-3, 3-3.5 and 3.5-4 count (3.0 s); 0.5-1 and 1-1.5 touch a lost frame.
//	reaching the platform at 1.5 s across the lost frame is not an entry; leaving at 2.5 s and
//	coming back at 3.0 s is, so there is one crossing and one platform_entry point at 3.0 s.
//	the first dwell (1.5 to 2.0 s) is too short; the second (3.0 to 4.0 s) is the escape at 3.0 s.
//	path up to the escape: 10 + 2 + 10 + 10 = 32 cm over 2.0 s of counted time.
//	occupancy uses the earlier sample: platform 1.5-2.5 and 3.0-4.0 s; wall zone only (40, 30), r = 50: 0.5 s.
func TestTrajectoryCountsPairsAcrossLostFrames(t *testing.T) {
	r := run(t, "MWM", Input{Parameters: tank, Samples: []Sample{
		sample(0, 0, 0), sample(0.5, 10, 0), lost(1), sample(1.5, 30, 28), sample(2, 30, 30),
		sample(2.5, 40, 30), sample(3, 30, 30), sample(3.5, 30, 31), sample(4, 30, 30),
	}})
	if r.Paradigm != "MWM" || r.ParadigmVersion != 1 || r.MetricEngineVersion != MetricEngineVersion || r.ResultSchemaVersion != ResultSchemaVersion {
		t.Fatalf("versions: %+v", r)
	}
	expectValue(t, r, "duration_s", 3)
	expectValue(t, r, "escape_latency_s", 3)
	expectValue(t, r, "path_length_cm", 32)
	expectValue(t, r, "mean_swim_speed_cm_s", 16)
	expectValue(t, r, "platform_crossings_count", 1)
	expectValue(t, r, "target_quadrant_time_ratio", 1)
	expectValue(t, r, "thigmotaxis_time_ratio", 0.5/3)
	expectValue(t, r, "mean_distance_to_platform_cm", (math.Hypot(30, 30)+2+0+10+0+1)*0.5/3)
	wantIntervals := []Interval{
		{Type: "in_platform", StartUs: at(1.5), EndUs: at(2.5)},
		{Type: "in_platform", StartUs: 3 * second, EndUs: 4 * second},
	}
	if !slices.Equal(r.Intervals, wantIntervals) {
		t.Errorf("intervals %+v\nwant %+v", r.Intervals, wantIntervals)
	}
	if !slices.Equal(r.Points, []Point{{Type: "platform_entry", AtUs: 3 * second}}) {
		t.Errorf("points %+v", r.Points)
	}
}

// The platform is a closed disk and the pool a closed circle: a sample on
// either boundary is inside, a sample just beyond the wall is lost.
func TestPlatformAndPoolIncludeTheirBoundary(t *testing.T) {
	r := run(t, "MWM", Input{Parameters: tank, Samples: []Sample{
		sample(0, 0, 0), sample(0.5, 35, 30), sample(1, 60, 0), sample(1.5, 59, 0), sample(2, 60.001, 0), sample(2.5, 0, 0),
	}})
	expectValue(t, r, "duration_s", 1.5)
	expectValue(t, r, "platform_crossings_count", 1)
	expectValue(t, r, "thigmotaxis_time_ratio", 1.0/3)
	expectValue(t, r, "path_length_cm", math.Hypot(35, 30)+math.Hypot(25, 30)+1)
	expectMissing(t, r, "escape_latency_s", ReasonNotObserved)
	if !slices.Equal(r.Points, []Point{{Type: "platform_entry", AtUs: at(0.5)}}) {
		t.Errorf("points %+v", r.Points)
	}
}

func TestMissingDataIsNotZero(t *testing.T) {
	for name, samples := range map[string][]Sample{
		"no samples":        nil,
		"one valid sample":  {sample(0, 5, 5)},
		"all frames lost":   {lost(0), lost(0.5), lost(1)},
		"gaps too long":     {sample(0, 5, 5), sample(1, 5, 5), sample(2, 5, 5)},
		"valid around lost": {sample(0, 5, 5), lost(0.25), sample(0.5, 5, 5)},
		"outside the pool":  {sample(0, 70, 0), sample(0.5, 0, 70)},
	} {
		t.Run(name, func(t *testing.T) {
			r := run(t, "MWM", Input{Parameters: tank, Samples: samples})
			if len(r.Metrics) == 0 {
				t.Fatal("metrics omitted instead of reported missing")
			}
			for _, m := range r.Metrics {
				if !m.Missing || m.Reason != ReasonNoValidIntervals || m.Value != 0 {
					t.Errorf("%s: %+v", m.Key, m)
				}
			}
			if len(r.Intervals) != 0 || len(r.Points) != 0 {
				t.Errorf("events derived from missing data: %+v %+v", r.Intervals, r.Points)
			}
		})
	}
}

func TestInvalidInputsAreRejected(t *testing.T) {
	cases := map[string]struct {
		key     string
		version int
		input   Input
		want    error
	}{
		"unknown paradigm":    {"mwm", 1, Input{}, ErrUnknownParadigm},
		"unknown version":     {"MWM", 2, Input{}, ErrUnknownVersion},
		"unknown parameter":   {"MWM", 1, Input{Parameters: map[string]float64{"arena_radius_cm": 30}}, ErrUnknownParameter},
		"parameter too large": {"MWM", 1, Input{Parameters: map[string]float64{"min_platform_dwell_s": 11}}, ErrParameterOutOfRange},
		"parameter NaN":       {"MWM", 1, Input{Parameters: map[string]float64{"tank_diameter_cm": math.NaN()}}, ErrParameterOutOfRange},
		"fractional integer":  {"TREADMILL", 1, Input{Parameters: map[string]float64{"accelerating": 0.5}}, ErrParameterOutOfRange},
		"valid NaN x":         {"MWM", 1, Input{Samples: []Sample{{TUs: 0, X: math.NaN(), Y: 1, Valid: true}}}, ErrInvalidSample},
		"valid infinite y":    {"MWM", 1, Input{Samples: []Sample{{TUs: 0, X: 1, Y: math.Inf(1), Valid: true}}}, ErrInvalidSample},
		"negative time":       {"MWM", 1, Input{Samples: []Sample{{TUs: -1, X: 1, Y: 1, Valid: true}}}, ErrInvalidSample},
		"repeated time":       {"MWM", 1, Input{Samples: []Sample{sample(1, 1, 1), sample(1, 2, 2)}}, ErrInvalidSample},
		"decreasing time":     {"MWM", 1, Input{Samples: []Sample{sample(2, 1, 1), lost(1)}}, ErrInvalidSample},
	}
	for name, tc := range cases {
		if _, err := Evaluate(tc.key, tc.version, tc.input); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	if _, err := Evaluate("MWM", 1, Input{Samples: []Sample{lost(0), sample(0.5, 1, 1)}}); err != nil {
		t.Errorf("lost frames may carry NaN coordinates: %v", err)
	}
}

// The water maze derives its zones from parameters, so a drawn shape is
// unknown; the other paradigms need every calibrated zone they declare. The
// generic rules hold for any version that declares calibrated zones.
func TestCalibratedZonesAreValidated(t *testing.T) {
	expectError(t, "MWM", Input{Zones: map[string][]Shape{"platform": {rect(0, 0, 1, 1)}}}, ErrUnknownZone)
	for key, zones := range map[string]map[string][]Shape{"NOVEL_OBJECT": objectZones, "POLE": poleZones, "TREADMILL": gridZones} {
		p, _ := Latest(key)
		for _, z := range p.Zones {
			if !z.Calibrated || !z.Required {
				t.Errorf("%s zone %s must be calibrated and required", key, z.Key)
			}
			partial := maps.Clone(zones)
			delete(partial, z.Key)
			expectError(t, key, Input{Zones: partial}, ErrMissingZone)
		}
		extra := maps.Clone(zones)
		extra["platform"] = []Shape{circle(0, 0, 1)}
		expectError(t, key, Input{Zones: extra}, ErrUnknownZone)
		if _, err := Evaluate(key, 1, Input{Zones: zones}); err != nil {
			t.Errorf("%s with its zones: %v", key, err)
		}
	}
	zoned := Paradigm{Zones: []Zone{
		{Key: "arms", Role: "OPEN", Calibrated: true, Required: true},
		{Key: "corner", Role: "CENTER", Calibrated: true},
		{Key: "center", Role: "CENTER"},
	}}
	valid := func(mutate func(map[string][]Shape)) map[string][]Shape {
		z := map[string][]Shape{"arms": {rect(0, 0, 10, 2), circle(5, 5, 1)}}
		mutate(z)
		return z
	}
	for name, tc := range map[string]struct {
		zones map[string][]Shape
		want  error
	}{
		"required only":         {valid(func(map[string][]Shape) {}), nil},
		"optional zone drawn":   {valid(func(z map[string][]Shape) { z["corner"] = []Shape{circle(1, 1, 1)} }), nil},
		"missing required zone": {valid(func(z map[string][]Shape) { delete(z, "arms") }), ErrMissingZone},
		"empty shape list":      {valid(func(z map[string][]Shape) { z["arms"] = []Shape{} }), ErrMissingZone},
		"derived zone":          {valid(func(z map[string][]Shape) { z["center"] = []Shape{rect(0, 0, 1, 1)} }), ErrUnknownZone},
		"undeclared zone":       {valid(func(z map[string][]Shape) { z["tail"] = []Shape{rect(0, 0, 1, 1)} }), ErrUnknownZone},
		"two-vertex polygon":    {valid(func(z map[string][]Shape) { z["arms"] = []Shape{{Polygon: []Vertex{{0, 0}, {1, 1}}}} }), ErrInvalidZone},
		"NaN vertex":            {valid(func(z map[string][]Shape) { z["arms"] = []Shape{rect(0, 0, math.NaN(), 1)} }), ErrInvalidZone},
		"zero radius":           {valid(func(z map[string][]Shape) { z["arms"] = []Shape{circle(0, 0, 0)} }), ErrInvalidZone},
		"infinite radius":       {valid(func(z map[string][]Shape) { z["arms"] = []Shape{circle(0, 0, math.Inf(1))} }), ErrInvalidZone},
		"circle and polygon together": {valid(func(z map[string][]Shape) {
			z["arms"] = []Shape{{Circle: &Circle{Radius: 1}, Polygon: rect(0, 0, 1, 1).Polygon}}
		}), ErrInvalidZone},
		"neither circle nor polygon": {valid(func(z map[string][]Shape) { z["arms"] = []Shape{{}} }), ErrInvalidZone},
	} {
		if err := validateZones(zoned, tc.zones); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}
