//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"testing"
	"time"
)

var (
	ctx        = context.Background()
	manager    = access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.LabManager}
	technician = access.Actor{UserID: "01a00000-0000-7000-8000-000000000002", Role: access.Technician}
	day        = 24 * time.Hour
	t0         = time.Now().UTC().Add(-10 * day).Truncate(time.Second)
)

// fixture creates experiment and subject rows with SQL; adapters of other
// domains are not imported.
type fixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	s    *application.Service
	n    int
}

func newFixture(t *testing.T) *fixture {
	pool := pgtest.Database(t)
	return &fixture{t: t, pool: pool, s: application.New(postgres.NewStore(pool), time.Now)}
}

func (f *fixture) id(sql string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) subject() string {
	f.n++
	return f.id("INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", fmt.Sprintf("S%d", f.n))
}

func (f *fixture) experiment() string {
	f.n++
	return f.id("INSERT INTO misko.experiments (code, title) VALUES ($1, 'Study') RETURNING id", fmt.Sprintf("E%d", f.n))
}

func (f *fixture) group(experimentID, role string) string {
	f.n++
	return f.id("INSERT INTO misko.experiment_groups (experiment_id, name, role) VALUES ($1, $2, $3) RETURNING id", experimentID, fmt.Sprintf("G%d", f.n), role)
}

func (f *fixture) phase(experimentID string, position int) string {
	return f.id("INSERT INTO misko.experiment_phases (experiment_id, name, position) VALUES ($1, $2, $3) RETURNING id", experimentID, fmt.Sprintf("P%d", position), position)
}

// enroll enrolls a subject at t0 and assigns groupID from t0 when given.
func (f *fixture) enroll(experimentID, subjectID, groupID string) string {
	id := f.id("INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at) VALUES ($1, $2, $3) RETURNING id", experimentID, subjectID, t0)
	if groupID != "" {
		f.id("INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from) VALUES ($1, $2, $3, $4) RETURNING id", experimentID, id, groupID, t0)
	}
	return id
}

