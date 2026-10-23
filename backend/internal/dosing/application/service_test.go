package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/domain"
	"slices"
	"testing"
	"time"
)

var (
	ctx   = context.Background()
	now   = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock = func() time.Time { return now }
)

var errStoreCalled = errors.New("store called")

// guard converts a call on the nil embedded store into errStoreCalled.
func guard(fn func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = errStoreCalled
		}
	}()
	return fn()
}

func TestEveryOperationRequiresItsPermission(t *testing.T) {
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"record administration", access.TestRun, func(s *Service, a access.Actor) error {
			return second(s.RecordAdministration(ctx, a, "x", "a", planned()))
		}},
		{"experiment administrations", access.Read, func(s *Service, a access.Actor) error {
			return second(s.ExperimentAdministrations(ctx, a, "x", AdministrationQuery{}))
		}},
		{"subject administrations", access.Read, func(s *Service, a access.Actor) error {
			return second(s.SubjectAdministrations(ctx, a, "s", AdministrationQuery{}))
		}},
		{"due doses", access.Read, func(s *Service, a access.Actor) error { return second(s.DueDoses(ctx, a, "x", "2026-09-11")) }},
		{"record induction outcome", access.TestRun, func(s *Service, a access.Actor) error {
			return second(s.RecordInductionOutcome(ctx, a, "x", "a", OutcomeInput{Status: "CONFIRMED", AssessedAt: now}))
		}},
		{"induction outcomes", access.Read, func(s *Service, a access.Actor) error { return second(s.InductionOutcomes(ctx, a, "x", "")) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error { return op.run(New(newStore(), clock), access.Actor{UserID: "u", Role: role}) })
			if role.Allows(op.permission) {
				if errors.Is(err, access.ErrForbidden) {
					t.Errorf("%s %s: denied", role, op.name)
				}
				continue
			}
			if !errors.Is(err, access.ErrForbidden) {
				t.Errorf("%s %s: err=%v want forbidden before any store call", role, op.name, err)
			}
		}
	}
	for _, op := range operations {
		if err := guard(func() error { return op.run(New(&fakeStore{}, clock), access.Actor{}) }); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: %v", op.name, err)
		}
	}
}

func TestInvalidInputsNeverReachTheStore(t *testing.T) {
	s := New(&fakeStore{}, clock)
	tech := access.Actor{UserID: "u", Role: access.Technician}
	record := func(mutate func(*AdministrationInput)) error {
		in := unplanned()
		mutate(&in)
		return guard(func() error { return second(s.RecordAdministration(ctx, tech, "x", "a", in)) })
	}
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"zero amount":          {record(func(a *AdministrationInput) { a.Amount = "0" }), domain.ErrInvalidAmount},
		"unknown unit":         {record(func(a *AdministrationInput) { a.Unit = "mg/g" }), domain.ErrInvalidUnit},
		"unknown route":        {record(func(a *AdministrationInput) { a.Route = "IP" }), domain.ErrInvalidRoute},
		"unknown purpose":      {record(func(a *AdministrationInput) { a.Purpose = "RESCUE" }), domain.ErrInvalidPurpose},
		"amount without unit":  {record(func(a *AdministrationInput) { a.Unit = "" }), ErrDoseIncomplete},
		"unplanned no route":   {record(func(a *AdministrationInput) { a.Route = "" }), ErrSubstanceRequired},
		"unplanned no purpose": {record(func(a *AdministrationInput) { a.Purpose = "" }), ErrSubstanceRequired},
		"unplanned no dose":    {record(func(a *AdministrationInput) { a.Amount, a.Unit = "", "" }), ErrSubstanceRequired},
		"unplanned no drug":    {record(func(a *AdministrationInput) { a.SubstanceID = "" }), ErrSubstanceRequired},
		"long notes":           {record(func(a *AdministrationInput) { a.Notes = string(make([]byte, 2001)) }), domain.ErrInvalidNotes},
		"outcome status": {guard(func() error {
			return second(s.RecordInductionOutcome(ctx, tech, "x", "a", OutcomeInput{Status: "INDUCED", AssessedAt: now}))
		}), domain.ErrInvalidOutcome},
		"inverted range": {guard(func() error {
			return second(s.ExperimentAdministrations(ctx, tech, "x", AdministrationQuery{From: now, To: now.Add(-time.Hour)}))
		}), ErrInvalidQuery},
		"bad purpose filter": {guard(func() error {
			return second(s.SubjectAdministrations(ctx, tech, "s", AdministrationQuery{Purpose: "x"}))
		}), ErrInvalidQuery},
		"negative page": {guard(func() error {
			return second(s.SubjectAdministrations(ctx, tech, "s", AdministrationQuery{Page: -1}))
		}), ErrInvalidQuery},
		"bad date":           {guard(func() error { return second(s.DueDoses(ctx, tech, "x", "11.09.2026")) }), domain.ErrInvalidDate},
		"bad outcome filter": {guard(func() error { return second(s.InductionOutcomes(ctx, tech, "x", "ILL")) }), ErrInvalidQuery},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, tc.err, tc.want)
		}
	}
}

