//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/adapters/catalog"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ctx        = context.Background()
	researcher = access.Actor{UserID: "00000000-0000-7000-8000-000000000001", Role: access.Researcher}
)

type fixture struct {
	t     *testing.T
	pool  *pgxpool.Pool
	store *postgres.Store
	s     *application.Service
	codes int
}

func newFixture(t *testing.T) *fixture {
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	return &fixture{t: t, pool: pool, store: store, s: application.New(store, catalog.New(), time.Now, rand.IntN)}
}

// id runs an INSERT ... RETURNING id. Rows of other contexts (subjects,
// catalog, protocols, tests) are written directly so this adapter stays
// independent of their packages.
func (f *fixture) id(sql string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f *fixture) subject() string {
	f.codes++
	return f.id(`INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'FEMALE') RETURNING id`, fmt.Sprintf("S%d", f.codes))
}

// must panics on an error, which fails the running test with a stack trace.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// study builds a design that meets every start rule.
type study struct {
	experiment                domain.Experiment
	healthy, disease, treated domain.Group
	timepoint                 domain.Timepoint
	treatment                 domain.RegimenItem
}

func (f *fixture) study(code string) study {
	t := f.t
	t.Helper()
	s := study{experiment: must(f.s.CreateExperiment(ctx, researcher, domain.ExperimentFields{Code: code, Title: "Study " + code}))}
	e := s.experiment.ID
	model := f.id(`INSERT INTO misko.disease_models (name, induction_type) VALUES ($1, 'CHEMICAL') RETURNING id`, "Model "+code)
	inducer := f.id(`INSERT INTO misko.substances (name, kind) VALUES ($1, 'INDUCER') RETURNING id`, "Inducer "+code)
	vehicle := f.id(`INSERT INTO misko.substances (name, kind) VALUES ($1, 'VEHICLE') RETURNING id`, "Vehicle "+code)
	compound := f.id(`INSERT INTO misko.substances (name, kind) VALUES ($1, 'COMPOUND') RETURNING id`, "Compound "+code)
	s.healthy = must(f.s.CreateGroup(ctx, researcher, e, domain.GroupFields{Code: "G1", Name: "Healthy", Role: "HEALTHY_CONTROL", TargetSize: 2}))
	s.disease = must(f.s.CreateGroup(ctx, researcher, e, domain.GroupFields{Code: "G2", Name: "Disease", Role: "DISEASE_CONTROL", DiseaseModelID: model, TargetSize: 2}))
	s.treated = must(f.s.CreateGroup(ctx, researcher, e, domain.GroupFields{Code: "G3", Name: "Treated", Role: "TREATMENT", DiseaseModelID: model, TargetSize: 2}))
	item := func(group domain.Group, purpose, substance string) domain.RegimenItem {
		return must(f.s.CreateRegimenItem(ctx, researcher, e, domain.RegimenFields{GroupID: group.ID, Purpose: purpose, SubstanceID: substance,
			Amount: "10", Unit: "mg/kg", Route: "INTRAPERITONEAL", Frequency: "DAILY", StartDay: 0, EndDay: 7}))
	}
	item(s.disease, "INDUCTION", inducer)
	item(s.disease, "VEHICLE", vehicle)
	item(s.treated, "INDUCTION", inducer)
	s.treatment = item(s.treated, "TREATMENT", compound)
	s.timepoint = must(f.s.CreateTimepoint(ctx, researcher, e, domain.TimepointFields{Name: "Day 7", StudyDay: 7, Position: 1}))
	f.protocol(e)
	must(f.s.CreateEndpoint(ctx, researcher, e, domain.EndpointFields{TimepointID: s.timepoint.ID, ParadigmKey: "OPEN_FIELD",
		MetricKey: "distance_cm", Kind: "PRIMARY", Direction: "HIGHER_IS_BETTER"}))
	return s
}

