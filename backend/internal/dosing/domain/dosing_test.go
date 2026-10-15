package domain

import (
	"errors"
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
	if FormatGrams(312_400) != "312.4" || FormatGrams(25_000) != "25" {
		t.Fatal("gram formatting")
	}
}

// Expected values were calculated by hand: absolute dose = dose per kg x body mass in kg.
func TestPerKilogramDoseConversionIndependentExamples(t *testing.T) {
	for _, tc := range []struct {
		name, amount, unit   string
		milligrams           int64
		wantAmount, wantUnit string
	}{
		{"10 mg/kg for a 25 g mouse is 0.25 mg", "10", "mg/kg", 25_000, "0.25", "mg"},
		{"2.5 ug/kg for a 312.4 g rat is 0.781 ug", "2.5", "ug/kg", 312_400, "0.781", "ug"},
		{"10 mL/kg for a 25 g mouse is 0.25 mL", "10", "mL/kg", 25_000, "0.25", "mL"},
		{"50 IU/kg for 20.5 g is 1.025 IU", "50", "IU/kg", 20_500, "1.025", "IU"},
		{"0.500001 micro-mg rounds half up to 0.000001 mg", "0.000003", "mg/kg", 166_667, "0.000001", "mg"},
		{"largest dose for the heaviest animal stays exact", "999999999.999999", "mg/kg", 5_000_000, "4999999999.999995", "mg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dose, err := ParseDose(tc.amount, tc.unit)
			if err != nil {
				t.Fatal(err)
			}
			abs, err := dose.Absolute(tc.milligrams)
			if err != nil || abs.Amount() != tc.wantAmount || abs.Unit != Unit(tc.wantUnit) {
				t.Fatalf("got %s %s, %v", abs.Amount(), abs.Unit, err)
			}
		})
	}
	tiny, _ := ParseDose("0.000001", "mg/kg")
	if _, err := tiny.Absolute(1); !errors.Is(err, ErrAmountTooSmall) {
		t.Fatalf("dose rounding to zero: %v", err)
	}
	if _, err := tiny.Absolute(0); !errors.Is(err, ErrInvalidWeight) {
		t.Fatalf("no weight: %v", err)
	}
	plain, _ := ParseDose("5", "mg")
	if abs, err := plain.Absolute(25_000); err != nil || abs != plain {
		t.Fatalf("absolute units are unchanged: %+v %v", abs, err)
	}
}

func TestUnitsRoutesPurposesAndOutcomes(t *testing.T) {
	for raw, perKg := range map[string]bool{
		"mg": false, "ug": false, "g": false, "mL": false, "uL": false, "IU": false,
		"mg/kg": true, "ug/kg": true, "mL/kg": true, "IU/kg": true,
	} {
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
	for _, raw := range []string{"INDUCTION", "VEHICLE", "TREATMENT"} {
		if p, err := ParsePurpose(raw); err != nil || string(p) != raw {
			t.Errorf("ParsePurpose(%q)=%v,%v", raw, p, err)
		}
	}
	if _, err := ParsePurpose("treatment"); !errors.Is(err, ErrInvalidPurpose) {
		t.Fatal(err)
	}
	if _, err := ParseOutcomeStatus("CONFIRMED"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"INDUCED", "RESOLVED", ""} {
		if _, err := ParseOutcomeStatus(raw); !errors.Is(err, ErrInvalidOutcome) {
			t.Errorf("ParseOutcomeStatus(%q) err=%v", raw, err)
		}
	}
}

func TestRecordTimesFitTheAllocation(t *testing.T) {
	excluded := now.Add(-time.Hour)
	alloc := Allocation{AllocatedAt: now.Add(-10 * 24 * time.Hour)}
	gone := Allocation{AllocatedAt: alloc.AllocatedAt, ExcludedAt: &excluded}
	for _, tc := range []struct {
		name  string
		at    time.Time
		alloc Allocation
		want  error
	}{
		{"now", now, alloc, nil},
		{"within clock skew", now.Add(ClockSkew), alloc, nil},
		{"missing", time.Time{}, alloc, ErrMissingTime},
		{"future", now.Add(ClockSkew + time.Second), alloc, ErrFutureTime},
		{"at allocation", alloc.AllocatedAt, alloc, nil},
		{"before allocation", alloc.AllocatedAt.Add(-time.Second), alloc, ErrBeforeAllocation},
		{"before exclusion", excluded.Add(-time.Second), gone, nil},
		{"at exclusion", excluded, gone, ErrAfterExclusion},
		{"after exclusion", now, gone, ErrAfterExclusion},
	} {
		if err := CheckTime(tc.at, tc.alloc, now); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v want %v", tc.name, err, tc.want)
		}
	}
}

func TestWeightOfADose(t *testing.T) {
	perKg := Administration{SubjectID: "s1", Dose: Dose{Micro: 10_000_000, Unit: MilligramPerKilogram}, AdministeredAt: now}
	absolute := Administration{SubjectID: "s1", Dose: Dose{Micro: 1_000_000, Unit: Milligram}, AdministeredAt: now}
	weight := func(subject string, before time.Duration) *Weight {
		return &Weight{ID: "w", SubjectID: subject, Milligrams: 25_000, MeasuredAt: now.Add(-before)}
	}
	for _, tc := range []struct {
		name   string
		a      Administration
		weight *Weight
		want   error
	}{
		{"per-kg dose with recent weight", perKg, weight("s1", time.Hour), nil},
		{"weight exactly 7 days before", perKg, weight("s1", WeightWindow), nil},
		{"weight older than 7 days", perKg, weight("s1", WeightWindow+time.Second), ErrWeightNotRelevant},
		{"weight after the dose", perKg, weight("s1", -time.Second), ErrWeightNotRelevant},
		{"weight of another animal", perKg, weight("s2", time.Hour), ErrWeightOtherSubject},
		{"per-kg dose without weight", perKg, nil, ErrWeightRequired},
		{"absolute dose without weight", absolute, nil, nil},
		{"absolute dose with a named weight", absolute, weight("s1", time.Hour), nil},
	} {
		if err := CheckWeight(tc.a, tc.weight); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v want %v", tc.name, err, tc.want)
		}
	}
	perKg.BodyMilligrams = 25_000
	if abs, ok := perKg.AbsoluteDose(); !ok || abs.Amount() != "0.25" || abs.Unit != Milligram {
		t.Fatalf("absolute dose: %+v %v", abs, ok)
	}
	perKg.BodyMilligrams = 0
	if _, ok := perKg.AbsoluteDose(); ok {
		t.Fatal("a per-kg dose without weight has no absolute amount")
	}
}

func TestNotes(t *testing.T) {
	if n, err := NormalizeNotes("  line one\nline two\t "); err != nil || n != "line one\nline two" {
		t.Fatalf("%q %v", n, err)
	}
	long := make([]rune, 2001)
	for i := range long {
		long[i] = 'a'
	}
	for _, raw := range []string{string(long), "bell\a", string([]byte{0xff})} {
		if _, err := NormalizeNotes(raw); !errors.Is(err, ErrInvalidNotes) {
			t.Errorf("NormalizeNotes(%q) err=%v", raw, err)
		}
	}
}
