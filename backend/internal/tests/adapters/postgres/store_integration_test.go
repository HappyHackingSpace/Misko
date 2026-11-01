//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"testing"
	"time"
)

const user = "00000000-0000-7000-8000-000000000001"

// The store maps the schema's rules to the use case errors, so a request that
// slips past the use case checks still answers with a code instead of a 500.
func TestStoreTranslatesSchemaRules(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	id := func(sql string, args ...any) string {
		t.Helper()
		var out string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return out
	}
	exp := id(`INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id`)
	group := id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, exp)
	day7 := id(`INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 7', 7, 1) RETURNING id`, exp)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var protocol, version string
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id`, exp).Scan(&protocol); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
		VALUES ($1, $2, 1, 1, $3) RETURNING id`, exp, protocol, user).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
		VALUES ($1, 1, 'MWM', 1, 'ACQUISITION', 1, '{}')`, version); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	allocate := func(code string) string {
		subject := id(`INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id`, code)
		return id(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
			VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id`, exp, group, subject, user)
	}
	first, second := allocate("M1"), allocate("M2")
	revision := func(name, paradigm string) string {
		env := id(`INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, $2) RETURNING id`, name, paradigm)
		return id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
			VALUES ($1, $2, 1, 1, '{}', $3) RETURNING id`, env, paradigm, user)
	}
	box, maze := revision("Box", "MWM"), revision("Maze", "POLE")

	store := postgres.NewStore(pool)
	planned, err := domain.NewTest(domain.Context{ExperimentID: exp, TimepointID: day7, ProtocolVersionID: version, StepPosition: 1,
		ParadigmKey: "MWM", ParadigmVersion: 1, PlannedTrials: 1}, time.Now(), "", user, []domain.Participant{{AllocationID: first, EnvironmentRevisionID: box}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTest(ctx, planned); !errors.Is(err, application.ErrExperimentNotRunning) {
		t.Fatalf("test in a draft: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1`, exp); err != nil {
		t.Fatal(err)
	}
	wrongStep := planned
	wrongStep.PlannedTrials = 5
	if _, err := store.CreateTest(ctx, wrongStep); !errors.Is(err, application.ErrStepNotFound) {
		t.Fatalf("step with other trials: %v", err)
	}
	created, err := store.CreateTest(ctx, planned)
	if err != nil || len(created.Subjects) != 1 || created.Subjects[0].SubjectCode != "M1" || created.TimepointName != "Day 7" || created.ProtocolVersion != 1 {
		t.Fatalf("created: %+v %v", created, err)
	}
	add := func(allocation, arena string) error {
		return store.AddTestSubject(ctx, exp, domain.TestSubject{TestID: created.ID, AllocationID: allocation, EnvironmentRevisionID: arena, CreatedBy: user}, "MWM", 1)
	}
	for name, tc := range map[string]struct {
		allocation, arena string
		want              error
	}{
		"arena of another paradigm": {second, maze, application.ErrArenaParadigmMismatch},
		"arena taken":               {second, box, domain.ErrDuplicateArena},
		"animal twice":              {first, revision("Box 2", "MWM"), domain.ErrDuplicateAnimal},
		"unknown allocation":        {created.ID, revision("Box 3", "MWM"), application.ErrAllocationNotFound},
	} {
		if err := add(tc.allocation, tc.arena); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	started, _ := created.Start(time.Now().UTC().Truncate(time.Microsecond), time.Now())
	if _, err := store.UpdateTestStatus(ctx, started, domain.Planned); err != nil {
		t.Fatal(err)
	}
	if err := add(second, revision("Box 4", "MWM")); !errors.Is(err, domain.ErrNotPlanned) {
		t.Errorf("animal joins a running test: %v", err)
	}
	if err := store.RemoveTestSubject(ctx, created.ID, created.Subjects[0].ID); !errors.Is(err, domain.ErrNotPlanned) {
		t.Errorf("animal leaves a running test: %v", err)
	}
	if _, err := store.UpdateTestStatus(ctx, started, domain.Planned); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("stale status: %v", err)
	}
	trial := domain.Trial{TestID: created.ID, TestSubjectID: created.Subjects[0].ID, Number: 1, Repetition: 2, Attempt: 1, StartedAt: time.Now(), RecordedBy: user}
	if _, err := store.CreateTrial(ctx, trial); !errors.Is(err, domain.ErrTestNotInProgress) {
		t.Errorf("unplanned repetition: %v", err)
	}
	// Search matches the study code, the timepoint name or a subject code, and
	// a wildcard is a literal character.
	for search, want := range map[string]int{"": 1, "e1": 1, "day 7": 1, "m1": 1, "M2": 0, "%": 0} {
		if tests, total, err := store.ListTests(ctx, application.TestFilter{Search: search, Limit: 10}); err != nil || len(tests) != want || total != want {
			t.Errorf("search %q: %d of %d, %v", search, len(tests), total, err)
		}
	}
}
