package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func row(test, subject, group, role string, value *float64, reason string) MetricRow {
	return MetricRow{
		Provenance: Provenance{TestID: test, SubjectID: subject, GroupID: group, GroupName: strings.ToUpper(group), GroupRole: role, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, MetricEngineVersion: 1},
		MetricKey:  "distance_cm", Unit: "cm", Value: value, MissingReason: reason,
	}
}

func v(f float64) *float64 { return &f }

func near(p *float64, want float64) bool { return p != nil && math.Abs(*p-want) < 1e-9 }

func TestSummaryUsesOneValuePerSubjectAndKeepsMissingResultsOut(t *testing.T) {
	rows := []MetricRow{
		row("t1", "s1", "treated", "TREATMENT", nil, "NOT_OBSERVED"),
		row("t2", "s1", "treated", "TREATMENT", v(5), ""),
		row("t3", "s2", "treated", "TREATMENT", nil, "NOT_OBSERVED"),
		row("t4", "a1", "control", "CONTROL", v(10), ""),
		row("t5", "a1", "control", "CONTROL", v(20), ""),
		row("t6", "a2", "control", "CONTROL", v(30), ""),
		row("t7", "u1", "", "", v(7), ""),
	}
	s, err := Summarize("distance_cm", rows)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version == nil || *s.Version != (Version{"OPEN_FIELD", 1, 1, "cm"}) || len(s.Groups) != 3 {
		t.Fatalf("summary: %+v", s)
	}
	control, treated, ungrouped := s.Groups[0], s.Groups[1], s.Groups[2]
	// a1 contributes mean(10, 20) = 15 once; repeated tests are not extra animals.
	if control.GroupID != "control" || control.Tests != 3 || control.Subjects != 2 || control.MissingSubjects != 0 ||
		!near(control.Mean, 22.5) || !near(control.SD, math.Sqrt(112.5)) || !near(control.SEM, 7.5) || !near(control.Min, 15) || !near(control.Max, 30) {
		t.Fatalf("control: %+v", control)
	}
	// s1's missing test is not a zero, and s2 has no value at all.
	if treated.GroupID != "treated" || treated.Tests != 3 || treated.Subjects != 1 || treated.MissingSubjects != 1 ||
		treated.MissingReasons["NOT_OBSERVED"] != 2 || !near(treated.Mean, 5) || treated.SD != nil || treated.SEM != nil {
		t.Fatalf("treated: %+v", treated)
	}
	if ungrouped.GroupID != "" || ungrouped.Subjects != 1 || !near(ungrouped.Mean, 7) {
		t.Fatalf("tests without a group come last: %+v", ungrouped)
	}
}

func TestSummaryOrdersControlsFirstThenByName(t *testing.T) {
	s, err := Summarize("distance_cm", []MetricRow{
		row("t1", "s1", "b", "TREATMENT", v(1), ""), row("t2", "s2", "z", "CONTROL", v(1), ""), row("t3", "s3", "a", "TREATMENT", v(1), ""),
	})
	if err != nil || s.Groups[0].GroupID != "z" || s.Groups[1].GroupID != "a" || s.Groups[2].GroupID != "b" {
		t.Fatalf("order: %+v %v", s.Groups, err)
	}
}

func TestSummaryRefusesIncomparableResults(t *testing.T) {
	engine2 := row("t1", "s1", "g", "CONTROL", v(1), "")
	engine2.MetricEngineVersion = 2
	meters := row("t2", "s2", "g", "CONTROL", v(1), "")
	meters.Unit = "m"
	version2 := row("t3", "s3", "g", "CONTROL", v(1), "")
	version2.ParadigmVersion = 2
	for name, other := range map[string]MetricRow{"engine version": engine2, "unit": meters, "paradigm version": version2} {
		// The same test with a newer engine is a version conflict, not a duplicate.
		first := row("t1", "s1", "g", "CONTROL", v(1), "")
		_, err := Summarize("distance_cm", []MetricRow{first, other})
		var incompatible IncompatibleError
		if !errors.Is(err, ErrIncompatibleVersions) || !errors.As(err, &incompatible) || len(incompatible.Versions) != 2 ||
			!strings.Contains(err.Error(), "OPEN_FIELD v1 engine 1 (cm)") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Summarize("distance_cm", []MetricRow{row("t1", "s1", "g", "CONTROL", v(1), ""), row("t1", "s1", "g", "CONTROL", v(2), "")}); !errors.Is(err, ErrDuplicateTest) {
		t.Fatalf("duplicate test: %v", err)
	}
	other := row("t2", "s1", "g", "CONTROL", v(1), "")
	other.MetricKey = "velocity_cm_s"
	if _, err := Summarize("distance_cm", []MetricRow{row("t1", "s1", "g", "CONTROL", v(1), ""), other}); !errors.Is(err, ErrMixedMetrics) {
		t.Fatalf("mixed metrics: %v", err)
	}
	if s, err := Summarize("distance_cm", nil); err != nil || s.Version != nil || s.Groups == nil || len(s.Groups) != 0 {
		t.Fatalf("empty: %+v %v", s, err)
	}
}