func TestPlannedDoseTakesTheRegimenItemAndLatestWeight(t *testing.T) {
	store := newStore()
	tech := access.Actor{UserID: "tech-1", Role: access.Technician}
	a, err := New(store, clock).RecordAdministration(ctx, tech, "x", "a", planned())
	if err != nil {
		t.Fatal(err)
	}
	if a.RegimenItemID != "item" || a.Purpose != domain.Treatment || a.SubstanceID != "drug" || a.SubstanceName != "Compound X" ||
		a.Dose != (domain.Dose{Micro: 10_000_000, Unit: domain.MilligramPerKilogram}) || a.Route != domain.Intraperitoneal ||
		a.WeightID != "latest" || a.BodyMilligrams != 25_000 || a.RecordedBy != "tech-1" ||
		a.ExperimentID != "x" || a.AllocationID != "a" || a.GroupID != "group" || a.SubjectID != "subject" {
		t.Fatalf("planned dose: %+v", a)
	}
	if abs, ok := a.AbsoluteDose(); !ok || abs.Amount() != "0.25" || abs.Unit != domain.Milligram {
		t.Fatalf("computed amount: %+v %v", abs, ok)
	}
	if !slices.Equal(store.calls, []string{"LatestWeight", "CreateAdministration"}) {
		t.Fatalf("calls: %v", store.calls)
	}

	// A named weight replaces the latest one; the actual amount and route may differ from the plan.
	in := planned()
	in.WeightID, in.Amount, in.Unit, in.Route = "named", "5", "mg/kg", "ORAL"
	a, err = New(newStore(), clock).RecordAdministration(ctx, tech, "x", "a", in)
	if err != nil || a.WeightID != "named" || a.BodyMilligrams != 30_000 || a.Dose.Micro != 5_000_000 || a.Route != domain.Oral {
		t.Fatalf("named weight: %+v %v", a, err)
	}

	// An absolute dose needs no weight and looks none up.
	store = newStore()
	in = unplanned()
	if a, err = New(store, clock).RecordAdministration(ctx, tech, "x", "a", in); err != nil || a.WeightID != "" || a.SubstanceName != "Saline" || a.RegimenItemID != "" {
		t.Fatalf("unplanned: %+v %v", a, err)
	}
	if slices.Contains(store.calls, "LatestWeight") {
		t.Fatal("absolute dose looked up a weight")
	}
}

