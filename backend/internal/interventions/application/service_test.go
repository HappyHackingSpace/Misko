package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/domain"
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
	name := "Renamed"
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"list disease models", access.Read, func(s *Service, a access.Actor) error { return second(s.DiseaseModels(ctx, a)) }},
		{"create disease model", access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.CreateDiseaseModel(ctx, a, CatalogInput{Name: "Stroke"}))
		}},
		{"update disease model", access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.UpdateDiseaseModel(ctx, a, "m", CatalogPatch{Name: &name}))
		}},
		{"list substances", access.Read, func(s *Service, a access.Actor) error { return second(s.Substances(ctx, a)) }},
		{"create substance", access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.CreateSubstance(ctx, a, CatalogInput{Name: "Saline"}))
		}},
		{"update substance", access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.UpdateSubstance(ctx, a, "s", CatalogPatch{Name: &name}))
		}},
		{"list plans", access.Read, func(s *Service, a access.Actor) error { return second(s.Plans(ctx, a, "x")) }},
		{"create plan", access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.CreatePlan(ctx, a, "x", validPlan()))
		}},
		{"update plan", access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.UpdatePlan(ctx, a, "x", "p", PlanPatch{Schedule: &name}))
		}},
		{"record weight", access.WeightWrite, func(s *Service, a access.Actor) error {
			return second(s.RecordWeight(ctx, a, "s", WeightInput{Grams: "25", MeasuredAt: now}))
		}},
		{"list weights", access.Read, func(s *Service, a access.Actor) error { return second(s.Weights(ctx, a, "s")) }},
		{"record condition", access.TestRun, func(s *Service, a access.Actor) error {
			return second(s.RecordCondition(ctx, a, "s", ConditionInput{DiseaseModelID: "m", Status: "INDUCED", ObservedAt: now}))
		}},
		{"subject conditions", access.Read, func(s *Service, a access.Actor) error { return second(s.SubjectConditions(ctx, a, "s", "")) }},
		{"list conditions", access.Read, func(s *Service, a access.Actor) error { return second(s.ListConditions(ctx, a, ConditionQuery{})) }},
		{"record administration", access.TestRun, func(s *Service, a access.Actor) error {
			return second(s.RecordAdministration(ctx, a, "x", "e", validAdministration()))
		}},
		{"subject administrations", access.Read, func(s *Service, a access.Actor) error {
			return second(s.SubjectAdministrations(ctx, a, "s", AdministrationQuery{}))
		}},
		{"list administrations", access.Read, func(s *Service, a access.Actor) error {
			return second(s.ListAdministrations(ctx, a, AdministrationQuery{}))
		}},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error { return op.run(New(&fakeStore{}, clock), access.Actor{UserID: "u", Role: role}) })
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
	manager := access.Actor{UserID: "u", Role: access.LabManager}
	amountOnly := "5"
	plan := func(mutate func(*PlanInput)) PlanInput { p := validPlan(); mutate(&p); return p }
	record := func(mutate func(*AdministrationInput)) AdministrationInput {
		a := validAdministration()
		mutate(&a)
		return a
	}
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"plan zero amount":      {second(s.CreatePlan(ctx, manager, "x", plan(func(p *PlanInput) { p.Amount = "0" }))), domain.ErrInvalidAmount},
		"plan unknown unit":     {second(s.CreatePlan(ctx, manager, "x", plan(func(p *PlanInput) { p.Unit = "mg/g" }))), domain.ErrInvalidUnit},
		"plan unknown route":    {second(s.CreatePlan(ctx, manager, "x", plan(func(p *PlanInput) { p.Route = "IP" }))), domain.ErrInvalidRoute},
		"plan amount only":      {second(s.UpdatePlan(ctx, manager, "x", "p", PlanPatch{Amount: &amountOnly})), ErrDoseIncomplete},
		"plan empty patch":      {second(s.UpdatePlan(ctx, manager, "x", "p", PlanPatch{})), ErrNoChanges},
		"weight malformed":      {second(s.RecordWeight(ctx, manager, "s", WeightInput{Grams: "25.1234", MeasuredAt: now})), domain.ErrInvalidWeight},
		"weight without time":   {second(s.RecordWeight(ctx, manager, "s", WeightInput{Grams: "25"})), domain.ErrMissingTime},
		"condition future":      {second(s.RecordCondition(ctx, manager, "s", ConditionInput{DiseaseModelID: "m", Status: "INDUCED", ObservedAt: now.Add(time.Hour)})), domain.ErrFutureTime},
		"condition status":      {second(s.RecordCondition(ctx, manager, "s", ConditionInput{DiseaseModelID: "m", Status: "ILL", ObservedAt: now})), domain.ErrInvalidStatus},
		"administration amount": {second(s.RecordAdministration(ctx, manager, "x", "e", record(func(a *AdministrationInput) { a.Amount = "-1" }))), domain.ErrInvalidAmount},
		"administration time":   {second(s.RecordAdministration(ctx, manager, "x", "e", record(func(a *AdministrationInput) { a.AdministeredAt = time.Time{} }))), domain.ErrMissingTime},
		"administration future": {second(s.RecordAdministration(ctx, manager, "x", "e", record(func(a *AdministrationInput) { a.AdministeredAt = now.Add(time.Hour) }))), domain.ErrFutureTime},
		"list bad status":       {second(s.ListConditions(ctx, manager, ConditionQuery{Status: "ILL"})), ErrInvalidQuery},
		"list inverted range":   {second(s.ListAdministrations(ctx, manager, AdministrationQuery{From: now, To: now.Add(-time.Hour)})), ErrInvalidQuery},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, tc.err, tc.want)
		}
	}
}

