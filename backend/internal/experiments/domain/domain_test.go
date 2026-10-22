package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExperimentValidation(t *testing.T) {
	e, err := NewExperiment(ExperimentFields{Code: " E-1 ", Title: " Study ", Hypothesis: "X rescues\nthe deficit", Blinded: true})
	if err != nil || e.Code != "E-1" || e.Title != "Study" || e.Status != Draft || !e.Blinded || e.Hypothesis != "X rescues\nthe deficit" {
		t.Fatalf("experiment=%+v err=%v", e, err)
	}
	for _, tc := range []struct {
		f    ExperimentFields
		want error
	}{
		{ExperimentFields{Code: "", Title: "T"}, ErrInvalidCode},
		{ExperimentFields{Code: "a b", Title: "T"}, ErrInvalidCode},
		{ExperimentFields{Code: "E", Title: ""}, ErrInvalidTitle},
		{ExperimentFields{Code: "E", Title: "a\nb"}, ErrInvalidTitle},
		{ExperimentFields{Code: "E", Title: "T", Hypothesis: strings.Repeat("h", 2001)}, ErrInvalidHypothesis},
		{ExperimentFields{Code: "E", Title: "T", Randomisation: strings.Repeat("r", 501)}, ErrInvalidRandomisation},
		{ExperimentFields{Code: "E", Title: "T", Description: "\x00"}, ErrInvalidDescription},
	} {
		if _, err := NewExperiment(tc.f); !errors.Is(err, tc.want) {
			t.Errorf("%+v: err=%v want %v", tc.f, err, tc.want)
		}
	}
	if !e.DesignChanged(ExperimentFields{Code: "E-1", Title: "Study", Hypothesis: "X rescues\nthe deficit"}) {
		t.Error("turning blinding off is a design change")
	}
	if e.DesignChanged(ExperimentFields{Code: "E-2", Title: "Renamed", Hypothesis: "X rescues\nthe deficit", Blinded: true}) {
		t.Error("renaming is not a design change")
	}
}