func TestAdministrationRules(t *testing.T) {
	tech := access.Actor{UserID: "tech-1", Role: access.Technician}
	excluded := now.Add(-3 * time.Hour)
	for _, tc := range []struct {
		name   string
		mutate func(*AdministrationInput, *fakeStore)
		want   error
	}{
		{"draft experiment", func(_ *AdministrationInput, f *fakeStore) { f.alloc.ExperimentStatus = "DRAFT" }, ErrExperimentNotRunning},
		{"completed experiment", func(_ *AdministrationInput, f *fakeStore) { f.alloc.ExperimentStatus = "COMPLETED" }, ErrExperimentNotRunning},
		{"before allocation", func(a *AdministrationInput, _ *fakeStore) { a.AdministeredAt = now.Add(-30 * 24 * time.Hour) }, domain.ErrBeforeAllocation},
		{"after exclusion", func(_ *AdministrationInput, f *fakeStore) { f.alloc.ExcludedAt = &excluded }, domain.ErrAfterExclusion},
		{"future", func(a *AdministrationInput, _ *fakeStore) { a.AdministeredAt = now.Add(time.Hour) }, domain.ErrFutureTime},
		{"missing time", func(a *AdministrationInput, _ *fakeStore) { a.AdministeredAt = time.Time{} }, domain.ErrMissingTime},
		{"item of another group", func(_ *AdministrationInput, f *fakeStore) { f.item.GroupID = "other" }, ErrRegimenItemOtherGroup},
		{"no-substance period", func(_ *AdministrationInput, f *fakeStore) {
			f.item = domain.RegimenItem{ID: f.item.ID, ExperimentID: f.item.ExperimentID, GroupID: f.item.GroupID, Purpose: domain.NoSubstance, StartDay: 0, EndDay: 14}
		}, ErrNoSubstanceItem},
		{"other substance than planned", func(a *AdministrationInput, _ *fakeStore) { a.SubstanceID = "saline" }, ErrSubstanceMismatch},
		{"other purpose than planned", func(a *AdministrationInput, _ *fakeStore) { a.Purpose = "VEHICLE" }, ErrPurposeMismatch},
		{"no weight yet", func(_ *AdministrationInput, f *fakeStore) { f.latest = nil }, domain.ErrWeightRequired},
		{"stale latest weight", func(_ *AdministrationInput, f *fakeStore) { f.latest.MeasuredAt = now.Add(-8 * 24 * time.Hour) }, domain.ErrWeightNotRelevant},
		{"weight of another animal", func(a *AdministrationInput, _ *fakeStore) { a.WeightID = "foreign" }, domain.ErrWeightOtherSubject},
		{"weight after the dose", func(a *AdministrationInput, _ *fakeStore) {
			a.WeightID, a.AdministeredAt = "named", now.Add(-3*time.Hour)
		}, domain.ErrWeightNotRelevant},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore()
			in := planned()
			tc.mutate(&in, store)
			if _, err := New(store, clock).RecordAdministration(ctx, tech, "x", "a", in); !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
			if slices.Contains(store.calls, "CreateAdministration") {
				t.Fatal("rejected administration was stored")
			}
		})
	}
}