func TestCreatingAPlanRecordsNoAdministration(t *testing.T) {
	store := &fakeStore{}
	manager := access.Actor{UserID: "u", Role: access.LabManager}
	if _, err := New(store, clock).CreatePlan(ctx, manager, "x", validPlan()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"CreatePlan"}) {
		t.Fatalf("store calls: %v", store.calls)
	}
}

func TestAdministrationReferencesAndSnapshots(t *testing.T) {
	technician := access.Actor{UserID: "tech-1", Role: access.Technician}
	for _, tc := range []struct {
		name   string
		mutate func(*AdministrationInput, *fakeStore)
		want   error
	}{
		{"plan for another group", func(a *AdministrationInput, f *fakeStore) { a.PlanID = "plan"; f.groupAt = "other-group" }, ErrPlanGroupMismatch},
		{"plan while unassigned", func(a *AdministrationInput, f *fakeStore) { a.PlanID = "plan"; f.groupAt = "" }, ErrPlanGroupMismatch},
		{"plan of another substance", func(a *AdministrationInput, f *fakeStore) { a.PlanID = "plan"; a.SubstanceID = "other-substance" }, ErrPlanSubstanceMismatch},
		{"weight of another subject", func(a *AdministrationInput, f *fakeStore) { a.WeightID = "foreign-weight" }, domain.ErrWeightOtherSubject},
		{"per-kg dose without weight", func(a *AdministrationInput, f *fakeStore) { a.Unit, a.WeightID = "mg/kg", "" }, domain.ErrWeightRequired},
		{"before enrollment", func(a *AdministrationInput, f *fakeStore) { a.AdministeredAt = now.Add(-30 * 24 * time.Hour) }, domain.ErrBeforeEnrollment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{groupAt: "group"}
			input := validAdministration()
			tc.mutate(&input, store)
			if _, err := New(store, clock).RecordAdministration(ctx, technician, "x", "e", input); !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
			if slices.Contains(store.calls, "CreateAdministration") {
				t.Fatal("rejected administration was stored")
			}
		})
	}
	store := &fakeStore{groupAt: "group"}
	input := validAdministration()
	input.PlanID = "plan"
	a, err := New(store, clock).RecordAdministration(ctx, technician, "x", "e", input)
	if err != nil {
		t.Fatal(err)
	}
	if a.SubjectID != "subject" || a.SubstanceName != "Saline" || a.BodyMilligrams != 25_000 || a.RecordedBy != "tech-1" || a.Dose.Unit != domain.MilligramPerKilogram {
		t.Fatalf("snapshot: %+v", a)
	}
	if abs, ok := a.AbsoluteDose(); !ok || abs.Amount() != "0.25" || abs.Unit != domain.Milligram {
		t.Fatalf("absolute dose: %+v %v", abs, ok)
	}
}

func validPlan() PlanInput {
	return PlanInput{GroupID: "group", SubstanceID: "substance", Amount: "10", Unit: "mg/kg", Route: "INTRAPERITONEAL", Schedule: "daily for 14 days"}
}

func validAdministration() AdministrationInput {
	return AdministrationInput{SubstanceID: "substance", WeightID: "weight", Amount: "10", Unit: "mg/kg", Route: "INTRAPERITONEAL", AdministeredAt: now.Add(-time.Hour)}
}

// fakeStore implements only what these tests use; any other call panics and
// guard reports it as a store call.
type fakeStore struct {
	Store
	calls   []string
	groupAt string
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error { return fn(f) }

func (f *fakeStore) CreatePlan(_ context.Context, p domain.Plan) (domain.Plan, error) {
	f.calls = append(f.calls, "CreatePlan")
	return p, nil
}

func (f *fakeStore) Enrollment(context.Context, string, string) (EnrollmentRef, error) {
	return EnrollmentRef{ID: "e", ExperimentID: "x", SubjectID: "subject", EnrolledAt: now.Add(-10 * 24 * time.Hour)}, nil
}

func (f *fakeStore) Substance(_ context.Context, id string) (domain.Substance, error) {
	return domain.Substance{ID: id, Name: "Saline"}, nil
}

func (f *fakeStore) Weight(_ context.Context, id string) (domain.Weight, error) {
	subject := "subject"
	if id == "foreign-weight" {
		subject = "someone-else"
	}
	return domain.Weight{ID: id, SubjectID: subject, Milligrams: 25_000, MeasuredAt: now.Add(-2 * time.Hour)}, nil
}

func (f *fakeStore) Plan(context.Context, string, string) (domain.Plan, error) {
	dose, _ := domain.ParseDose("10", "mg/kg")
	return domain.Plan{ID: "plan", ExperimentID: "x", GroupID: "group", SubstanceID: "substance", Dose: dose, Route: domain.Intraperitoneal}, nil
}

func (f *fakeStore) GroupAt(context.Context, string, time.Time) (string, error) {
	return f.groupAt, nil
}

func (f *fakeStore) CreateAdministration(_ context.Context, a domain.Administration) (domain.Administration, error) {
	f.calls = append(f.calls, "CreateAdministration")
	return a, nil
}

func second[T any](_ T, err error) error { return err }