func TestGroupNeedsADiseaseModelUnlessHealthy(t *testing.T) {
	if _, err := NewGroup("e", GroupFields{Code: "G1", Name: "Healthy", Role: "HEALTHY_CONTROL"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		f    GroupFields
		want error
	}{
		{GroupFields{Code: "G1", Name: "Healthy", Role: "HEALTHY_CONTROL", DiseaseModelID: "m"}, ErrDiseaseModelNotAllowed},
		{GroupFields{Code: "G2", Name: "Sick", Role: "DISEASE_CONTROL"}, ErrDiseaseModelRequired},
		{GroupFields{Code: "G2", Name: "Sick", Role: "CONTROL", DiseaseModelID: "m"}, ErrInvalidRole},
		{GroupFields{Code: "G 2", Name: "Sick", Role: "SHAM", DiseaseModelID: "m"}, ErrInvalidGroupCode},
		{GroupFields{Code: strings.Repeat("G", 17), Name: "Sick", Role: "SHAM", DiseaseModelID: "m"}, ErrInvalidGroupCode},
		{GroupFields{Code: "G2", Name: "", Role: "SHAM", DiseaseModelID: "m"}, ErrInvalidName},
		{GroupFields{Code: "G2", Name: "Sick", Role: "SHAM", DiseaseModelID: "m", TargetSize: -1}, ErrInvalidTargetSize},
		{GroupFields{Code: "G2", Name: "Sick", Role: "SHAM", DiseaseModelID: "m", TargetSize: 100001}, ErrInvalidTargetSize},
	} {
		if _, err := NewGroup("e", tc.f); !errors.Is(err, tc.want) {
			t.Errorf("%+v: err=%v want %v", tc.f, err, tc.want)
		}
	}
	g, _ := NewGroup("e", GroupFields{Code: "G2", Name: "Sick", Role: "DISEASE_CONTROL", DiseaseModelID: "m", TargetSize: 8})
	before := g
	if err := g.Apply(GroupFields{Code: "G2", Name: "Sick", Role: "HEALTHY_CONTROL", DiseaseModelID: "m"}); err == nil || g != before {
		t.Fatalf("a failed Apply changed the group: %+v err=%v", g, err)
	}
}

func TestRegimenItemValidation(t *testing.T) {
	valid := RegimenFields{GroupID: "g", Purpose: "TREATMENT", SubstanceID: "s", Amount: "2.5", Unit: "mg/kg", Route: "ORAL", Frequency: "DAILY", StartDay: 1, EndDay: 14}
	r, err := NewRegimenItem("e", valid)
	if err != nil || r.DoseMicro != 2_500_000 || r.Amount() != "2.5" {
		t.Fatalf("item=%+v err=%v", r, err)
	}
	change := func(edit func(*RegimenFields)) RegimenFields {
		f := valid
		edit(&f)
		return f
	}
	for _, tc := range []struct {
		f    RegimenFields
		want error
	}{
		{change(func(f *RegimenFields) { f.Purpose = "DOSE" }), ErrInvalidPurpose},
		{change(func(f *RegimenFields) { f.Amount = "0" }), ErrInvalidAmount},
		{change(func(f *RegimenFields) { f.Amount = "1.0000001" }), ErrInvalidAmount},
		{change(func(f *RegimenFields) { f.Amount = "-1" }), ErrInvalidAmount},
		{change(func(f *RegimenFields) { f.Unit = "kg" }), ErrInvalidUnit},
		{change(func(f *RegimenFields) { f.Route = "RECTAL" }), ErrInvalidRoute},
		{change(func(f *RegimenFields) { f.Frequency = "HOURLY" }), ErrInvalidFrequency},
		{change(func(f *RegimenFields) { f.Frequency = "EVERY_N_DAYS" }), ErrInvalidInterval},
		{change(func(f *RegimenFields) { f.IntervalDays = 3 }), ErrInvalidInterval},
		{change(func(f *RegimenFields) { f.Frequency, f.IntervalDays = "EVERY_N_DAYS", 1 }), ErrInvalidInterval},
		{change(func(f *RegimenFields) { f.EndDay = 0 }), ErrInvalidDays},
		{change(func(f *RegimenFields) { f.Frequency = "ONCE" }), ErrInvalidDays},
		{change(func(f *RegimenFields) { f.StartDay = -366 }), ErrInvalidDays},
	} {
		if _, err := NewRegimenItem("e", tc.f); !errors.Is(err, tc.want) {
			t.Errorf("%+v: err=%v want %v", tc.f, err, tc.want)
		}
	}
	if _, err := NewRegimenItem("e", change(func(f *RegimenFields) { f.Frequency, f.IntervalDays, f.Unit = "EVERY_N_DAYS", 3, "mL/kg" })); err != nil {
		t.Error(err)
	}
}

func TestAmountsAreExact(t *testing.T) {
	for raw, want := range map[string]int64{"1": 1_000_000, "0.000001": 1, "999999999.999999": 999_999_999_999_999, " 10.50 ": 10_500_000} {
		if got, err := ParseMicro(raw); err != nil || got != want {
			t.Errorf("ParseMicro(%q)=%d,%v want %d", raw, got, err, want)
		}
	}
	for micro, want := range map[int64]string{1_000_000: "1", 1: "0.000001", 10_500_000: "10.5"} {
		if got := FormatMicro(micro); got != want {
			t.Errorf("FormatMicro(%d)=%q want %q", micro, got, want)
		}
	}
}

func TestTimepointAndEndpointValidation(t *testing.T) {
	if _, err := NewTimepoint("e", TimepointFields{Name: "Baseline", StudyDay: -1, Position: 1}); err != nil {
		t.Fatal(err)
	}
	for f, want := range map[TimepointFields]error{
		{Name: "", StudyDay: 0, Position: 1}:     ErrInvalidName,
		{Name: "D", StudyDay: 3651, Position: 1}: ErrInvalidStudyDay,
		{Name: "D", StudyDay: 0, Position: 0}:    ErrInvalidPosition,
	} {
		if _, err := NewTimepoint("e", f); !errors.Is(err, want) {
			t.Errorf("%+v: err=%v want %v", f, err, want)
		}
	}
	valid := EndpointFields{TimepointID: "t2", BaselineTimepointID: "t1", ParadigmKey: "MWM", MetricKey: "path_length_cm", Kind: "PRIMARY", Direction: "HIGHER_IS_BETTER"}
	if _, err := NewEndpoint("e", valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		edit func(*EndpointFields)
		want error
	}{
		{func(f *EndpointFields) { f.BaselineTimepointID = "t2" }, ErrBaselineSameTimepoint},
		{func(f *EndpointFields) { f.ParadigmKey = "water_maze" }, ErrInvalidParadigm},
		{func(f *EndpointFields) { f.MetricKey = "Distance" }, ErrInvalidMetric},
		{func(f *EndpointFields) { f.Kind = "MAIN" }, ErrInvalidKind},
		{func(f *EndpointFields) { f.Direction = "UP" }, ErrInvalidDirection},
	} {
		f := valid
		tc.edit(&f)
		if _, err := NewEndpoint("e", f); !errors.Is(err, tc.want) {
			t.Errorf("%+v: err=%v want %v", f, err, tc.want)
		}
	}
}

func TestExclusionHappensOnceAndAfterAllocation(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	a, err := NewAllocation("e", "g", "s", " C1 ", time.Time{}, now, "u")
	if err != nil || !a.AllocatedAt.Equal(now) || a.Cage != "C1" {
		t.Fatalf("allocation=%+v err=%v", a, err)
	}
	if _, err := NewAllocation("e", "g", "s", "", now.Add(time.Hour), now, "u"); !errors.Is(err, ErrFutureTime) {
		t.Errorf("future allocation: %v", err)
	}
	if _, err := NewAllocation("e", "g", "s", strings.Repeat("c", 33), now, now, "u"); !errors.Is(err, ErrInvalidCage) {
		t.Errorf("long cage: %v", err)
	}
	if err := a.Exclude("LOST", "", time.Time{}, now); !errors.Is(err, ErrInvalidExclusionReason) {
		t.Errorf("reason: %v", err)
	}
	if err := a.Exclude("DIED", "", now.Add(-time.Hour), now); !errors.Is(err, ErrExclusionBeforeAllocation) {
		t.Errorf("before allocation: %v", err)
	}
	if err := a.Exclude("DIED", "found in cage", time.Time{}, now); err != nil || !a.Excluded() || a.ExclusionReason != Died {
		t.Fatalf("exclude: %+v %v", a, err)
	}
	if err := a.Exclude("OTHER", "", time.Time{}, now); !errors.Is(err, ErrAlreadyExcluded) {
		t.Errorf("second exclusion: %v", err)
	}
}

// rng is a deterministic source for PlanRandomisation.
func rng(seed int) func(int) int {
	return func(n int) int {
		seed = (seed*1103515245 + 12345) & 0x7fffffff
		return seed % n
	}
}

func TestRandomisationFillsGroupsEvenlyUpToTheirTargets(t *testing.T) {
	subjects := []string{"a", "b", "c", "d", "e", "f", "g"}
	plan := PlanRandomisation(subjects, []Capacity{{"G1", 3}, {"G2", 3}, {"G3", 0}, {"G4", 1}}, rng(7))
	if len(plan) != len(subjects) {
		t.Fatalf("plan=%v", plan)
	}
	count := map[string]int{}
	for _, s := range subjects {
		count[plan[s]]++
	}
	if count["G1"] != 3 || count["G2"] != 3 || count["G4"] != 1 || count["G3"] != 0 {
		t.Fatalf("counts=%v", count)
	}
	if PlanRandomisation(subjects, []Capacity{{"G1", 3}, {"G2", 3}}, rng(1)) != nil {
		t.Fatal("over-filled the groups")
	}
	// Equal free places are filled in turn: four animals over two groups of
	// ten end two and two, whatever the random order.
	for seed := range 20 {
		count := map[string]int{}
		for _, g := range PlanRandomisation([]string{"a", "b", "c", "d"}, []Capacity{{"G1", 10}, {"G2", 10}}, rng(seed)) {
			count[g]++
		}
		if count["G1"] != 2 || count["G2"] != 2 {
			t.Fatalf("seed %d: counts=%v", seed, count)
		}
	}
	// Different random sources give different assignments.
	seen := map[string]bool{}
	for seed := range 20 {
		p := PlanRandomisation([]string{"a", "b", "c", "d"}, []Capacity{{"G1", 2}, {"G2", 2}}, rng(seed))
		seen[p["a"]+p["b"]+p["c"]+p["d"]] = true
	}
	if len(seen) < 2 {
		t.Fatal("randomisation is not random")
	}
}

func TestReadinessListsEveryUnmetStartRule(t *testing.T) {
	if got := rules(Readiness(Design{})); !slices.Equal(got, []string{RuleNoTimepoints, RuleNoGroups, RuleHealthyControlMissing, RulePrimaryEndpointMissing}) {
		t.Fatalf("empty design: %v", got)
	}

	group := func(id string, role Role, model string, target int) Group {
		return Group{ID: id, Role: role, DiseaseModelID: model, TargetSize: target}
	}
	item := func(group string, purpose Purpose) RegimenItem { return RegimenItem{GroupID: group, Purpose: purpose} }
	ready := Design{
		Groups: []Group{
			group("healthy", HealthyControl, "", 8), group("sham", Sham, "mptp", 8), group("disease", DiseaseControl, "mptp", 8),
			group("positive", PositiveControl, "mptp", 8), group("treated", Treatment, "mptp", 8),
		},
		Items: []RegimenItem{
			item("healthy", Vehicle), item("sham", Vehicle),
			item("disease", Induction), item("disease", Vehicle),
			item("positive", Induction), item("positive", TreatmentPurpose),
			item("treated", Induction), item("treated", TreatmentPurpose), item("treated", Vehicle),
		},
		Timepoints: []Timepoint{{ID: "day14"}},
		Endpoints:  []Endpoint{{ID: "primary", Kind: Primary, ParadigmKey: "MWM", MetricKey: "path_length_cm"}},
		Metrics:    map[string]map[string]bool{"MWM": {"path_length_cm": true}},
	}
	if got := Readiness(ready); len(got) != 0 {
		t.Fatalf("ready design: %v", got)
	}

	broken := ready
	broken.Groups = []Group{
		group("healthy", HealthyControl, "", 0), group("sham", Sham, "lps", 8), group("disease", DiseaseControl, "mptp", 8),
		group("positive", PositiveControl, "mptp", 8), group("treated", Treatment, "mptp", 8),
	}
	broken.Items = []RegimenItem{
		item("healthy", Induction), item("healthy", TreatmentPurpose),
		item("sham", Induction), item("sham", TreatmentPurpose),
		item("disease", TreatmentPurpose),
		item("treated", Vehicle),
	}
	broken.Endpoints = []Endpoint{{ID: "secondary", Kind: Secondary, ParadigmKey: "POLE", MetricKey: "t_turn_s"}}
	want := []Problem{
		{Rule: RulePrimaryEndpointMissing},
		{Rule: RuleEndpointMetricMissing, EndpointID: "secondary"},
		{Rule: RuleTargetSizeMissing, GroupID: "healthy"},
		{Rule: RuleInductionItemNotAllowed, GroupID: "healthy"},
		{Rule: RuleTreatmentItemNotAllowed, GroupID: "healthy"},
		{Rule: RuleVehicleItemMissing, GroupID: "sham"},
		{Rule: RuleInductionItemNotAllowed, GroupID: "sham"},
		{Rule: RuleTreatmentItemNotAllowed, GroupID: "sham"},
		{Rule: RuleDiseaseControlMissing, GroupID: "sham"},
		{Rule: RuleInductionItemMissing, GroupID: "disease"},
		{Rule: RuleVehicleItemMissing, GroupID: "disease"},
		{Rule: RuleTreatmentItemNotAllowed, GroupID: "disease"},
		{Rule: RuleTreatmentItemMissing, GroupID: "positive"},
		{Rule: RuleInductionItemMissing, GroupID: "positive"},
		{Rule: RuleTreatmentItemMissing, GroupID: "treated"},
		{Rule: RuleInductionItemMissing, GroupID: "treated"},
	}
	if got := Readiness(broken); !slices.Equal(got, want) {
		t.Fatalf("broken design:\n got %v\nwant %v", got, want)
	}
}

func TestReadinessNeedsAHealthyControl(t *testing.T) {
	base := Design{
		Timepoints: []Timepoint{{ID: "day14"}},
		Endpoints:  []Endpoint{{ID: "primary", Kind: Primary, ParadigmKey: "MWM", MetricKey: "path_length_cm"}},
		Metrics:    map[string]map[string]bool{"MWM": {"path_length_cm": true}},
	}
	tests := []struct {
		name   string
		groups []Group
		items  []RegimenItem
		want   []string
	}{
		{"no groups", nil, nil, []string{RuleNoGroups, RuleHealthyControlMissing}},
		{
			"disease control only",
			[]Group{{ID: "disease", Role: DiseaseControl, DiseaseModelID: "mptp", TargetSize: 8}},
			[]RegimenItem{{GroupID: "disease", Purpose: Induction}, {GroupID: "disease", Purpose: Vehicle}},
			[]string{RuleHealthyControlMissing},
		},
		{
			"naive healthy control without items",
			[]Group{{ID: "healthy", Role: HealthyControl, TargetSize: 8}},
			nil,
			[]string{},
		},
		{
			"vehicle healthy control",
			[]Group{{ID: "healthy", Role: HealthyControl, TargetSize: 8}},
			[]RegimenItem{{GroupID: "healthy", Purpose: Vehicle}},
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := base
			d.Groups, d.Items = tt.groups, tt.items
			if got := rules(Readiness(d)); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRolePurposes(t *testing.T) {
	tests := []struct {
		role    Role
		allowed []Purpose
	}{
		{HealthyControl, []Purpose{Vehicle}},
		{Sham, []Purpose{Vehicle}},
		{DiseaseControl, []Purpose{Induction, Vehicle}},
		{PositiveControl, []Purpose{Induction, Vehicle, TreatmentPurpose}},
		{Treatment, []Purpose{Induction, Vehicle, TreatmentPurpose}},
		{Role("UNKNOWN"), nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.Purposes(); !slices.Equal(got, tt.allowed) {
				t.Fatalf("purposes %v, want %v", got, tt.allowed)
			}
			for _, p := range []Purpose{Induction, Vehicle, TreatmentPurpose} {
				err := tt.role.AllowPurpose(p)
				if want := slices.Contains(tt.allowed, p); want != (err == nil) {
					t.Fatalf("%s: allowed=%v err=%v", p, want, err)
				}
				if err != nil && !errors.Is(err, ErrPurposeNotAllowed) {
					t.Fatalf("%s: %v", p, err)
				}
			}
		})
	}
}

func rules(problems []Problem) []string {
	out := []string{}
	for _, p := range problems {
		out = append(out, p.Rule)
	}
	return out
}
