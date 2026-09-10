package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

func TestDecimalAmountsAreExact(t *testing.T) {
	for raw, want := range map[string]int64{
		"0.25": 250_000, "10": 10_000_000, "0.000001": 1, " 7.5 ": 7_500_000, "999999999.999999": 999_999_999_999_999,
	} {
		got, err := ParseMicro(raw)
		if err != nil || got != want {
			t.Errorf("ParseMicro(%q)=%d,%v want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "0", "0.000000", "-1", "1e3", "1,5", "1.1234567", "1000000000", ".5", "5.", "abc"} {
		if _, err := ParseMicro(raw); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("ParseMicro(%q) err=%v", raw, err)
		}
	}
	for micro, want := range map[int64]string{250_000: "0.25", 10_000_000: "10", 1: "0.000001", 999_999_999_999_999: "999999999.999999"} {
		if got := FormatMicro(micro); got != want {
			t.Errorf("FormatMicro(%d)=%q want %q", micro, got, want)
		}
	}
	for raw, want := range map[string]int64{"25": 25_000, "312.4": 312_400, "0.001": 1, "5000": 5_000_000} {
		if got, err := ParseGrams(raw); err != nil || got != want {
			t.Errorf("ParseGrams(%q)=%d,%v want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"0", "5000.001", "12.3456", "-2", "", "1e2"} {
		if _, err := ParseGrams(raw); !errors.Is(err, ErrInvalidWeight) {
			t.Errorf("ParseGrams(%q) err=%v", raw, err)
		}
	}
	if FormatGrams(312_400) != "312.4" || FormatGrams(25_000) != "25" {
		t.Fatal("gram formatting")
	}
}

// Expected values were calculated by hand: absolute dose = dose per kg x body mass in kg.
func TestPerKilogramDoseConversionIndependentExamples(t *testing.T) {
	for _, tc := range []struct {
		name, amount, unit, grams string
		wantAmount, wantUnit      string
	}{
		{"10 mg/kg for a 25 g mouse is 0.25 mg", "10", "mg/kg", "25", "0.25", "mg"},
		{"2.5 ug/kg for a 312.4 g rat is 0.781 ug", "2.5", "ug/kg", "312.4", "0.781", "ug"},
		{"50 IU/kg for 20.5 g is 1.025 IU", "50", "IU/kg", "20.5", "1.025", "IU"},
		{"0.500001 micro-mg rounds half up to 0.000001 mg", "0.000003", "mg/kg", "166.667", "0.000001", "mg"},
		{"largest dose for the heaviest subject stays exact", "999999999.999999", "mg/kg", "5000", "4999999999.999995", "mg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dose, err := ParseDose(tc.amount, tc.unit)
			if err != nil {
				t.Fatal(err)
			}
			body, err := ParseGrams(tc.grams)
			if err != nil {
				t.Fatal(err)
			}
			abs, err := dose.Absolute(body)
			if err != nil || abs.Amount() != tc.wantAmount || abs.Unit != Unit(tc.wantUnit) {
				t.Fatalf("got %s %s, %v", abs.Amount(), abs.Unit, err)
			}
		})
	}
	tiny, _ := ParseDose("0.000001", "mg/kg")
	if _, err := tiny.Absolute(1); !errors.Is(err, ErrAmountTooSmall) {
		t.Fatalf("dose rounding to zero: %v", err)
	}
	plain, _ := ParseDose("5", "mg")
	if abs, err := plain.Absolute(25_000); err != nil || abs != plain {
		t.Fatalf("absolute units are unchanged: %+v %v", abs, err)
	}
}

func TestUnitsAndRoutes(t *testing.T) {
	for raw, perKg := range map[string]bool{"mg": false, "ug": false, "g": false, "mL": false, "uL": false, "IU": false, "mg/kg": true, "ug/kg": true, "IU/kg": true} {
		u, err := ParseUnit(raw)
		if err != nil || u.PerBodyMass() != perKg {
			t.Errorf("ParseUnit(%q)=%v,%v perKg=%v", raw, u, err, u.PerBodyMass())
		}
	}
	for _, raw := range []string{"MG", "µg", "mg/g", "ml", ""} {
		if _, err := ParseUnit(raw); !errors.Is(err, ErrInvalidUnit) {
			t.Errorf("ParseUnit(%q) err=%v", raw, err)
		}
	}
	if _, err := ParseRoute("INTRAPERITONEAL"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"ip", "Oral", "OTHER", ""} {
		if _, err := ParseRoute(raw); !errors.Is(err, ErrInvalidRoute) {
			t.Errorf("ParseRoute(%q) err=%v", raw, err)
		}
	}
}

func TestAdministrationChecks(t *testing.T) {
	enrolled := now.Add(-10 * 24 * time.Hour)
	perKg, _ := ParseDose("10", "mg/kg")
	absolute, _ := ParseDose("1", "mg")
	weight := func(subject string, before time.Duration) *Weight {
		return &Weight{ID: "w", SubjectID: subject, Milligrams: 25_000, MeasuredAt: now.Add(-before)}
	}
	for _, tc := range []struct {
		name   string
		dose   Dose
		at     time.Time
		weight *Weight
		want   error
	}{
		{"per-kg dose with recent weight", perKg, now, weight("s1", time.Hour), nil},
		{"weight exactly 7 days before", perKg, now, weight("s1", 7*24*time.Hour), nil},
		{"absolute dose without weight", absolute, now.Add(-time.Hour), nil, nil},
		{"before enrollment", absolute, enrolled.Add(-time.Second), nil, ErrBeforeEnrollment},
		{"in the future", absolute, now.Add(6 * time.Minute), nil, ErrFutureTime},
		{"per-kg dose without weight", perKg, now, nil, ErrWeightRequired},
		{"weight of another subject", absolute, now, weight("s2", time.Hour), ErrWeightOtherSubject},
		{"weight measured after administration", perKg, now.Add(-2 * time.Hour), weight("s1", time.Hour), ErrWeightNotRelevant},
		{"weight older than 7 days", perKg, now, weight("s1", 7*24*time.Hour+time.Second), ErrWeightNotRelevant},
	} {
		a := Administration{SubjectID: "s1", Dose: tc.dose, Route: Oral, AdministeredAt: tc.at}
		if err := CheckAdministration(a, enrolled, tc.weight, now); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: err=%v want %v", tc.name, err, tc.want)
		}
	}
}

// Induction is recorded separately from confirmation, and the current status
// of each disease model follows observation time, not insertion order.
func TestCurrentConditionsKeepInductionSeparateFromConfirmation(t *testing.T) {
	day := 24 * time.Hour
	history := []Condition{
		{ID: "c3", DiseaseModelID: "stroke", Status: Induced, ObservedAt: now.Add(-2 * day)},
		{ID: "c2", DiseaseModelID: "diabetes", Status: Confirmed, ObservedAt: now.Add(-3 * day)},
		{ID: "c1", DiseaseModelID: "diabetes", Status: Induced, ObservedAt: now.Add(-9 * day)},
	}
	current := CurrentConditions(history)
	if len(current) != 2 || current["diabetes"].Status != Confirmed || current["stroke"].Status != Induced {
		t.Fatalf("current=%+v", current)
	}
	if current["stroke"].Status == Confirmed {
		t.Fatal("induction was treated as confirmed disease")
	}
	history = append(history, Condition{ID: "c4", DiseaseModelID: "stroke", Status: NotConfirmed, ObservedAt: now.Add(-day)})
	if CurrentConditions(history)["stroke"].Status != NotConfirmed {
		t.Fatal("later observation not current")
	}
}

func TestRecordValidation(t *testing.T) {
	dose, _ := ParseDose("1", "mg")
	for name, err := range map[string]error{
		"empty substance name": second(NewSubstance(" ", "")),
		"long model name":      second(NewDiseaseModel(strings.Repeat("n", 121), "")),
		"empty schedule":       second(NewPlan("e", "g", "", "s", dose, Oral, " ", "")),
		"long schedule":        second(NewPlan("e", "g", "", "s", dose, Oral, strings.Repeat("x", 501), "")),
		"future weight":        second(NewWeight("s", "25", now.Add(time.Hour), now, "u")),
		"invalid weight":       second(NewWeight("s", "0", now, now, "u")),
		"unknown status":       second(NewCondition("s", "m", "SICK", now, now, "", "", "u")),
		"future condition":     second(NewCondition("s", "m", "INDUCED", now.Add(time.Hour), now, "", "", "u")),
		"notes control char":   second(NewCondition("s", "m", "INDUCED", now, now, "", "bad\x00", "u")),
	} {
		if err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	p, err := NewPlan("e", "g", "", "s", dose, Oral, " daily for 14 days ", "")
	if err != nil || p.Schedule != "daily for 14 days" {
		t.Fatalf("plan: %+v %v", p, err)
	}
	w, err := NewWeight("s", "24.6", now.Add(time.Minute), now, "u")
	if err != nil || w.Milligrams != 24_600 {
		t.Fatalf("weight within clock tolerance: %+v %v", w, err)
	}
}

func second[T any](_ T, err error) error { return err }
