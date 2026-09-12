package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func step(position int) Step {
	return Step{Position: position, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, EnvironmentRevisionID: "rev", TrialType: "STANDARD", Trials: 1}
}

func TestProtocolFieldsAreValidated(t *testing.T) {
	p, err := NewProtocol("exp", "  Anxiety battery ", " two tests ")
	if err != nil || p.Name != "Anxiety battery" || p.Description != "two tests" || p.ExperimentID != "exp" {
		t.Fatalf("normalized: %+v %v", p, err)
	}
	for name, tc := range map[string]struct {
		name, description string
		want              error
	}{
		"blank name":       {" ", "", ErrInvalidName},
		"long name":        {strings.Repeat("a", 121), "", ErrInvalidName},
		"control char":     {"Bat\ntery", "", ErrInvalidName},
		"long description": {"Battery", strings.Repeat("d", 5001), ErrInvalidDescription},
	} {
		if _, err := NewProtocol("exp", tc.name, tc.description); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func TestVersionStepsAreOrderedByPosition(t *testing.T) {
	epm := step(1)
	epm.ParadigmKey = "EPM"
	openField := step(2)
	openField.Session = map[string]float64{"max_sample_gap_s": 0.25}
	v, err := NewVersion("protocol", " baseline ", "user", []Step{openField, epm})
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{v.Steps[0].ParadigmKey, v.Steps[1].ParadigmKey}
	if !slices.Equal(keys, []string{"EPM", "OPEN_FIELD"}) || v.Notes != "baseline" || v.CreatedBy != "user" {
		t.Fatalf("version: %+v", v)
	}
	openField.Session["max_sample_gap_s"] = 4
	if v.Steps[1].Session["max_sample_gap_s"] != 0.25 {
		t.Fatal("a version must not share session values with the caller")
	}

	tooMany := make([]Step, 51)
	for i := range tooMany {
		tooMany[i] = step(i + 1)
	}
	with := func(position int, mutate func(*Step)) Step { s := step(position); mutate(&s); return s }
	for name, tc := range map[string]struct {
		steps []Step
		want  error
	}{
		"no steps":             {nil, ErrNoSteps},
		"too many steps":       {tooMany, ErrTooManySteps},
		"gap":                  {[]Step{step(1), step(3)}, ErrInvalidStepOrder},
		"duplicate position":   {[]Step{step(1), step(1)}, ErrInvalidStepOrder},
		"position zero":        {[]Step{step(0), step(1)}, ErrInvalidStepOrder},
		"not starting at one":  {[]Step{step(2)}, ErrInvalidStepOrder},
		"zero trials":          {[]Step{with(1, func(s *Step) { s.Trials = 0 })}, ErrInvalidTrials},
		"too many trials":      {[]Step{with(1, func(s *Step) { s.Trials = 1001 })}, ErrInvalidTrials},
		"negative interval":    {[]Step{with(1, func(s *Step) { s.InterTrialIntervalS = -1 })}, ErrInvalidInterval},
		"interval over a day":  {[]Step{with(1, func(s *Step) { s.InterTrialIntervalS = 86401 })}, ErrInvalidInterval},
		"malformed trial type": {[]Step{with(1, func(s *Step) { s.TrialType = "probe" })}, ErrInvalidTrialType},
		"malformed paradigm":   {[]Step{with(1, func(s *Step) { s.ParadigmKey = "maze" })}, ErrInvalidParadigmKey},
		"paradigm version 0":   {[]Step{with(1, func(s *Step) { s.ParadigmVersion = 0 })}, ErrInvalidParadigmVersion},
		"no environment":       {[]Step{with(1, func(s *Step) { s.EnvironmentRevisionID = "" })}, ErrMissingEnvironment},
		"long step notes":      {[]Step{with(1, func(s *Step) { s.Notes = strings.Repeat("n", 2001) })}, ErrInvalidNotes},
	} {
		if _, err := NewVersion("protocol", "", "user", tc.steps); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	if _, err := NewVersion("protocol", strings.Repeat("n", 2001), "user", []Step{step(1)}); !errors.Is(err, ErrInvalidNotes) {
		t.Errorf("version notes: %v", err)
	}
}
