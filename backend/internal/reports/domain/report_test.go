package domain

import (
	"math"
	"testing"
)

func dose(v int64) *int64 { return &v }

var groups = []Group{
	{ID: "H", Code: "G1", Role: HealthyControl, Allocated: 2},
	{ID: "S", Code: "G2", Role: Sham, DiseaseModelID: "dm"},
	{ID: "D", Code: "G3", Role: DiseaseControl, DiseaseModelID: "dm"},
	{ID: "P", Code: "G4", Role: PositiveControl, DiseaseModelID: "dm", SubstanceID: "ref", DoseMicro: dose(5_000_000), Unit: "mg/kg"},
	{ID: "T2", Code: "G6", Role: Treatment, DiseaseModelID: "dm", SubstanceID: "x", SubstanceName: "X", DoseMicro: dose(3_000_000), Unit: "mg/kg", Excluded: 1},
	{ID: "T1", Code: "G5", Role: Treatment, DiseaseModelID: "dm", SubstanceID: "x", SubstanceName: "X", DoseMicro: dose(1_000_000), Unit: "mg/kg"},
	{ID: "O", Code: "G7", Role: Treatment, DiseaseModelID: "other", SubstanceID: "y", DoseMicro: dose(1), Unit: "mg"},
}

var timepoints = []Timepoint{{ID: "base", Name: "Baseline", StudyDay: -1, Position: 1}, {ID: "d14", Name: "Day 14", StudyDay: 14, Position: 2}, {ID: "d28", Position: 3}}

// accepted is an accepted measurement of an animal (allocation) in a group.
func accepted(group, animal, timepoint string, value float64) Measurement {
	return Measurement{TestSubjectID: animal + timepoint, AllocationID: animal, GroupID: group, TimepointID: timepoint, ResultStatus: "ACCEPTED",
		ParadigmVersion: 1, HasMetric: true, Unit: "cm", Value: &value, MetricEngineVersion: 1}
}

func status(group, animal, timepoint, s string) Measurement {
	return Measurement{TestSubjectID: animal + timepoint + s, AllocationID: animal, GroupID: group, TimepointID: timepoint, ResultStatus: s, ParadigmVersion: 1}
}

func rows() []Measurement {
	excluded := accepted("T2", "t2x", "d14", 1000)
	excluded.Excluded, excluded.HasMetric = true, false
	missing := status("T2", "t2m", "d14", "ACCEPTED")
	missing.HasMetric, missing.Unit, missing.MissingReason, missing.MetricEngineVersion = true, "cm", "NO_TRACKING", 1
	return []Measurement{
		accepted("H", "a1", "base", 90), accepted("D", "d1", "base", 100),
		accepted("H", "a1", "d14", 100), accepted("H", "a2", "d14", 120),
		accepted("S", "s1", "d14", 105),
		accepted("D", "d1", "d14", 40), accepted("D", "d2", "d14", 60),
		accepted("P", "p1", "d14", 90),
		accepted("T1", "t1", "d14", 60), status("T1", "t1b", "d14", "PENDING"),
		// One animal tested twice at the same timepoint counts once, with its mean.
		accepted("T2", "t2", "d14", 80), accepted("T2", "t2", "d14", 100),
		excluded, status("T2", "t2r", "d14", "REJECTED"), status("T2", "t2w", "d14", ""), missing,
	}
}