// protocol stores a one-step OPEN_FIELD protocol and returns its version id.
func (f *fixture) protocol(experimentID string) string {
	f.t.Helper()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var protocol, version string
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id`, experimentID).Scan(&protocol); err != nil {
		f.t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
		VALUES ($1, $2, 1, 1, $3) RETURNING id`, experimentID, protocol, researcher.UserID).Scan(&version); err != nil {
		f.t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
		VALUES ($1, 1, 'OPEN_FIELD', 1, 'STANDARD', 1, '{}')`, version); err != nil {
		f.t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		f.t.Fatal(err)
	}
	return version
}

// The store translates every trigger and constraint the use cases rely on,
// even when the application check is bypassed.
func TestDatabaseRulesBecomeApplicationErrors(t *testing.T) {
	f := newFixture(t)
	s := f.study("E1")
	e := s.experiment.ID
	animal := f.subject()
	allocation := must(f.s.Allocate(ctx, researcher, e, application.AllocateInput{SubjectID: animal, GroupID: s.healthy.ID}))

	expect := func(label string, err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Errorf("%s: err=%v want %v", label, err, want)
		}
	}
	_, err := f.store.CreateGroup(ctx, domain.Group{ExperimentID: e, Code: "G1", Name: "Other", Role: domain.HealthyControl})
	expect("group code", err, application.ErrGroupCodeTaken)
	_, err = f.store.CreateGroup(ctx, domain.Group{ExperimentID: e, Code: "g9", Name: "healthy", Role: domain.HealthyControl})
	expect("group name", err, application.ErrGroupNameTaken)
	_, err = f.store.CreateGroup(ctx, domain.Group{ExperimentID: e, Code: "G9", Name: "Sham", Role: domain.Sham, DiseaseModelID: "00000000-0000-7000-8000-00000000abcd"})
	expect("disease model", err, application.ErrDiseaseModelNotFound)
	other := f.study("E2")
	_, err = f.store.CreateAllocation(ctx, domain.Allocation{ExperimentID: e, GroupID: other.healthy.ID, SubjectID: f.subject(), AllocatedAt: time.Now(), CreatedBy: researcher.UserID})
	expect("foreign group", err, application.ErrGroupNotInExperiment)
	_, err = f.store.CreateAllocation(ctx, domain.Allocation{ExperimentID: e, GroupID: s.healthy.ID, SubjectID: animal, AllocatedAt: time.Now(), CreatedBy: researcher.UserID})
	expect("second allocation", err, application.ErrAlreadyAllocated)
	_, err = f.store.CreateAllocation(ctx, domain.Allocation{ExperimentID: e, GroupID: s.healthy.ID, SubjectID: "00000000-0000-7000-8000-00000000abcd", AllocatedAt: time.Now(), CreatedBy: researcher.UserID})
	expect("unknown subject", err, application.ErrSubjectNotFound)
	_, err = f.store.CreateTimepoint(ctx, domain.Timepoint{ExperimentID: e, Name: "day 7", StudyDay: 8, Position: 2})
	expect("timepoint name", err, application.ErrTimepointConflict)
	expect("timepoint in use", f.store.DeleteTimepoint(ctx, e, s.timepoint.ID), application.ErrTimepointInUse)
	expect("group in use", f.s.DeleteGroup(ctx, researcher, e, s.healthy.ID), application.ErrGroupInUse)
	_, err = f.store.CreateRegimenItem(ctx, domain.RegimenItem{ExperimentID: e, GroupID: s.treated.ID, Purpose: domain.TreatmentPurpose, SubstanceID: s.treatment.SubstanceID,
		DoseMicro: 1, Unit: "mg", Route: "ORAL", Frequency: domain.Daily, StartDay: 1, EndDay: 2})
	expect("second treatment", err, application.ErrTreatmentItemTaken)
	_, err = f.store.CreateEndpoint(ctx, domain.Endpoint{ExperimentID: e, TimepointID: s.timepoint.ID, ParadigmKey: "OPEN_FIELD", MetricKey: "center_time_s",
		Kind: domain.Primary, Direction: domain.HigherIsBetter})
	expect("second primary", err, application.ErrPrimaryEndpointTaken)
	_, err = f.store.CreateEndpoint(ctx, domain.Endpoint{ExperimentID: e, TimepointID: other.timepoint.ID, ParadigmKey: "OPEN_FIELD", MetricKey: "center_time_s",
		Kind: domain.Secondary, Direction: domain.HigherIsBetter})
	expect("foreign timepoint", err, application.ErrTimepointNotInExperiment)

	must(f.s.Start(ctx, researcher, e, "2026-10-15"))
	_, err = f.store.CreateGroup(ctx, domain.Group{ExperimentID: e, Code: "G9", Name: "Late", Role: domain.HealthyControl})
	expect("group after start", err, application.ErrDesignFrozen)
	item := s.treatment
	item.DoseMicro = 1
	_, err = f.store.UpdateRegimenItem(ctx, item)
	expect("regimen after start", err, application.ErrDesignFrozen)
	expect("timepoint after start", f.store.DeleteTimepoint(ctx, e, s.timepoint.ID), application.ErrDesignFrozen)
	moved := allocation
	moved.GroupID = s.disease.ID
	_, err = f.store.UpdateAllocation(ctx, moved)
	expect("move after start", err, application.ErrAllocationFrozen)
	expect("remove after start", f.store.DeleteAllocation(ctx, e, allocation.ID), application.ErrAllocationFrozen)
	excluded := must(f.s.Exclude(ctx, researcher, e, allocation.ID, application.ExcludeInput{Reason: "DIED"}))
	excluded.ExcludedAt, excluded.ExclusionReason = nil, ""
	_, err = f.store.UpdateAllocation(ctx, excluded)
	expect("undo exclusion", err, application.ErrAllocationFrozen)

	must(f.s.Complete(ctx, researcher, e))
	running := s.experiment
	running.Title = "Renamed"
	_, err = f.store.UpdateExperiment(ctx, running)
	expect("rename completed", err, application.ErrInvalidTransition)
	_, err = f.store.CreateAllocation(ctx, domain.Allocation{ExperimentID: e, GroupID: s.healthy.ID, SubjectID: f.subject(), AllocatedAt: time.Now(), CreatedBy: researcher.UserID})
	expect("allocate after completion", err, application.ErrAllocationFrozen)
}

// Concurrent randomisations of one experiment never fill a group beyond its
// target: the second one sees the first one's animals.
func TestConcurrentRandomisationRespectsTargets(t *testing.T) {
	f := newFixture(t)
	s := f.study("E1")
	batches := [][]string{{f.subject(), f.subject(), f.subject(), f.subject()}, {f.subject(), f.subject(), f.subject(), f.subject()}}
	var wg sync.WaitGroup
	errs := make([]error, len(batches))
	for i, batch := range batches {
		wg.Go(func() {
			_, errs[i] = f.s.Randomise(ctx, researcher, s.experiment.ID, application.RandomiseInput{SubjectIDs: batch})
		})
	}
	wg.Wait()
	failed := 0
	for _, err := range errs {
		if errors.Is(err, application.ErrNotEnoughPlaces) {
			failed++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	fills := must(f.s.Groups(ctx, researcher, s.experiment.ID))
	total := 0
	for _, g := range fills {
		if g.Allocated > g.Group.TargetSize {
			t.Errorf("group %s over target: %d", g.Group.Code, g.Allocated)
		}
		total += g.Allocated
	}
	if failed != 1 || total != 4 {
		t.Fatalf("failed=%d total=%d", failed, total)
	}
}

func TestCompletionWaitsForOpenTests(t *testing.T) {
	f := newFixture(t)
	s := f.study("E1")
	e := s.experiment.ID
	version := f.id(`SELECT v.id FROM misko.protocol_versions v WHERE v.experiment_id = $1`, e)
	must(f.s.Start(ctx, researcher, e, "2026-10-15"))
	test := f.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
		planned_trials, scheduled_at, created_by) VALUES ($1, $2, $3, 1, 'OPEN_FIELD', 1, 1, now(), $4) RETURNING id`, e, s.timepoint.ID, version, researcher.UserID)
	if _, err := f.s.Complete(ctx, researcher, e); !errors.Is(err, application.ErrOpenTests) {
		t.Fatalf("complete with a planned test: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE misko.tests SET status = 'CANCELLED', cancelled_at = now(), cancel_reason = 'x' WHERE id = $1`, test); err != nil {
		t.Fatal(err)
	}
	done := must(f.s.Complete(ctx, researcher, e))
	if done.Status != domain.Completed || done.CompletedAt == nil || done.CompletedAt.Before(*done.StartedAt) {
		t.Fatalf("completed: %+v", done)
	}
}
