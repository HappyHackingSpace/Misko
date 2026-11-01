package domain

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func date(t *testing.T, raw string) time.Time {
	t.Helper()
	d, err := ParseDate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParseDateAndStudyDay(t *testing.T) {
	for _, raw := range []string{"", "2026-13-01", "2026-02-30", "26-01-01", "2026-01-01T00:00:00Z", "today"} {
		if _, err := ParseDate(raw); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseDate(%q) err=%v", raw, err)
		}
	}
	zero := date(t, "2026-03-27")
	for raw, want := range map[string]int{
		"2026-03-27": 0, "2026-03-28": 1, "2026-03-26": -1, "2026-04-10": 14,
		// Across the European daylight saving change (29 March) and a leap day.
		"2026-03-30": 3, "2028-03-01": 705, "2025-03-27": -365,
	} {
		if got := StudyDay(zero, date(t, raw)); got != want {
			t.Errorf("StudyDay(%s)=%d want %d", raw, got, want)
		}
	}
	// Day 0 given with a time of day still counts calendar days.
	if got := StudyDay(time.Date(2026, 3, 27, 23, 0, 0, 0, time.UTC), date(t, "2026-03-28")); got != 1 {
		t.Fatalf("calendar days: %d", got)
	}
}

// days lists the study days in [from, to] with a dose due and how many.
func days(r RegimenItem, from, to int) map[int]int {
	out := map[int]int{}
	for d := from; d <= to; d++ {
		if n := r.DosesOn(d); n > 0 {
			out[d] = n
		}
	}
	return out
}

