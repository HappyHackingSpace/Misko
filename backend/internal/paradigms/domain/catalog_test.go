package domain

import (
	"errors"
	"math"
	"slices"
	"testing"
)

var elevenKeys = []string{"BARNES_MAZE", "EPM", "LIGHT_DARK", "MWM", "NOVEL_OBJECT", "OPEN_FIELD", "POLE", "ROTAROD", "THREE_CHAMBER", "TREADMILL", "Y_MAZE"}

// minimalInputs are valid inputs that produce every metric of each paradigm.
func minimalInputs() map[string]Input {
	return map[string]Input{
		"BARNES_MAZE": {Zones: barnesZones(), Samples: []Sample{sample(0, 0, 0), sample(0.5, 38, 0), sample(1, 0, 38)}},
		"EPM": {Zones: epmZones(), ScoredEvents: []string{"risk_assessment"}, Events: []ObservedEvent{mark("risk_assessment", 0.2)},
			Samples: []Sample{sample(0, 0, 0), sample(0.5, 10, 0), sample(1, 0, 10)}},
		"LIGHT_DARK":   {Samples: []Sample{sample(0, 5, 10), sample(0.5, 30, 10), sample(1, 10, 10)}},
		"MWM":          {Samples: []Sample{sample(0, 0, 0), sample(0.5, 30, 30), sample(1, 30, 31), sample(1.5, 30, 30)}},
		"NOVEL_OBJECT": {ScoredEvents: []string{"exploration"}, Events: []ObservedEvent{span("exploration", "novel", 1, 3), span("exploration", "familiar", 4, 5)}},
		"OPEN_FIELD":   {Parameters: arena40, Samples: []Sample{sample(0, 5, 5), sample(0.5, 15, 15), sample(1, 15, 15), sample(1.5, 15, 15)}},
		"POLE": {ScoredEvents: []string{"turn_complete", "base_reached", "fall"},
			Events: []ObservedEvent{mark("turn_complete", 1), mark("base_reached", 4)}},
		"ROTAROD": {ScoredEvents: []string{"fall"}, Events: []ObservedEvent{mark("fall", 100)}, DurationUs: at(300)},
		"THREE_CHAMBER": {Zones: func() map[string][]Shape {
			z := threeChamberZones()
			z["object_interaction"] = []Shape{circle(50, 20, 5)}
			return z
		}(), Samples: []Sample{sample(0, 30, 20), sample(0.5, 10, 20), sample(1, 50, 20), sample(1.5, 51, 20)}},
		"TREADMILL": {ScoredEvents: []string{"exhaustion", "shock"}, Events: []ObservedEvent{mark("shock", 10), mark("exhaustion", 60)}},
		"Y_MAZE": {Zones: yMazeZones(), Parameters: map[string]float64{"novel_arm": 1},
			Samples: []Sample{sample(0, 5, 5), sample(0.5, 25, 5), sample(1, 45, 5), sample(1.5, 5, 5)}},
	}
}

func TestCatalogReturnsCopies(t *testing.T) {
	p, err := Latest("EPM")
	if err != nil {
		t.Fatal(err)
	}
	p.Species[0] = "zebrafish"
	p.Metrics[0].Key = "hacked"
	p.Metrics[0].Inputs[0] = "hacked"
	p.ApparatusParameters[0].Max = -1
	p.Zones[0].Key = "hacked"
	p.InputEvents[0].Type = "hacked"
	all := All()
	all[0].Name = "hacked"
	all[0].Zones = nil
	result, _ := Evaluate("OPEN_FIELD", 1, Input{})
	result.Parameters["arena_width_cm"] = -1

	fresh, err := Version("EPM", 1)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Species[0] == "zebrafish" || fresh.Metrics[0].Key == "hacked" || fresh.Metrics[0].Inputs[0] == "hacked" ||
		fresh.ApparatusParameters[0].Max == -1 || fresh.Zones[0].Key == "hacked" || fresh.InputEvents[0].Type == "hacked" {
		t.Fatal("caller mutated the registry")
	}
	if again := All(); again[0].Name == "hacked" || len(again[0].Zones) == 0 {
		t.Fatal("caller mutated All()")
	}
	if again, _ := Evaluate("OPEN_FIELD", 1, Input{}); again.Parameters["arena_width_cm"] != 50 {
		t.Fatal("caller mutated default parameters")
	}
}