func (f *fixture) substance(name string) domain.Substance {
	f.t.Helper()
	s, err := f.s.CreateSubstance(ctx, manager, application.CatalogInput{Name: name})
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *fixture) weight(subjectID, grams string, at time.Time) domain.Weight {
	f.t.Helper()
	w, err := f.s.RecordWeight(ctx, technician, subjectID, application.WeightInput{Grams: grams, MeasuredAt: at})
	if err != nil {
		f.t.Fatal(err)
	}
	return w
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestPlansAndAdministrationsKeepReferencesAndHistory(t *testing.T) {
	f := newFixture(t)
	expA, expB := f.experiment(), f.experiment()
	treated, control, foreignGroup := f.group(expA, "TREATMENT"), f.group(expA, "CONTROL"), f.group(expB, "TREATMENT")
	baseline, foreignPhase := f.phase(expA, 1), f.phase(expB, 1)
	s1, s2 := f.subject(), f.subject()
	e1, e2 := f.enroll(expA, s1, treated), f.enroll(expA, s2, control)
	saline := f.substance("Saline")

	plan, err := f.s.CreatePlan(ctx, manager, expA, application.PlanInput{
		GroupID: treated, PhaseID: baseline, SubstanceID: saline.ID, Amount: "10", Unit: "mg/kg", Route: "INTRAPERITONEAL", Schedule: "once daily",
	})
	if err != nil {
		t.Fatal(err)
	}
	if page, err := f.s.ListAdministrations(ctx, manager, application.AdministrationQuery{}); err != nil || page.Total != 0 {
		t.Fatalf("a plan created administrations: %+v %v", page, err)
	}
	foreignPlan, err := f.s.CreatePlan(ctx, manager, expB, application.PlanInput{
		GroupID: foreignGroup, SubstanceID: saline.ID, Amount: "1", Unit: "mg", Route: "ORAL", Schedule: "once",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		input application.PlanInput
		want  error
	}{
		"group of another experiment": {application.PlanInput{GroupID: foreignGroup, SubstanceID: saline.ID, Amount: "1", Unit: "mg", Route: "ORAL", Schedule: "once"}, application.ErrGroupNotFound},
		"phase of another experiment": {application.PlanInput{GroupID: treated, PhaseID: foreignPhase, SubstanceID: saline.ID, Amount: "1", Unit: "mg", Route: "ORAL", Schedule: "once"}, application.ErrPhaseNotFound},
		"unknown substance":           {application.PlanInput{GroupID: treated, SubstanceID: "01a00000-0000-7000-8000-00000000ffff", Amount: "1", Unit: "mg", Route: "ORAL", Schedule: "once"}, application.ErrSubstanceNotFound},
	} {
		if _, err := f.s.CreatePlan(ctx, manager, expA, tc.input); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}

	w1, w2 := f.weight(s1, "25", time.Now().Add(-2*time.Hour)), f.weight(s2, "30", time.Now().Add(-2*time.Hour))
	record := func(experimentID, enrollmentID, planID, weightID string) (domain.Administration, error) {
		return f.s.RecordAdministration(ctx, technician, experimentID, enrollmentID, application.AdministrationInput{
			SubstanceID: saline.ID, PlanID: planID, WeightID: weightID, Amount: "10", Unit: "mg/kg", Route: "INTRAPERITONEAL",
			AdministeredAt: time.Now().Add(-time.Hour),
		})
	}
	given, err := record(expA, e1, plan.ID, w1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if abs, ok := given.AbsoluteDose(); !ok || abs.Amount() != "0.25" || given.SubstanceName != "Saline" || given.SubjectID != s1 || given.RecordedBy != technician.UserID {
		t.Fatalf("recorded administration: %+v abs=%+v", given, abs)
	}
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"subject not in plan group":    {second(record(expA, e2, plan.ID, w2.ID)), application.ErrPlanGroupMismatch},
		"weight of another subject":    {second(record(expA, e1, "", w2.ID)), domain.ErrWeightOtherSubject},
		"plan of another experiment":   {second(record(expA, e1, foreignPlan.ID, w1.ID)), application.ErrPlanNotFound},
		"enrollment through other exp": {second(record(expB, e1, "", w1.ID)), application.ErrEnrollmentNotFound},
		"unknown weight":               {second(record(expA, e1, "", "01a00000-0000-7000-8000-00000000ffff")), application.ErrWeightNotFound},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, tc.err, tc.want)
		}
	}

	// The database rejects the same wrong references when use cases are bypassed.
	insert := "INSERT INTO misko.administrations (experiment_id, enrollment_id, subject_id, substance_id, substance_name, weight_measurement_id, body_milligrams, amount_micro, unit, route, administered_at, recorded_by) VALUES ($1, $2, $3, $4, 'Saline', $5, $6, 10000000, $7, 'ORAL', now(), $8)"
	for name, tc := range map[string]struct {
		subject string
		weight  any
		body    any
		unit    string
		code    string
	}{
		"subject differs from enrollment": {s2, nil, nil, "mg", "23503"},
		"weight of another subject":       {s1, w2.ID, int64(30000), "mg/kg", "23503"},
		"per-kg dose without weight":      {s1, nil, nil, "mg/kg", "23514"},
	} {
		_, err := f.pool.Exec(ctx, insert, expA, e1, tc.subject, saline.ID, tc.weight, tc.body, tc.unit, technician.UserID)
		if pgCode(err) != tc.code {
			t.Errorf("raw insert %s: %v want SQLSTATE %s", name, err, tc.code)
		}
	}
	if _, err := f.pool.Exec(ctx, "DELETE FROM misko.experiment_phases WHERE id = $1", baseline); pgCode(err) != "23503" {
		t.Errorf("phase referenced by a plan was deleted: %v", err)
	}

	// Changing current definitions does not rewrite recorded history.
	renamed, twenty, kg := "Sodium chloride 0.9%", "20", "mg/kg"
	if _, err := f.s.UpdateSubstance(ctx, manager, saline.ID, application.CatalogPatch{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	if updated, err := f.s.UpdatePlan(ctx, manager, expA, plan.ID, application.PlanPatch{Amount: &twenty, Unit: &kg}); err != nil || updated.Dose.Amount() != "20" {
		t.Fatalf("plan update: %+v %v", updated, err)
	}
	history, err := f.s.SubjectAdministrations(ctx, manager, s1, application.AdministrationQuery{})
	if err != nil || history.Total != 1 || history.Administrations[0].SubstanceName != "Saline" || history.Administrations[0].Dose.Amount() != "10" || history.Administrations[0].BodyMilligrams != 25_000 {
		t.Fatalf("history after definition changes: %+v %v", history, err)
	}

	later := time.Now().Add(-30 * time.Minute)
	if _, err := f.s.RecordAdministration(ctx, technician, expA, e2, application.AdministrationInput{SubstanceID: f.substance("Vehicle").ID, Amount: "0.1", Unit: "mL", Route: "ORAL", AdministeredAt: later}); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		query application.AdministrationQuery
		total int
	}{
		"all":                {application.AdministrationQuery{}, 2},
		"by substance":       {application.AdministrationQuery{SubstanceID: saline.ID}, 1},
		"by experiment":      {application.AdministrationQuery{ExperimentID: expA}, 2},
		"other experiment":   {application.AdministrationQuery{ExperimentID: expB}, 0},
		"range before later": {application.AdministrationQuery{From: time.Now().Add(-2 * time.Hour), To: later}, 1},
		"range from later":   {application.AdministrationQuery{From: later}, 1},
	} {
		page, err := f.s.ListAdministrations(ctx, manager, tc.query)
		if err != nil || page.Total != tc.total || len(page.Administrations) != tc.total {
			t.Errorf("%s: %+v %v want %d", name, page, err, tc.total)
		}
	}
}

func TestConditionHistoryFiltersAndReferences(t *testing.T) {
	f := newFixture(t)
	exp := f.experiment()
	s1, s2 := f.subject(), f.subject()
	f.enroll(exp, s1, "")
	e2 := f.enroll(exp, s2, "")
	stroke, err := f.s.CreateDiseaseModel(ctx, manager, application.CatalogInput{Name: "Stroke"})
	if err != nil {
		t.Fatal(err)
	}
	diabetes, err := f.s.CreateDiseaseModel(ctx, manager, application.CatalogInput{Name: "Diabetes"})
	if err != nil {
		t.Fatal(err)
	}
	observe := func(subject, model, status string, ago time.Duration, enrollment string) (domain.Condition, error) {
		return f.s.RecordCondition(ctx, technician, subject, application.ConditionInput{DiseaseModelID: model, Status: status, ObservedAt: time.Now().Add(-ago), EnrollmentID: enrollment})
	}
	for _, o := range []struct {
		subject, model, status string
		ago                    time.Duration
	}{
		{s1, stroke.ID, "INDUCED", 5 * day}, {s1, stroke.ID, "CONFIRMED", 2 * day}, {s2, stroke.ID, "INDUCED", 3 * day}, {s1, diabetes.ID, "INDUCED", day},
	} {
		if _, err := observe(o.subject, o.model, o.status, o.ago, ""); err != nil {
			t.Fatal(err)
		}
	}
	history, err := f.s.SubjectConditions(ctx, manager, s1, "")
	if err != nil || len(history.History) != 3 || len(history.Current) != 2 {
		t.Fatalf("history: %+v %v", history, err)
	}
	current := map[string]domain.ConditionStatus{}
	for _, c := range history.Current {
		current[c.DiseaseModelName] = c.Status
	}
	if current["Stroke"] != domain.Confirmed || current["Diabetes"] != domain.Induced {
		t.Fatalf("current: %v", current)
	}
	subjects := func(q application.ConditionQuery) []string {
		t.Helper()
		page, err := f.s.ListConditions(ctx, manager, q)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, c := range page.Conditions {
			ids = append(ids, c.SubjectID)
		}
		slices.Sort(ids)
		return ids
	}
	if got := subjects(application.ConditionQuery{DiseaseModelID: stroke.ID, Status: "CONFIRMED", CurrentOnly: true}); !slices.Equal(got, []string{s1}) {
		t.Errorf("currently confirmed: %v", got)
	}
	if got := subjects(application.ConditionQuery{DiseaseModelID: stroke.ID, Status: "INDUCED", CurrentOnly: true}); !slices.Equal(got, []string{s2}) {
		t.Errorf("currently induced only: %v", got)
	}
	if got := subjects(application.ConditionQuery{DiseaseModelID: stroke.ID, Status: "INDUCED"}); len(got) != 2 {
		t.Errorf("all inductions: %v", got)
	}

	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"enrollment of another subject": {second(observe(s1, stroke.ID, "RESOLVED", 0, e2)), application.ErrEnrollmentOtherSubject},
		"unknown model":                 {second(observe(s1, "01a00000-0000-7000-8000-00000000ffff", "INDUCED", 0, "")), application.ErrDiseaseModelNotFound},
		"unknown subject":               {second(observe("01a00000-0000-7000-8000-00000000ffff", stroke.ID, "INDUCED", 0, "")), application.ErrSubjectNotFound},
		"duplicate model name":          {second(f.s.CreateDiseaseModel(ctx, manager, application.CatalogInput{Name: "stroke"})), application.ErrNameTaken},
		"weight for unknown subject":    {second(f.s.RecordWeight(ctx, technician, "01a00000-0000-7000-8000-00000000ffff", application.WeightInput{Grams: "20", MeasuredAt: time.Now()})), application.ErrSubjectNotFound},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, tc.err, tc.want)
		}
	}
	if _, err := observe(s2, stroke.ID, "CONFIRMED", 0, e2); err != nil {
		t.Fatalf("condition with own enrollment: %v", err)
	}

	renamed := "Ischemic stroke"
	if _, err := f.s.UpdateDiseaseModel(ctx, manager, stroke.ID, application.CatalogPatch{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	if history, err := f.s.SubjectConditions(ctx, manager, s1, stroke.ID); err != nil || len(history.History) != 2 || history.History[0].DiseaseModelName != "Stroke" {
		t.Fatalf("recorded model names changed: %+v %v", history, err)
	}

	f.weight(s1, "24.5", time.Now().Add(-3*day))
	f.weight(s1, "25.1", time.Now().Add(-day))
	if weights, err := f.s.Weights(ctx, manager, s1); err != nil || len(weights) != 2 || domain.FormatGrams(weights[0].Milligrams) != "25.1" {
		t.Fatalf("weights newest first: %+v %v", weights, err)
	}
}

func second[T any](_ T, err error) error { return err }