func TestInductionOutcome(t *testing.T) {
	tech := access.Actor{UserID: "tech-1", Role: access.Technician}
	store := newStore()
	o, err := New(store, clock).RecordInductionOutcome(ctx, tech, "x", "a", OutcomeInput{Status: "NOT_CONFIRMED", Note: " weak ", AssessedAt: now})
	if err != nil || o.AllocationID != "a" || o.ExperimentID != "x" || o.Status != domain.NotConfirmed || o.Note != "weak" || o.RecordedBy != "tech-1" {
		t.Fatalf("outcome: %+v %v", o, err)
	}
	for name, mutate := range map[string]func(*fakeStore){
		"healthy control": func(f *fakeStore) { f.alloc.HasDisease = false },
		"not running":     func(f *fakeStore) { f.alloc.ExperimentStatus = "COMPLETED" },
	} {
		store := newStore()
		mutate(store)
		_, err := New(store, clock).RecordInductionOutcome(ctx, tech, "x", "a", OutcomeInput{Status: "CONFIRMED", AssessedAt: now})
		if err == nil || slices.Contains(store.calls, "CreateInductionOutcome") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestDueDosesNeedDayZeroAndDefaultToToday(t *testing.T) {
	reader := access.Actor{UserID: "u", Role: access.Viewer}
	store := newStore()
	store.day = ExperimentDay{Status: "DRAFT", Today: now}
	if _, err := New(store, clock).DueDoses(ctx, reader, "x", ""); !errors.Is(err, ErrExperimentNotStarted) {
		t.Fatalf("draft: %v", err)
	}
	zero := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	store.day = ExperimentDay{Status: "RUNNING", DayZero: &zero, Today: today}
	store.candidates = []domain.DueCandidate{{
		Allocation: domain.Allocation{ID: "a"},
		Item:       domain.RegimenItem{ID: "item", Frequency: domain.Daily, StartDay: 1, EndDay: 3, Dose: domain.Dose{Micro: 1, Unit: domain.Milligram}},
	}}
	s, err := New(store, clock).DueDoses(ctx, reader, "x", "")
	if err != nil || !s.Date.Equal(today) || s.StudyDay != 1 || len(s.Animals) != 1 || !store.asked.Equal(today) {
		t.Fatalf("today: %+v %v", s, err)
	}
	s, err = New(store, clock).DueDoses(ctx, reader, "x", "2026-09-20")
	if err != nil || s.StudyDay != 10 || len(s.Animals) != 0 {
		t.Fatalf("after the regimen: %+v %v", s, err)
	}
}

func planned() AdministrationInput {
	return AdministrationInput{RegimenItemID: "item", AdministeredAt: now.Add(-time.Hour)}
}

func unplanned() AdministrationInput {
	return AdministrationInput{Purpose: "VEHICLE", SubstanceID: "saline", Amount: "0.2", Unit: "mL", Route: "ORAL", AdministeredAt: now.Add(-time.Hour)}
}

// fakeStore implements only what these tests use; any other call panics and
// guard reports it as a store call.
type fakeStore struct {
	Store
	calls      []string
	alloc      AllocationRef
	item       domain.RegimenItem
	latest     *domain.Weight
	day        ExperimentDay
	candidates []domain.DueCandidate
	asked      time.Time
}

func newStore() *fakeStore {
	return &fakeStore{
		alloc: AllocationRef{
			Allocation:       domain.Allocation{ID: "a", ExperimentID: "x", GroupID: "group", SubjectID: "subject", AllocatedAt: now.Add(-10 * 24 * time.Hour)},
			ExperimentStatus: "RUNNING", HasDisease: true,
		},
		item: domain.RegimenItem{
			ID: "item", ExperimentID: "x", GroupID: "group", Purpose: domain.Treatment, SubstanceID: "drug", SubstanceName: "Compound X",
			Dose: domain.Dose{Micro: 10_000_000, Unit: domain.MilligramPerKilogram}, Route: domain.Intraperitoneal, Frequency: domain.Daily, StartDay: 1, EndDay: 14,
		},
		latest: &domain.Weight{ID: "latest", SubjectID: "subject", Milligrams: 25_000, MeasuredAt: now.Add(-2 * time.Hour)},
	}
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error { return fn(f) }

func (f *fakeStore) Allocation(context.Context, string, string) (AllocationRef, error) {
	return f.alloc, nil
}

func (f *fakeStore) RegimenItem(context.Context, string, string) (domain.RegimenItem, error) {
	return f.item, nil
}

func (f *fakeStore) SubstanceName(context.Context, string) (string, error) { return "Saline", nil }

func (f *fakeStore) Weight(_ context.Context, id string) (domain.Weight, error) {
	subject := "subject"
	if id == "foreign" {
		subject = "someone-else"
	}
	return domain.Weight{ID: id, SubjectID: subject, Milligrams: 30_000, MeasuredAt: now.Add(-2 * time.Hour)}, nil
}

func (f *fakeStore) LatestWeight(context.Context, string, time.Time) (*domain.Weight, error) {
	f.calls = append(f.calls, "LatestWeight")
	return f.latest, nil
}

func (f *fakeStore) CreateAdministration(_ context.Context, a domain.Administration) (domain.Administration, error) {
	f.calls = append(f.calls, "CreateAdministration")
	return a, nil
}

func (f *fakeStore) ExperimentDay(context.Context, string) (ExperimentDay, error) { return f.day, nil }

func (f *fakeStore) DueCandidates(_ context.Context, _ string, day time.Time) ([]domain.DueCandidate, error) {
	f.asked = day
	return f.candidates, nil
}

func (f *fakeStore) CreateInductionOutcome(_ context.Context, o domain.InductionOutcome) (domain.InductionOutcome, error) {
	f.calls = append(f.calls, "CreateInductionOutcome")
	return o, nil
}

func (f *fakeStore) ListAdministrations(context.Context, AdministrationFilter) ([]domain.Administration, int, error) {
	return nil, 0, nil
}

func (f *fakeStore) ListInductionOutcomes(context.Context, string, OutcomeFilter) ([]OutcomeRow, error) {
	return nil, nil
}

func second[T any](_ T, err error) error { return err }