func TestAllElevenParadigmsArePublished(t *testing.T) {
	if !slices.Equal(Keys(), elevenKeys) {
		t.Fatalf("keys %v", Keys())
	}
	for _, key := range elevenKeys {
		versions, err := Versions(key)
		latest, latestErr := Latest(key)
		if err != nil || latestErr != nil || !slices.Equal(versions, []int{1}) || latest.Version != 1 || latest.Key != key {
			t.Errorf("%s: versions %v %v latest %+v %v", key, versions, err, latest.Version, latestErr)
		}
	}
	if _, err := Versions("MAZE"); !errors.Is(err, ErrUnknownParadigm) {
		t.Fatalf("unknown key: %v", err)
	}
	if MetricEngineVersion < 1 || ResultSchemaVersion < 1 {
		t.Fatal("engine and result schema versions must be published")
	}
}

// Every advertised metric is fully specified and produced by the engine, the
// engine produces nothing undeclared, and derived events match declared kinds.
func TestEveryContractMatchesItsEngine(t *testing.T) {
	units := []Unit{Centimeter, CentimeterPerSecond, Second, Count, Ratio, Rpm, Boolean}
	inputs := minimalInputs()
	for _, p := range All() {
		t.Run(p.Key, func(t *testing.T) {
			if len(p.Metrics) == 0 || p.Name == "" || p.Category == "" || len(p.Species) == 0 || len(p.TrialTypes) == 0 {
				t.Errorf("incomplete paradigm %+v", p)
			}
			declared := map[string]bool{}
			for _, m := range p.Metrics {
				declared[m.Key] = true
				if m.Label == "" || m.Definition == "" || m.Formula == "" || m.MissingWhen == "" || len(m.Inputs) == 0 ||
					!slices.Contains(units, m.Unit) || m.Min > m.Max || m.Tolerance <= 0 {
					t.Errorf("incomplete metric %+v", m)
				}
			}
			for _, param := range append(p.ApparatusParameters, p.SessionParameters...) {
				if param.Label == "" || param.Default < param.Min || param.Default > param.Max || !slices.Contains(units, param.Unit) ||
					(param.Integer && param.Default != math.Trunc(param.Default)) {
					t.Errorf("parameter %+v", param)
				}
			}
			for _, z := range p.Zones {
				if z.Key == "" || z.Role == "" || z.Geometry == "" {
					t.Errorf("zone %+v", z)
				}
			}
			kinds := map[string]EventKind{}
			for _, e := range append(slices.Clone(p.Events), p.InputEvents...) {
				if e.Definition == "" || (e.Kind != IntervalEvent && e.Kind != PointEvent) {
					t.Errorf("event %+v", e)
				}
				kinds[e.Type] = e.Kind
			}
			in, ok := inputs[p.Key]
			if !ok {
				t.Fatal("no minimal input")
			}
			r := run(t, p.Key, in)
			if len(r.Metrics) != len(p.Metrics) {
				t.Errorf("engine produced %d metrics, catalog declares %d", len(r.Metrics), len(p.Metrics))
			}
			for _, m := range r.Metrics {
				if !declared[m.Key] {
					t.Errorf("undeclared metric %s", m.Key)
				}
				if m.Missing {
					t.Errorf("minimal input left %s missing (%s)", m.Key, m.Reason)
				}
			}
			outputs := map[string]EventKind{}
			for _, e := range p.Events {
				outputs[e.Type] = e.Kind
			}
			for _, i := range r.Intervals {
				if outputs[i.Type] != IntervalEvent {
					t.Errorf("interval %s not declared", i.Type)
				}
			}
			for _, pt := range r.Points {
				if outputs[pt.Type] != PointEvent {
					t.Errorf("point %s not declared", pt.Type)
				}
			}
		})
	}
}
