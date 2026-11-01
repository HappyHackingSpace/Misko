//go:build integration

package postgres_test

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"testing"
	"time"
)

var (
	ctx        = context.Background()
	user       = "01a00000-0000-7000-8000-000000000001"
	technician = access.Actor{UserID: user, Role: access.Technician}
	viewer     = access.Actor{UserID: user, Role: access.Viewer}
	// The tests run as if it were this instant, ten days after day 0.
	clock = func() time.Time { return time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) }
)

// study is a running experiment built with plain SQL, since the contexts that
// own experiments, catalog and weights are not imported here.
type study struct {
	t                                      *testing.T
	pool                                   *pgxpool.Pool
	exp, draft                             string
	healthy, disease                       string
	saline, mptp                           string
	healthyVehicle, induction, sickVehicle string
	healthyPeriod                          string
	m1, m2, m3, m4                         string
	a1, a2, a3, a4, draftAllocation        string
}

func (s *study) id(sql string, args ...any) string {
	s.t.Helper()
	var id string
	if err := s.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		s.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (s *study) exec(sql string, args ...any) {
	s.t.Helper()
	if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
		s.t.Fatalf("%s: %v", sql, err)
	}
}

// newStudy builds a study with day 0 on 1 March 2026 in a laboratory at UTC+3:
// G1 healthy (no substance on days -5 to 30, and saline 10 mL/kg daily on days
// 1 to 5 written before no-substance periods existed) and G2 disease (MPTP
// 20 mg/kg once on day 0, saline daily on days 1 to 5).
func newStudy(t *testing.T) *study {
	s := &study{t: t, pool: pgtest.Database(t)}
	s.exec(`INSERT INTO misko.laboratory (name, timezone) VALUES ('Lab', 'Europe/Istanbul')`)
	s.saline = s.id(`INSERT INTO misko.substances (name, kind) VALUES ('Saline', 'VEHICLE') RETURNING id`)
	s.mptp = s.id(`INSERT INTO misko.substances (name, kind) VALUES ('MPTP', 'INDUCER') RETURNING id`)
	model := s.id(`INSERT INTO misko.disease_models (name, induction_type) VALUES ('MPTP Parkinson', 'CHEMICAL') RETURNING id`)
	s.exp = s.id(`INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id`)
	s.draft = s.id(`INSERT INTO misko.experiments (code, title) VALUES ('E2', 'Draft') RETURNING id`)
	s.healthy = s.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, target_size) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL', 4) RETURNING id`, s.exp)
	s.disease = s.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id, target_size)
		VALUES ($1, 'G2', 'MPTP', 'DISEASE_CONTROL', $2, 4) RETURNING id`, s.exp, model)
	draftGroup := s.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, s.draft)
	item := func(group, purpose, substance string, dose int64, unit, frequency string, start, end int) string {
		return s.id(`INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
			VALUES ($1, $2, $3, $4, $5, $6, 'INTRAPERITONEAL', $7, $8, $9) RETURNING id`, s.exp, group, purpose, substance, dose, unit, frequency, start, end)
	}
	// A healthy control's vehicle from before migration 020: a started study
	// keeps it and doses it, though no new one can be written.
	s.exec(`ALTER TABLE misko.regimen_items DISABLE TRIGGER regimen_items_role`)
	s.healthyVehicle = item(s.healthy, "VEHICLE", s.saline, 10_000_000, "mL/kg", "DAILY", 1, 5)
	s.exec(`ALTER TABLE misko.regimen_items ENABLE TRIGGER regimen_items_role`)
	s.healthyPeriod = s.id(`INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, start_day, end_day)
		VALUES ($1, $2, 'NONE', -5, 30) RETURNING id`, s.exp, s.healthy)
	s.induction = item(s.disease, "INDUCTION", s.mptp, 20_000_000, "mg/kg", "ONCE", 0, 0)
	s.sickVehicle = item(s.disease, "VEHICLE", s.saline, 10_000_000, "mL/kg", "DAILY", 1, 5)
	animal := func(code string) string {
		return s.id(`INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id`, code)
	}
	s.m1, s.m2, s.m3, s.m4 = animal("M1"), animal("M2"), animal("M3"), animal("M4")
	allocate := func(exp, group, subject, at string) string {
		return s.id(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			exp, group, subject, at, user)
	}
	s.a1 = allocate(s.exp, s.healthy, s.m1, "2026-02-20T08:00:00Z")
	s.a2 = allocate(s.exp, s.disease, s.m2, "2026-02-20T08:00:00Z")
	s.a3 = allocate(s.exp, s.disease, s.m3, "2026-02-20T08:00:00Z")
	// Allocated late, on 3 March, after the study started.
	s.a4 = allocate(s.exp, s.healthy, s.m4, "2026-03-03T08:00:00Z")
	s.draftAllocation = allocate(s.draft, draftGroup, animal("M5"), "2026-02-20T08:00:00Z")
	s.exec(`UPDATE misko.experiments SET status = 'RUNNING', day_zero = '2026-03-01', started_at = '2026-02-28T12:00:00Z' WHERE id = $1`, s.exp)
	// M3 died at noon UTC on 2 March.
	s.exec(`UPDATE misko.allocations SET excluded_at = '2026-03-02T12:00:00Z', exclusion_reason = 'DIED' WHERE id = $1`, s.a3)
	return s
}

func (s *study) weight(subject string, milligrams int, at string) string {
	return s.id(`INSERT INTO misko.weight_measurements (subject_id, body_milligrams, measured_at, recorded_by) VALUES ($1, $2, $3, $4) RETURNING id`,
		subject, milligrams, at, user)
}

func at(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDatabaseRulesMapToErrors(t *testing.T) {
	s := newStudy(t)
	store := postgres.NewStore(s.pool)
	w2 := s.weight(s.m2, 24_000, "2026-02-28T08:00:00Z")
	base := domain.Administration{
		ExperimentID: s.exp, AllocationID: s.a2, GroupID: s.disease, SubjectID: s.m2, Purpose: domain.Vehicle, SubstanceID: s.saline,
		Dose: domain.Dose{Micro: 200_000, Unit: domain.Milliliter}, Route: domain.Oral, AdministeredAt: at("2026-03-02T08:00:00Z"), RecordedBy: user,
	}
	for _, tc := range []struct {
		name   string
		mutate func(*domain.Administration)
		want   error
	}{
		{"draft experiment", func(a *domain.Administration) {
			a.ExperimentID, a.AllocationID, a.GroupID = s.draft, s.draftAllocation, ""
		}, application.ErrExperimentNotRunning},
		{"allocation of another experiment", func(a *domain.Administration) { a.AllocationID = s.draftAllocation }, application.ErrAllocationNotFound},
		{"group other than the allocation's", func(a *domain.Administration) { a.GroupID = s.healthy }, application.ErrAllocationNotFound},
		{"regimen item of another group", func(a *domain.Administration) { a.RegimenItemID = s.healthyVehicle }, application.ErrRegimenItemOtherGroup},
		{"no-substance period", func(a *domain.Administration) {
			a.AllocationID, a.GroupID, a.SubjectID, a.RegimenItemID = s.a1, s.healthy, s.m1, s.healthyPeriod
		}, application.ErrNoSubstanceItem},
		{"per-kg dose without weight", func(a *domain.Administration) { a.Dose.Unit = domain.MilliliterPerKilogram }, domain.ErrWeightRequired},
		{"weight of another animal", func(a *domain.Administration) {
			a.WeightID = s.weight(s.m1, 25_000, "2026-02-28T08:00:00Z")
		}, domain.ErrWeightOtherSubject},
		{"unknown substance", func(a *domain.Administration) { a.SubstanceID = "01a00000-0000-7000-8000-0000000000ff" }, application.ErrSubstanceNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := base
			tc.mutate(&a)
			if a.GroupID == "" {
				var group string
				if err := s.pool.QueryRow(ctx, `SELECT group_id FROM misko.allocations WHERE id = $1`, a.AllocationID).Scan(&group); err != nil {
					t.Fatal(err)
				}
				a.GroupID = group
				a.SubjectID = ""
				if err := s.pool.QueryRow(ctx, `SELECT subject_id FROM misko.allocations WHERE id = $1`, a.AllocationID).Scan(&a.SubjectID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.CreateAdministration(ctx, a); !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
	ok := base
	ok.RegimenItemID, ok.Dose, ok.WeightID = s.sickVehicle, domain.Dose{Micro: 10_000_000, Unit: domain.MilliliterPerKilogram}, w2
	if _, err := store.CreateAdministration(ctx, ok); err != nil {
		t.Fatalf("valid administration: %v", err)
	}

	outcome := domain.InductionOutcome{AllocationID: s.a2, ExperimentID: s.exp, Status: domain.Confirmed, AssessedAt: at("2026-03-05T08:00:00Z"), RecordedBy: user}
	if _, err := store.CreateInductionOutcome(ctx, outcome); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateInductionOutcome(ctx, outcome); !errors.Is(err, application.ErrOutcomeExists) {
		t.Fatalf("second outcome: %v", err)
	}
	draft := domain.InductionOutcome{AllocationID: s.draftAllocation, ExperimentID: s.draft, Status: domain.Confirmed, AssessedAt: outcome.AssessedAt, RecordedBy: user}
	if _, err := store.CreateInductionOutcome(ctx, draft); !errors.Is(err, application.ErrExperimentNotRunning) {
		t.Fatalf("outcome in a draft: %v", err)
	}
	other := domain.InductionOutcome{AllocationID: s.draftAllocation, ExperimentID: s.exp, Status: domain.Confirmed, AssessedAt: outcome.AssessedAt, RecordedBy: user}
	if _, err := store.CreateInductionOutcome(ctx, other); !errors.Is(err, application.ErrAllocationNotFound) {
		t.Fatalf("outcome for another experiment's animal: %v", err)
	}
}

func TestRecordAndListThroughTheService(t *testing.T) {
	s := newStudy(t)
	service := application.New(postgres.NewStore(s.pool), clock)
	s.weight(s.m2, 24_000, "2026-02-27T08:00:00Z")
	latest := s.weight(s.m2, 25_000, "2026-02-28T08:00:00Z")
	// Measured after the dose below, so it is not the weight it uses.
	s.weight(s.m2, 30_000, "2026-03-01T07:00:00Z")

	induced, err := service.RecordAdministration(ctx, technician, s.exp, s.a2, application.AdministrationInput{
		RegimenItemID: s.induction, AdministeredAt: at("2026-03-01T06:00:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	abs, ok := induced.AbsoluteDose()
	if induced.WeightID != latest || induced.SubstanceName != "MPTP" || induced.Purpose != domain.Induction || !ok || abs.Amount() != "0.5" || abs.Unit != domain.Milligram {
		t.Fatalf("induction: %+v %+v", induced, abs)
	}
	if _, err := service.RecordAdministration(ctx, technician, s.exp, s.a1, application.AdministrationInput{
		RegimenItemID: s.induction, AdministeredAt: at("2026-03-01T06:00:00Z"),
	}); !errors.Is(err, application.ErrRegimenItemOtherGroup) {
		t.Fatalf("induction of a healthy animal: %v", err)
	}
	if _, err := service.RecordAdministration(ctx, technician, s.exp, s.a1, application.AdministrationInput{
		RegimenItemID: s.healthyVehicle, AdministeredAt: at("2026-03-02T06:00:00Z"),
	}); !errors.Is(err, domain.ErrWeightRequired) {
		t.Fatalf("per-kg dose for an animal never weighed: %v", err)
	}
	if _, err := service.RecordAdministration(ctx, technician, s.exp, s.a1, application.AdministrationInput{
		RegimenItemID: s.healthyPeriod, AdministeredAt: at("2026-03-02T06:00:00Z"),
	}); !errors.Is(err, application.ErrNoSubstanceItem) {
		t.Fatalf("dose of a no-substance period: %v", err)
	}
	if _, err := service.RecordAdministration(ctx, technician, s.exp, s.a3, application.AdministrationInput{
		Purpose: "VEHICLE", SubstanceID: s.saline, Amount: "0.2", Unit: "mL", Route: "ORAL", AdministeredAt: at("2026-03-02T13:00:00Z"),
	}); !errors.Is(err, domain.ErrAfterExclusion) {
		t.Fatalf("dose after death: %v", err)
	}
	if _, err := service.RecordAdministration(ctx, technician, s.draft, s.draftAllocation, application.AdministrationInput{
		Purpose: "VEHICLE", SubstanceID: s.saline, Amount: "0.2", Unit: "mL", Route: "ORAL", AdministeredAt: at("2026-03-02T13:00:00Z"),
	}); !errors.Is(err, application.ErrExperimentNotRunning) {
		t.Fatalf("dose in a draft: %v", err)
	}
	extra, err := service.RecordAdministration(ctx, technician, s.exp, s.a2, application.AdministrationInput{
		Purpose: "TREATMENT", SubstanceID: s.saline, Amount: "0.2", Unit: "mL", Route: "ORAL", Notes: "rescue", AdministeredAt: at("2026-03-03T06:00:00Z"),
	})
	if err != nil || extra.RegimenItemID != "" || extra.WeightID != "" {
		t.Fatalf("unplanned: %+v %v", extra, err)
	}

	page, err := service.ExperimentAdministrations(ctx, viewer, s.exp, application.AdministrationQuery{})
	if err != nil || page.Total != 2 || page.Administrations[0].ID != extra.ID || page.Administrations[1].BodyMilligrams != 25_000 || page.Administrations[1].SubstanceName != "MPTP" {
		t.Fatalf("experiment list: %+v %v", page, err)
	}
	page, err = service.ExperimentAdministrations(ctx, viewer, s.exp, application.AdministrationQuery{Purpose: "INDUCTION"})
	if err != nil || page.Total != 1 || page.Administrations[0].ID != induced.ID {
		t.Fatalf("purpose filter: %+v %v", page, err)
	}
	page, err = service.ExperimentAdministrations(ctx, viewer, s.exp, application.AdministrationQuery{AllocationID: s.a1})
	if err != nil || page.Total != 0 {
		t.Fatalf("allocation filter: %+v %v", page, err)
	}
	page, err = service.SubjectAdministrations(ctx, viewer, s.m2, application.AdministrationQuery{From: at("2026-03-02T00:00:00Z")})
	if err != nil || page.Total != 1 || page.Administrations[0].Notes != "rescue" {
		t.Fatalf("subject list: %+v %v", page, err)
	}

	if _, err := service.RecordInductionOutcome(ctx, technician, s.exp, s.a1, application.OutcomeInput{Status: "CONFIRMED", AssessedAt: at("2026-03-05T08:00:00Z")}); !errors.Is(err, application.ErrNoDisease) {
		t.Fatalf("outcome of a healthy control: %v", err)
	}
	if _, err := service.RecordInductionOutcome(ctx, technician, s.exp, s.a2, application.OutcomeInput{Status: "NOT_CONFIRMED", AssessedAt: at("2026-03-05T08:00:00Z")}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordInductionOutcome(ctx, technician, s.exp, s.a2, application.OutcomeInput{Status: "CONFIRMED", AssessedAt: at("2026-03-06T08:00:00Z")}); !errors.Is(err, application.ErrOutcomeExists) {
		t.Fatalf("second outcome: %v", err)
	}
	rows, err := service.InductionOutcomes(ctx, viewer, s.exp, "")
	if err != nil || len(rows) != 1 || rows[0].SubjectCode != "M2" || rows[0].GroupCode != "G2" || rows[0].Status != domain.NotConfirmed {
		t.Fatalf("outcomes: %+v %v", rows, err)
	}
	if rows, err := service.InductionOutcomes(ctx, viewer, s.exp, "CONFIRMED"); err != nil || len(rows) != 0 {
		t.Fatalf("status filter: %+v %v", rows, err)
	}
}

func TestDueDosesFollowTheLaboratoryDay(t *testing.T) {
	s := newStudy(t)
	store := postgres.NewStore(s.pool)
	service := application.New(store, clock)
	s.weight(s.m1, 25_000, "2026-02-28T08:00:00Z")
	// 23:30 on 2 March in the laboratory: the latest weight for that day.
	late := s.weight(s.m1, 26_000, "2026-03-02T20:30:00Z")
	// 01:00 on 2 March in the laboratory, still 1 March in UTC.
	s.exec(`INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, regimen_item_id, purpose, substance_id,
		dose_micro, unit, route, weight_measurement_id, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, $5, 'VEHICLE', $6, 10000000, 'mL/kg', 'INTRAPERITONEAL',
		(SELECT id FROM misko.weight_measurements WHERE subject_id = $4 ORDER BY measured_at LIMIT 1), '2026-03-01T22:00:00Z', $7)`,
		s.exp, s.a1, s.healthy, s.m1, s.healthyVehicle, s.saline, user)

	type row struct {
		code    string
		items   []string
		given   []int
		exclude bool
	}
	summary := func(day string) (int, []row) {
		t.Helper()
		schedule, err := service.DueDoses(ctx, viewer, s.exp, day)
		if err != nil {
			t.Fatal(err)
		}
		out := []row{}
		for _, a := range schedule.Animals {
			r := row{code: a.SubjectCode, exclude: a.Excluded}
			for _, d := range a.Doses {
				r.items = append(r.items, d.Item.ID)
				r.given = append(r.given, d.Given)
			}
			out = append(out, r)
		}
		return schedule.StudyDay, out
	}

	day0, rows := summary("2026-03-01")
	if day0 != 0 || len(rows) != 2 || rows[0].code != "M2" || rows[1].code != "M3" ||
		!slices.Equal(rows[0].items, []string{s.induction}) || rows[0].given[0] != 0 {
		t.Fatalf("day 0: %+v", rows)
	}
	day1, rows := summary("2026-03-02")
	if day1 != 1 || len(rows) != 3 || rows[0].code != "M1" || rows[1].code != "M2" || rows[2].code != "M3" ||
		!slices.Equal(rows[0].given, []int{1}) || !slices.Equal(rows[1].items, []string{s.sickVehicle}) || !rows[2].exclude || rows[1].exclude {
		t.Fatalf("day 1: %+v", rows)
	}
	schedule, _ := service.DueDoses(ctx, viewer, s.exp, "2026-03-02")
	m1 := schedule.Animals[0]
	if m1.Weight == nil || m1.Weight.ID != late || !m1.Doses[0].Complete() || m1.Doses[0].AbsoluteDose.Amount() != "0.26" || m1.Doses[0].AbsoluteDose.Unit != domain.Milliliter {
		t.Fatalf("M1 on day 1: %+v %+v", m1, m1.Doses[0].AbsoluteDose)
	}
	if m2 := schedule.Animals[1]; m2.Weight != nil || m2.Doses[0].AbsoluteDose != nil || m2.Doses[0].Complete() {
		t.Fatalf("M2 has no weight: %+v", m2)
	}
	_, rows = summary("2026-03-03")
	if len(rows) != 3 || rows[0].code != "M1" || rows[1].code != "M4" || rows[2].code != "M2" || !slices.Equal(rows[0].given, []int{0}) {
		t.Fatalf("day 2, M3 gone and M4 allocated: %+v", rows)
	}
	if day, rows := summary("2026-03-07"); day != 6 || len(rows) != 0 {
		t.Fatalf("after the regimen: %d %+v", day, rows)
	}
	if _, err := service.DueDoses(ctx, viewer, s.draft, "2026-03-02"); !errors.Is(err, application.ErrExperimentNotStarted) {
		t.Fatalf("draft: %v", err)
	}
	if _, err := service.DueDoses(ctx, viewer, "01a00000-0000-7000-8000-0000000000ff", ""); !errors.Is(err, application.ErrExperimentNotFound) {
		t.Fatalf("unknown experiment: %v", err)
	}
	today, err := service.DueDoses(ctx, viewer, s.exp, "")
	if err != nil {
		t.Fatal(err)
	}
	var local string
	if err := s.pool.QueryRow(ctx, `SELECT to_char((now() AT TIME ZONE 'Europe/Istanbul')::date, 'YYYY-MM-DD')`).Scan(&local); err != nil || today.Date.Format(time.DateOnly) != local {
		t.Fatalf("today: %s want %s (%v)", today.Date, local, err)
	}
}
