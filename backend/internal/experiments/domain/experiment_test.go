package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func TestNewExperimentPhaseAndGroup(t *testing.T) {
	e, err := NewExperiment(" EXP-1 ", "  Anxiety study ", " notes ", true)
	if err != nil || e.Code != "EXP-1" || e.Title != "Anxiety study" || e.Description != "notes" || !e.RequiresControl {
		t.Fatalf("experiment: %+v %v", e, err)
	}
	p, err := NewPhase("x", " Baseline ", 1, "")
	if err != nil || p.Name != "Baseline" || p.Position != 1 || p.ExperimentID != "x" {
		t.Fatalf("phase: %+v %v", p, err)
	}
	g, err := NewGroup("x", " Vehicle ", "CONTROL", 12, "")
	if err != nil || g.Name != "Vehicle" || g.Role != Control || g.TargetSize != 12 {
		t.Fatalf("group: %+v %v", g, err)
	}
	if g, err := NewGroup("x", "Drug", "TREATMENT", 0, ""); err != nil || g.TargetSize != 0 {
		t.Fatalf("group without target: %+v %v", g, err)
	}
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"experiment code", second(NewExperiment("EXP 1", "T", "", false)), ErrInvalidCode},
		{"experiment title", second(NewExperiment("E", " ", "", false)), ErrInvalidTitle},
		{"experiment long title", second(NewExperiment("E", strings.Repeat("t", 201), "", false)), ErrInvalidTitle},
		{"experiment description", second(NewExperiment("E", "T", strings.Repeat("d", 5001), false)), ErrInvalidDescription},
		{"phase name", second(NewPhase("x", "", 1, "")), ErrInvalidName},
		{"phase position zero", second(NewPhase("x", "P", 0, "")), ErrInvalidPosition},
		{"phase position too high", second(NewPhase("x", "P", 1001, "")), ErrInvalidPosition},
		{"group role", second(NewGroup("x", "G", "PLACEBO", 1, "")), ErrInvalidRole},
		{"negative target", second(NewGroup("x", "G", "CONTROL", -1, "")), ErrInvalidTargetSize},
		{"huge target", second(NewGroup("x", "G", "CONTROL", 100001, "")), ErrInvalidTargetSize},
		{"group name control char", second(NewGroup("x", "G\x07", "CONTROL", 1, "")), ErrInvalidName},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: err=%v want %v", tc.name, tc.err, tc.want)
		}
	}
}

func TestPlanAssignment(t *testing.T) {
	enrollment := Enrollment{ID: "e", ExperimentID: "x", EnrolledAt: t0}
	control := Group{ID: "control", ExperimentID: "x", Role: Control}
	treated := Group{ID: "treated", ExperimentID: "x", Role: Treatment}
	foreign := Group{ID: "foreign", ExperimentID: "y", Role: Treatment}
	inControl := func(from time.Time) *Assignment { return &Assignment{GroupID: "control", ValidFrom: from} }
	for _, tc := range []struct {
		name    string
		current *Assignment
		group   Group
		from    time.Time
		want    error
	}{
		{"first assignment at enrollment", nil, treated, t0, nil},
		{"group from another experiment", nil, foreign, t0, ErrGroupNotInExperiment},
		{"before enrollment", nil, treated, t0.Add(-time.Second), ErrAssignmentBeforeEnrollment},
		{"crossover later", inControl(t0), treated, t0.Add(time.Hour), nil},
		{"same group again", inControl(t0), control, t0.Add(time.Hour), ErrAlreadyInGroup},
		{"same start as current", inControl(t0.Add(2 * time.Hour)), treated, t0.Add(2 * time.Hour), ErrAssignmentOutOfOrder},
		{"rewrites history", inControl(t0.Add(2 * time.Hour)), treated, t0.Add(time.Hour), ErrAssignmentOutOfOrder},
	} {
		if err := PlanAssignment(enrollment, tc.current, tc.group, tc.from); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: err=%v want %v", tc.name, err, tc.want)
		}
	}
}

// A subject assigned to the treatment arm is still healthy during its baseline
// measurement. The baseline phase must report the assigned arm, not a control
// condition, and a later crossover must not rewrite earlier periods.
func TestHealthyBaselineKeepsAssignedGroupAndHistory(t *testing.T) {
	baseline := t0.Add(24 * time.Hour)
	treatmentStarts := t0.Add(7 * 24 * time.Hour)
	crossover := t0.Add(30 * 24 * time.Hour)
	history := []Assignment{
		{ID: "a1", GroupID: "treated", ValidFrom: t0, ValidTo: &crossover},
		{ID: "a2", GroupID: "control", ValidFrom: crossover},
	}
	for _, tc := range []struct {
		at   time.Time
		want string
	}{{baseline, "treated"}, {treatmentStarts, "treated"}, {crossover.Add(-time.Microsecond), "treated"}, {crossover, "control"}} {
		if got, ok := GroupAt(history, tc.at); !ok || got != tc.want {
			t.Errorf("GroupAt(%v)=%q,%v want %q", tc.at, got, ok, tc.want)
		}
	}
	if _, ok := GroupAt(history, t0.Add(-time.Second)); ok {
		t.Error("group reported before enrollment")
	}
	if open := OpenAssignment(history); open == nil || open.ID != "a2" {
		t.Fatalf("open assignment: %+v", open)
	}
	if OpenAssignment(history[:1]) != nil || OpenAssignment(nil) != nil {
		t.Fatal("closed history reported an open assignment")
	}
}

func TestControlRequirement(t *testing.T) {
	control := Group{Role: Control}
	treated := Group{Role: Treatment}
	for _, tc := range []struct {
		requires bool
		groups   []Group
		want     bool
	}{{false, nil, true}, {true, nil, false}, {true, []Group{treated}, false}, {true, []Group{treated, control}, true}} {
		if got := ControlRequirementMet(tc.requires, tc.groups); got != tc.want {
			t.Errorf("requires=%v groups=%v: got %v", tc.requires, tc.groups, got)
		}
	}
}

func second[T any](_ T, err error) error { return err }
