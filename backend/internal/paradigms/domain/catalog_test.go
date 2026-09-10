package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestCatalogReturnsCopies(t *testing.T) {
	p, err := Latest("OPEN_FIELD")
	if err != nil {
		t.Fatal(err)
	}
	p.Species[0] = "zebrafish"
	p.Metrics[0].Key = "hacked"
	p.Metrics[0].Inputs[0] = "hacked"
	p.ApparatusParameters[0].Max = -1
	p.Zones = nil
	all := All()
	all[0].Name = "hacked"
	result, _ := Evaluate("OPEN_FIELD", 1, Input{})
	result.Parameters["arena_width_cm"] = -1

	fresh, err := Version("OPEN_FIELD", 1)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Species[0] == "zebrafish" || fresh.Metrics[0].Key == "hacked" || fresh.Metrics[0].Inputs[0] == "hacked" ||
		fresh.ApparatusParameters[0].Max == -1 || len(fresh.Zones) == 0 || All()[0].Name == "hacked" {
		t.Fatal("caller mutated the registry")
	}
	again, _ := Evaluate("OPEN_FIELD", 1, Input{})
	if again.Parameters["arena_width_cm"] != 50 {
		t.Fatal("caller mutated default parameters")
	}
}

func TestVersionsAreSeparateAndLookedUp(t *testing.T) {
	if versions, err := Versions("OPEN_FIELD"); err != nil || !slices.Equal(versions, []int{1}) {
		t.Fatalf("versions %v %v", versions, err)
	}
	if _, err := Versions("MAZE"); !errors.Is(err, ErrUnknownParadigm) {
		t.Fatalf("unknown key: %v", err)
	}
	latest, err := Latest("OPEN_FIELD")
	if err != nil || latest.Version != 1 {
		t.Fatalf("latest %+v %v", latest, err)
	}
	if MetricEngineVersion < 1 || ResultSchemaVersion < 1 {
		t.Fatal("engine and result schema versions must be published")
	}
	if !slices.Equal(Keys(), []string{"OPEN_FIELD"}) {
		t.Fatalf("keys %v", Keys())
	}
}

// Every advertised metric is fully specified and produced by the engine, and
// the engine produces nothing undeclared.
func TestOpenFieldContractMatchesEngine(t *testing.T) {
	p, _ := Latest("OPEN_FIELD")
	units := []Unit{Centimeter, CentimeterPerSecond, Second, Count, Ratio}
	declared := map[string]bool{}
	for _, m := range p.Metrics {
		declared[m.Key] = true
		if m.Label == "" || m.Definition == "" || m.Formula == "" || m.MissingWhen == "" || len(m.Inputs) == 0 ||
			!slices.Contains(units, m.Unit) || m.Min > m.Max || m.Tolerance <= 0 {
			t.Errorf("incomplete metric definition %+v", m)
		}
	}
	for _, param := range append(p.ApparatusParameters, p.SessionParameters...) {
		if param.Default < param.Min || param.Default > param.Max || !slices.Contains(units, param.Unit) {
			t.Errorf("parameter %+v", param)
		}
	}
	events := map[string]EventKind{}
	for _, e := range p.Events {
		events[e.Type] = e.Kind
	}
	r := evaluate(t, arena40, sample(0, 5, 5), sample(0.5, 15, 15), sample(1, 15, 15), sample(1.5, 15, 15))
	if len(r.Metrics) != len(p.Metrics) {
		t.Errorf("engine produced %d metrics, catalog declares %d", len(r.Metrics), len(p.Metrics))
	}
	for _, m := range r.Metrics {
		if !declared[m.Key] {
			t.Errorf("undeclared metric %s", m.Key)
		}
	}
	for _, i := range r.Intervals {
		if events[i.Type] != IntervalEvent {
			t.Errorf("interval %s not declared as an interval event", i.Type)
		}
	}
	for _, pt := range r.Points {
		if events[pt.Type] != PointEvent {
			t.Errorf("point %s not declared as a point event", pt.Type)
		}
	}
}