func near(t *testing.T, label string, got *float64, want float64) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s: got %v want %v", label, deref(got), want)
	}
}

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func TestSectionDescribesGroupsAndComparesByRole(t *testing.T) {
	e := Endpoint{ID: "e", TimepointID: "d14", BaselineTimepointID: "base", ParadigmKey: "MWM", MetricKey: "path_length_cm", Direction: "HIGHER_IS_BETTER"}
	s := Analyse(e, groups, timepoints, rows())
	if !s.Comparable || len(s.Versions) != 1 || s.Versions[0] != (Version{1, 1, "cm"}) || len(s.Groups) != len(groups) {
		t.Fatalf("section: %+v", s)
	}
	stats := map[string]GroupStats{}
	for _, g := range s.Groups {
		stats[g.GroupID] = g
	}
	h := stats["H"]
	near(t, "healthy mean", h.Mean, 110)
	near(t, "healthy sd", h.SD, math.Sqrt(200))
	near(t, "healthy sem", h.SEM, 10)
	near(t, "healthy change", h.Change.Mean, 10)
	if h.N != 2 || h.Tested != 2 || h.Change.N != 1 {
		t.Errorf("healthy: %+v", h)
	}
	if s := stats["S"]; s.N != 1 || s.SD != nil || s.SEM != nil {
		t.Errorf("one animal has no deviation: %+v", s)
	}
	near(t, "disease change", stats["D"].Change.Mean, -60)
	t2 := stats["T2"]
	near(t, "repeated tests are averaged per animal", t2.Mean, 90)
	if t2.N != 1 || t2.Tested != 5 || t2.Rejected != 1 || t2.AwaitingAnalysis != 1 || t2.Missing != 1 || t2.Excluded != 1 || t2.Pending != 0 {
		t.Errorf("treatment high counts: %+v", t2)
	}
	if t1 := stats["T1"]; t1.Pending != 1 || t1.N != 1 {
		t.Errorf("pending: %+v", t1)
	}
	if o := stats["O"]; o.N != 0 || o.Mean != nil || o.Tested != 0 {
		t.Errorf("untested group: %+v", o)
	}

	byPair := map[string]Comparison{}
	for _, c := range s.Comparisons {
		byPair[c.Kind+" "+c.GroupID+" "+c.ReferenceGroupID] = c
	}
	if len(s.Comparisons) != 5 {
		t.Errorf("comparisons: %+v", s.Comparisons)
	}
	mv := byPair["MODEL_VALIDITY D H"]
	near(t, "model validity difference", mv.Difference, -60)
	near(t, "model validity percent", mv.PercentDifference, -60.0/110*100)
	near(t, "model validity effect", mv.Effect, -60)
	if mv.RescuePercent != nil {
		t.Errorf("model validity has no rescue: %+v", mv)
	}
	near(t, "disease vs sham", byPair["MODEL_VALIDITY D S"].Difference, -55)
	as := byPair["ASSAY_SENSITIVITY P D"]
	near(t, "assay sensitivity rescue", as.RescuePercent, 40.0/60*100)
	if as.HealthyReferenceGroupID != "H" {
		t.Errorf("healthy reference: %+v", as)
	}
	near(t, "efficacy low rescue", byPair["EFFICACY T1 D"].RescuePercent, 10.0/60*100)
	near(t, "efficacy high effect", byPair["EFFICACY T2 D"].Effect, 40)
	if _, ok := byPair["EFFICACY O D"]; ok {
		t.Errorf("a treatment is compared only with the disease control of its own model")
	}

	if len(s.DoseResponse) != 1 || s.DoseResponse[0].SubstanceID != "x" || len(s.DoseResponse[0].Points) != 2 ||
		s.DoseResponse[0].Points[0].GroupID != "T1" || s.DoseResponse[0].Points[1].DoseMicro != 3_000_000 {
		t.Fatalf("dose response: %+v", s.DoseResponse)
	}
	near(t, "dose response rescue", s.DoseResponse[0].Points[1].RescuePercent, 40.0/60*100)

	if len(s.TimeCourse) != 2 || s.TimeCourse[0].TimepointID != "base" || s.TimeCourse[1].TimepointID != "d14" || s.TimeCourse[0].Groups[0].Change != nil {
		t.Fatalf("time course: %+v", s.TimeCourse)
	}
	near(t, "baseline healthy", s.TimeCourse[0].Groups[0].Mean, 90)
}

func TestDirectionAndShamReference(t *testing.T) {
	noHealthy := []Group{groups[1], groups[2], groups[5]}
	e := Endpoint{TimepointID: "d14", Direction: LowerIsBetter}
	s := Analyse(e, noHealthy, timepoints, rows())
	for _, c := range s.Comparisons {
		if c.Kind == Efficacy {
			near(t, "lower is better flips the effect", c.Effect, -10)
			near(t, "sham is the healthy reference", c.RescuePercent, 10.0/55*100)
			if c.HealthyReferenceGroupID != "S" {
				t.Errorf("reference: %+v", c)
			}
		}
	}
	if len(s.DoseResponse) != 0 {
		t.Errorf("one dose level is no dose response: %+v", s.DoseResponse)
	}
}

func TestMixedVersionsAreNeverPooled(t *testing.T) {
	mixed := rows()
	mixed[2].MetricEngineVersion = 2
	s := Analyse(Endpoint{TimepointID: "d14"}, groups, timepoints, mixed)
	if s.Comparable || len(s.Versions) != 2 || len(s.Comparisons) != 0 || len(s.DoseResponse) != 0 {
		t.Fatalf("mixed: %+v", s)
	}
	for _, g := range s.Groups {
		if g.Mean != nil || g.SD != nil {
			t.Fatalf("no statistics over mixed versions: %+v", g)
		}
	}
	if s.Groups[0].N != 2 {
		t.Fatalf("counts stay: %+v", s.Groups[0])
	}
}