func TestDosesDuePerFrequency(t *testing.T) {
	for _, tc := range []struct {
		name string
		item RegimenItem
		want map[int]int
	}{
		{"once on day 0", RegimenItem{Frequency: Once, StartDay: 0, EndDay: 0}, map[int]int{0: 1}},
		{"once before day 0", RegimenItem{Frequency: Once, StartDay: -3, EndDay: -3}, map[int]int{-3: 1}},
		{"daily 1 to 4", RegimenItem{Frequency: Daily, StartDay: 1, EndDay: 4}, map[int]int{1: 1, 2: 1, 3: 1, 4: 1}},
		{"daily across day 0", RegimenItem{Frequency: Daily, StartDay: -1, EndDay: 1}, map[int]int{-1: 1, 0: 1, 1: 1}},
		{"twice daily 2 to 3", RegimenItem{Frequency: TwiceDaily, StartDay: 2, EndDay: 3}, map[int]int{2: 2, 3: 2}},
		{"weekly from day 1", RegimenItem{Frequency: Weekly, StartDay: 1, EndDay: 21}, map[int]int{1: 1, 8: 1, 15: 1}},
		{"weekly ends on a dose day", RegimenItem{Frequency: Weekly, StartDay: 0, EndDay: 14}, map[int]int{0: 1, 7: 1, 14: 1}},
		{"every 3 days", RegimenItem{Frequency: EveryNDays, IntervalDays: 3, StartDay: 2, EndDay: 10}, map[int]int{2: 1, 5: 1, 8: 1}},
		{"every 2 days from a negative day", RegimenItem{Frequency: EveryNDays, IntervalDays: 2, StartDay: -3, EndDay: 1}, map[int]int{-3: 1, -1: 1, 1: 1}},
		{"every n days without an interval", RegimenItem{Frequency: EveryNDays, StartDay: 0, EndDay: 5}, map[int]int{}},
		{"unknown frequency", RegimenItem{Frequency: "HOURLY", StartDay: 0, EndDay: 5}, map[int]int{}},
		{"no-substance period", RegimenItem{Purpose: NoSubstance, StartDay: 0, EndDay: 14}, map[int]int{}},
		{"no-substance period with a stray frequency", RegimenItem{Purpose: NoSubstance, Frequency: Daily, StartDay: 0, EndDay: 14}, map[int]int{}},
	} {
		got := days(tc.item, -30, 30)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
			continue
		}
		for d, n := range tc.want {
			if got[d] != n {
				t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}

func TestBuildScheduleGroupsDueDosesPerAnimal(t *testing.T) {
	zero := date(t, "2026-03-27")
	mptp := RegimenItem{ID: "induction", Purpose: Induction, Dose: Dose{Micro: 20_000_000, Unit: MilligramPerKilogram}, Frequency: Once, StartDay: 0, EndDay: 0}
	saline := RegimenItem{ID: "vehicle", Purpose: Vehicle, Dose: Dose{Micro: 10_000_000, Unit: MilliliterPerKilogram}, Frequency: Daily, StartDay: 1, EndDay: 14}
	drug := RegimenItem{ID: "treatment", Purpose: Treatment, Dose: Dose{Micro: 1_500_000, Unit: Milligram}, Frequency: TwiceDaily, StartDay: 1, EndDay: 14}
	mouse := func(id string) Allocation { return Allocation{ID: id, SubjectID: "s-" + id, GroupID: "g"} }
	weight := &Weight{ID: "w1", SubjectID: "s-a1", Milligrams: 25_000}
	candidates := []DueCandidate{
		{Allocation: mouse("a1"), SubjectCode: "M1", Weight: weight, Item: mptp},
		{Allocation: mouse("a1"), SubjectCode: "M1", Weight: weight, Item: saline, Given: 1},
		{Allocation: mouse("a1"), SubjectCode: "M1", Weight: weight, Item: drug, Given: 1},
		{Allocation: mouse("a2"), SubjectCode: "M2", Item: mptp},
		{Allocation: mouse("a2"), SubjectCode: "M2", Item: saline},
		// A healthy control's no-substance period is never due.
		{Allocation: mouse("a3"), SubjectCode: "M3", Item: RegimenItem{ID: "period", Purpose: NoSubstance, StartDay: -5, EndDay: 30}},
	}

	s := BuildSchedule(zero, date(t, "2026-03-28"), candidates)
	if s.StudyDay != 1 || len(s.Animals) != 2 {
		t.Fatalf("day 1: %+v", s)
	}
	first := s.Animals[0]
	if first.Allocation.ID != "a1" || first.Weight != weight || len(first.Doses) != 2 {
		t.Fatalf("first animal: %+v", first)
	}
	vehicle, treatment := first.Doses[0], first.Doses[1]
	if vehicle.Item.ID != "vehicle" || vehicle.Due != 1 || vehicle.Given != 1 || !vehicle.Complete() ||
		vehicle.AbsoluteDose == nil || vehicle.AbsoluteDose.Amount() != "0.25" || vehicle.AbsoluteDose.Unit != Milliliter {
		t.Fatalf("vehicle: %+v %+v", vehicle, vehicle.AbsoluteDose)
	}
	if treatment.Due != 2 || treatment.Given != 1 || treatment.Complete() || *treatment.AbsoluteDose != drug.Dose {
		t.Fatalf("treatment: %+v", treatment)
	}
	second := s.Animals[1]
	if second.Allocation.ID != "a2" || len(second.Doses) != 1 || second.Doses[0].AbsoluteDose != nil || second.Doses[0].Complete() {
		t.Fatalf("an animal without weight gets no computed per-kg amount: %+v", second)
	}

	day0 := BuildSchedule(zero, zero, candidates)
	ids := []string{}
	for _, a := range day0.Animals {
		for _, d := range a.Doses {
			ids = append(ids, a.Allocation.ID+"/"+d.Item.ID)
		}
	}
	if day0.StudyDay != 0 || !slices.Equal(ids, []string{"a1/induction", "a2/induction"}) {
		t.Fatalf("day 0: %v", ids)
	}
	if abs := day0.Animals[0].Doses[0].AbsoluteDose; abs == nil || abs.Amount() != "0.5" || abs.Unit != Milligram {
		t.Fatalf("induction amount: %+v", abs)
	}

	if empty := BuildSchedule(zero, date(t, "2026-04-20"), candidates); empty.StudyDay != 24 || empty.Animals == nil || len(empty.Animals) != 0 {
		t.Fatalf("after the regimen: %+v", empty)
	}
	if none := BuildSchedule(zero, zero, nil); none.Animals == nil || len(none.Animals) != 0 {
		t.Fatalf("no candidates: %+v", none)
	}
}
