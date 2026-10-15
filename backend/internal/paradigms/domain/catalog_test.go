package domain

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
)

var fourKeys = []string{"MWM", "NOVEL_OBJECT", "POLE", "TREADMILL"}

// minimalInputs are valid inputs that produce every metric of each paradigm.
func minimalInputs() map[string]Input {
	return map[string]Input{
		"MWM":          {Samples: []Sample{sample(0, 0, 0), sample(0.5, 30, 30), sample(1, 30, 31), sample(1.5, 30, 30)}},
		"NOVEL_OBJECT": {ScoredEvents: []string{"exploration"}, Events: []ObservedEvent{span("exploration", "novel", 1, 3), span("exploration", "familiar", 4, 5)}},
		"POLE": {ScoredEvents: []string{"turn_complete", "base_reached", "fall"},
			Events: []ObservedEvent{mark("turn_complete", 1), mark("base_reached", 4)}},
		"TREADMILL": {ScoredEvents: []string{"exhaustion", "shock"}, Events: []ObservedEvent{mark("shock", 10), mark("exhaustion", 60)}},
	}
}

func TestCatalogReturnsCopies(t *testing.T) {
	p, err := Latest("MWM")
	if err != nil {
		t.Fatal(err)
	}
	p.Species[0] = "zebrafish"
	p.Metrics[0].Key = "hacked"
	p.Metrics[0].Inputs[0] = "hacked"
	p.Parameters[0].Max = -1
	p.TrialTypes[0].Key = "hacked"
	p.Zones[0].Key = "hacked"
	scored, err := Latest("NOVEL_OBJECT")
	if err != nil {
		t.Fatal(err)
	}
	scored.InputEvents[0].Type = "hacked"
	scored.InputEvents[0].Labels[0] = "hacked"
	all := All()
	all[0].Name.EN = "hacked"
	all[0].Zones = nil
	result, _ := Evaluate("MWM", 1, Input{})
	result.Parameters["tank_diameter_cm"] = -1

	fresh, err := Version("MWM", 1)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Species[0] == "zebrafish" || fresh.Metrics[0].Key == "hacked" || fresh.Metrics[0].Inputs[0] == "hacked" ||
		fresh.Parameters[0].Max == -1 || fresh.TrialTypes[0].Key == "hacked" || fresh.Zones[0].Key == "hacked" {
		t.Fatal("caller mutated the registry")
	}
	if fresh, _ := Version("NOVEL_OBJECT", 1); fresh.InputEvents[0].Type == "hacked" || fresh.InputEvents[0].Labels[0] == "hacked" {
		t.Fatal("caller mutated the observed events")
	}
	if again := All(); again[0].Name.EN == "hacked" || len(again[0].Zones) == 0 {
		t.Fatal("caller mutated All()")
	}
	if again, _ := Evaluate("MWM", 1, Input{}); again.Parameters["tank_diameter_cm"] != 120 {
		t.Fatal("caller mutated default parameters")
	}
}

func TestAllFourParadigmsArePublished(t *testing.T) {
	if !slices.Equal(Keys(), fourKeys) {
		t.Fatalf("keys %v", Keys())
	}
	for _, key := range fourKeys {
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
	units := []Unit{Centimeter, CentimeterPerSecond, Second, Count, Ratio, Boolean}
	inputs := minimalInputs()
	for _, p := range All() {
		t.Run(p.Key, func(t *testing.T) {
			if len(p.Metrics) == 0 || p.Name.EN == "" || p.Category == "" || len(p.Species) == 0 || len(p.TrialTypes) == 0 {
				t.Errorf("incomplete paradigm %+v", p)
			}
			declared := map[string]bool{}
			for _, m := range p.Metrics {
				declared[m.Key] = true
				if m.Label.EN == "" || m.Definition.EN == "" || m.Formula == "" || m.MissingWhen.EN == "" || len(m.Inputs) == 0 ||
					!slices.Contains(units, m.Unit) || m.Min > m.Max || m.Tolerance <= 0 {
					t.Errorf("incomplete metric %+v", m)
				}
			}
			for _, param := range p.Parameters {
				if param.Label.EN == "" || param.Default < param.Min || param.Default > param.Max || !slices.Contains(units, param.Unit) ||
					(param.Integer && param.Default != math.Trunc(param.Default)) {
					t.Errorf("parameter %+v", param)
				}
			}
			for _, z := range p.Zones {
				if z.Key == "" || z.Role == "" || z.Geometry.EN == "" {
					t.Errorf("zone %+v", z)
				}
			}
			kinds := map[string]EventKind{}
			for _, e := range append(slices.Clone(p.Events), p.InputEvents...) {
				if e.Definition.EN == "" || (e.Kind != IntervalEvent && e.Kind != PointEvent) {
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

// Every user-facing text exists in every language, reads as words rather than
// keys, and every parameter says where it is set and what it means.
func TestCatalogTextIsCompleteAndReadable(t *testing.T) {
	scopes := []Scope{ApparatusScope, AnalysisScope, SessionScope}
	for _, p := range All() {
		t.Run(p.Key, func(t *testing.T) {
			check := func(what string, text Text, short bool) {
				t.Helper()
				for _, lang := range Languages {
					value := text.In(lang)
					if (lang == "tr" && text.TR == "") || value == "" {
						t.Errorf("%s: missing %s text", what, lang)
					}
					if short && strings.Contains(value, "_") {
						t.Errorf("%s: %s label looks like a key: %q", what, lang, value)
					}
					if strings.ContainsRune(value, '\u2014') {
						t.Errorf("%s: %s text uses an em dash", what, lang)
					}
				}
			}
			check("name", p.Name, true)
			check("description", p.Description, false)
			for _, tt := range p.TrialTypes {
				check("trial type "+tt.Key, tt.Label, true)
			}
			seen := map[string]bool{}
			lastScope := 0
			for _, param := range p.Parameters {
				check("parameter "+param.Key, param.Label, true)
				check("parameter help "+param.Key, param.Help, false)
				i := slices.Index(scopes, param.Scope)
				if i < 0 || i < lastScope {
					t.Errorf("parameter %s: scope %q missing or out of order (apparatus, analysis, session)", param.Key, param.Scope)
				}
				lastScope = max(lastScope, i)
				if seen[param.Key] {
					t.Errorf("parameter %s declared twice", param.Key)
				}
				seen[param.Key] = true
			}
			for _, z := range p.Zones {
				check("zone "+z.Key, z.Label, true)
				check("zone geometry "+z.Key, z.Geometry, false)
			}
			for _, e := range append(slices.Clone(p.Events), p.InputEvents...) {
				check("event "+e.Type, e.Label, true)
				check("event definition "+e.Type, e.Definition, false)
			}
			for _, m := range p.Metrics {
				check("metric "+m.Key, m.Label, true)
				check("metric description "+m.Key, m.Description, false)
				check("metric definition "+m.Key, m.Definition, false)
				check("metric missing rule "+m.Key, m.MissingWhen, false)
			}
		})
	}
}
