package domain

import (
	"errors"
	"math"
	"slices"
	"testing"
)

const second = int64(1_000_000)

func at(seconds float64) int64 { return int64(math.Round(seconds * 1_000_000)) }

func sample(t, x, y float64) Sample { return Sample{TUs: at(t), X: x, Y: y, Valid: true} }

func lost(t float64) Sample { return Sample{TUs: at(t), X: math.NaN(), Y: math.NaN()} }

func evaluate(t *testing.T, params map[string]float64, samples ...Sample) Result {
	t.Helper()
	r, err := Evaluate("OPEN_FIELD", 1, Input{Parameters: params, Samples: samples})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

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

// Arena 40 x 40 cm with center fraction 0.5: the center zone is [10, 30] x [10, 30].
var arena40 = map[string]float64{"arena_width_cm": 40, "arena_height_cm": 40, "center_fraction": 0.5}

// Hand calculation (pairs count when both samples are valid and 0 < dt <= 0.5 s):
//
//	pairs 0-0.5, 0.5-1, 1-1.5, 1.5-2, 2-2.5, 2.5-3 and 4-4.5 count (3.5 s); 3-3.5 and 3.5-4 touch a lost frame.
//	distance: 1-1.5 moves 10 cm, 1.5-2 moves 10 cm, the rest 0 cm = 20 cm; mean speed 20/3.5 = 5.714285714 cm/s.
//	zone time uses the earlier sample: periphery 0-2 (2.0 s), center 2-3 and 4-4.5 (1.5 s).
//	one periphery-to-center transition at 2.0 s.
//	immobile pairs (speed < 2 cm/s): 0-1 (bout 1.0 s), 2-3 (bout 1.0 s), 4-4.5 (0.5 s, shorter than 1 s, ignored) = 2.0 s.
func TestOpenFieldHandCalculatedTrajectory(t *testing.T) {
	r := evaluate(t, arena40,
		sample(0, 5, 5), sample(0.5, 5, 5), sample(1, 5, 5), sample(1.5, 15, 5), sample(2, 15, 15),
		sample(2.5, 15, 15), sample(3, 15, 15), lost(3.5), sample(4, 15, 15), sample(4.5, 15, 15),
	)
	if r.Paradigm != "OPEN_FIELD" || r.ParadigmVersion != 1 || r.MetricEngineVersion != MetricEngineVersion || r.ResultSchemaVersion != ResultSchemaVersion {
		t.Fatalf("versions: %+v", r)
	}
	expectValue(t, r, "duration_s", 3.5)
	expectValue(t, r, "distance_cm", 20)
	expectValue(t, r, "mean_speed_cm_s", 20.0/3.5)
	expectValue(t, r, "center_time_s", 1.5)
	expectValue(t, r, "periphery_time_s", 2.0)
	expectValue(t, r, "center_time_ratio", 1.5/3.5)
	expectValue(t, r, "periphery_time_ratio", 2.0/3.5)
	expectValue(t, r, "center_entries_count", 1)
	expectValue(t, r, "immobility_s", 2.0)
	wantIntervals := []Interval{
		{Type: "immobile", StartUs: 0, EndUs: 1 * second},
		{Type: "immobile", StartUs: 2 * second, EndUs: 3 * second},
		{Type: "in_center", StartUs: 2 * second, EndUs: 3 * second},
		{Type: "in_center", StartUs: 4 * second, EndUs: at(4.5)},
	}
	if !slices.Equal(r.Intervals, wantIntervals) {
		t.Errorf("intervals %+v\nwant %+v", r.Intervals, wantIntervals)
	}
	if !slices.Equal(r.Points, []Point{{Type: "center_entry", AtUs: 2 * second}}) {
		t.Errorf("points %+v", r.Points)
	}
}

// Center is a closed rectangle; samples outside the arena are treated as lost.
func TestZoneBoundariesAndArenaBounds(t *testing.T) {
	params := map[string]float64{"arena_width_cm": 40, "arena_height_cm": 40, "center_fraction": 0.5, "max_sample_gap_s": 5}
	r := evaluate(t, params,
		sample(0, 10, 10),     // on the center corner: center
		sample(1, 9.999, 10),  // just outside the center: periphery
		sample(2, 30, 30),     // opposite center corner: center
		sample(3, 30.001, 20), // periphery
		sample(4, 40, 40),     // arena corner: periphery
		sample(5, 40.001, 40), // outside the arena: lost
	)
	expectValue(t, r, "duration_s", 4)
	expectValue(t, r, "center_time_s", 2)
	expectValue(t, r, "periphery_time_s", 2)
	expectValue(t, r, "center_entries_count", 1)
	distance := math.Hypot(0.001, 0) + math.Hypot(20.001, 20) + math.Hypot(0.001, 10) + math.Hypot(9.999, 20)
	expectValue(t, r, "distance_cm", distance)
}

// Speed equal to the threshold is movement; bouts shorter than the minimum are not immobility.
func TestImmobilityBoutsAndEventDurations(t *testing.T) {
	params := map[string]float64{"arena_width_cm": 40, "arena_height_cm": 40, "center_fraction": 0.5, "immobility_threshold_cm_s": 2, "min_immobility_bout_s": 1}
	var samples []Sample
	for i := 0; i <= 5; i++ {
		samples = append(samples, sample(float64(i)*0.25, 5, 5)) // 0 to 1.25 s still: 1.25 s bout
	}
	samples = append(samples,
		sample(1.5, 10, 5),                                         // 20 cm/s
		sample(1.75, 10, 5), sample(2, 10, 5), sample(2.25, 10, 5), // 0.75 s still: too short
		sample(2.5, 10.5, 5), sample(2.75, 11, 5), // exactly 2 cm/s: moving
	)
	r := evaluate(t, params, samples...)
	expectValue(t, r, "duration_s", 2.75)
	expectValue(t, r, "immobility_s", 1.25)
	var immobile []Interval
	for _, i := range r.Intervals {
		if i.Type == "immobile" {
			immobile = append(immobile, i)
		}
	}
	if !slices.Equal(immobile, []Interval{{Type: "immobile", StartUs: 0, EndUs: at(1.25)}}) {
		t.Fatalf("immobile intervals %+v", immobile)
	}
}

func TestMissingDataIsNotZero(t *testing.T) {
	for name, samples := range map[string][]Sample{
		"no samples":        nil,
		"one valid sample":  {sample(0, 5, 5)},
		"all frames lost":   {lost(0), lost(0.5), lost(1)},
		"gaps too long":     {sample(0, 5, 5), sample(1, 5, 5), sample(2, 5, 5)},
		"valid around lost": {sample(0, 5, 5), lost(0.25), sample(0.5, 5, 5)},
	} {
		t.Run(name, func(t *testing.T) {
			r := evaluate(t, arena40, samples...)
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
		"unknown paradigm":    {"open_field", 1, Input{}, ErrUnknownParadigm},
		"unknown version":     {"OPEN_FIELD", 2, Input{}, ErrUnknownVersion},
		"unknown parameter":   {"OPEN_FIELD", 1, Input{Parameters: map[string]float64{"arena_radius_cm": 30}}, ErrUnknownParameter},
		"parameter too large": {"OPEN_FIELD", 1, Input{Parameters: map[string]float64{"center_fraction": 0.9}}, ErrParameterOutOfRange},
		"parameter NaN":       {"OPEN_FIELD", 1, Input{Parameters: map[string]float64{"arena_width_cm": math.NaN()}}, ErrParameterOutOfRange},
		"valid NaN x":         {"OPEN_FIELD", 1, Input{Samples: []Sample{{TUs: 0, X: math.NaN(), Y: 1, Valid: true}}}, ErrInvalidSample},
		"valid infinite y":    {"OPEN_FIELD", 1, Input{Samples: []Sample{{TUs: 0, X: 1, Y: math.Inf(1), Valid: true}}}, ErrInvalidSample},
		"negative time":       {"OPEN_FIELD", 1, Input{Samples: []Sample{{TUs: -1, X: 1, Y: 1, Valid: true}}}, ErrInvalidSample},
		"repeated time":       {"OPEN_FIELD", 1, Input{Samples: []Sample{sample(1, 1, 1), sample(1, 2, 2)}}, ErrInvalidSample},
		"decreasing time":     {"OPEN_FIELD", 1, Input{Samples: []Sample{sample(2, 1, 1), lost(1)}}, ErrInvalidSample},
	}
	for name, tc := range cases {
		if _, err := Evaluate(tc.key, tc.version, tc.input); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	if _, err := Evaluate("OPEN_FIELD", 1, Input{Samples: []Sample{lost(0), sample(0.5, 1, 1)}}); err != nil {
		t.Errorf("lost frames may carry NaN coordinates: %v", err)
	}
}
