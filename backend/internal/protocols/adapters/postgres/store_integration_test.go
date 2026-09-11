//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/adapters/catalog"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/application"
	"github.com/jackc/pgx/v5/pgxpool"
	"reflect"
	"slices"
	"sync"
	"testing"
)

var (
	ctx     = context.Background()
	manager = access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.LabManager}
	unknown = "01a00000-0000-7000-8000-00000000dead"
)

// fixture creates experiments and environment revisions with SQL; adapters of
// other domains are not imported.
type fixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	s    *application.Service
	n    int
}

func newFixture(t *testing.T) *fixture {
	pool := pgtest.Database(t)
	return &fixture{t: t, pool: pool, s: application.New(postgres.NewStore(pool), catalog.New())}
}

func (f *fixture) id(sql string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) experiment() string {
	f.n++
	return f.id("INSERT INTO misko.experiments (code, title) VALUES ($1, 'Study') RETURNING id", fmt.Sprintf("E%d", f.n))
}

func (f *fixture) environment(paradigm string) string {
	f.n++
	return f.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, $2) RETURNING id", fmt.Sprintf("Env %d", f.n), paradigm)
}

func (f *fixture) revision(environmentID, paradigm string, number int, apparatus string) string {
	return f.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, $2, $3, 1, $4::jsonb, $5) RETURNING id`, environmentID, paradigm, number, apparatus, manager.UserID)
}

const arenaApparatus = `{"arena_width_cm": 50, "arena_height_cm": 50, "center_fraction": 0.5}`

func openField(position int, revisionID string) application.StepInput {
	return application.StepInput{Position: position, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, EnvironmentRevisionID: revisionID, TrialType: "STANDARD", Trials: 1}
}

func elevatedPlusMaze(position int, revisionID string) application.StepInput {
	return application.StepInput{Position: position, ParadigmKey: "EPM", ParadigmVersion: 1, EnvironmentRevisionID: revisionID, TrialType: "STANDARD", Trials: 1}
}

func protocol(name string, steps ...application.StepInput) application.ProtocolInput {
	return application.ProtocolInput{Name: name, Version: application.VersionInput{Steps: steps}}
}

func TestVersionsPinTheirEnvironmentRevisions(t *testing.T) {
	f := newFixture(t)
	exp := f.experiment()
	arena, maze := f.environment("OPEN_FIELD"), f.environment("EPM")
	arena1 := f.revision(arena, "OPEN_FIELD", 1, arenaApparatus)
	maze1 := f.revision(maze, "EPM", 1, `{"arm_length_cm": 35, "arm_width_cm": 6}`)

	of := openField(2, arena1)
	of.Trials, of.InterTrialIntervalS = 2, 300
	of.Session = map[string]float64{"max_sample_gap_s": 0.25}
	in := protocol("Anxiety battery", of, elevatedPlusMaze(1, maze1))
	in.Description, in.Version.Notes = "Two paradigms", "baseline"
	created, err := f.s.Create(ctx, manager, exp, in)
	if err != nil {
		t.Fatal(err)
	}
	p, v1 := created.Protocol, created.Version
	if p.LatestVersion != 1 || v1.Number != 1 || v1.Notes != "baseline" || v1.CreatedBy != manager.UserID || len(v1.Steps) != 2 {
		t.Fatalf("created: %+v", created)
	}
	first, last := v1.Steps[0], v1.Steps[1]
	if first.ParadigmKey != "EPM" || first.EnvironmentID != maze || first.EnvironmentRevision != 1 ||
		last.ParadigmKey != "OPEN_FIELD" || last.EnvironmentRevisionID != arena1 || last.Trials != 2 || last.InterTrialIntervalS != 300 {
		t.Fatalf("steps: %+v", v1.Steps)
	}
	if last.Session["max_sample_gap_s"] != 0.25 || last.Session["immobility_threshold_cm_s"] != 2 || len(last.Session) != 3 {
		t.Fatalf("session with defaults: %v", last.Session)
	}

	// A new environment revision leaves version 1 on revision 1.
	arena2 := f.revision(arena, "OPEN_FIELD", 2, `{"arena_width_cm": 60, "arena_height_cm": 60, "center_fraction": 0.5}`)
	v2, err := f.s.AddVersion(ctx, manager, exp, p.ID, application.VersionInput{Steps: []application.StepInput{openField(1, arena2)}})
	if err != nil || v2.Number != 2 || v2.Steps[0].EnvironmentRevision != 2 {
		t.Fatalf("version 2: %+v %v", v2, err)
	}
	again, err := f.s.Version(ctx, manager, exp, p.ID, 1)
	if err != nil || again.ID != v1.ID || !reflect.DeepEqual(again.Steps, v1.Steps) {
		t.Fatalf("version 1 changed: %+v %v", again, err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var numbers []int
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := f.s.AddVersion(ctx, manager, exp, p.ID, application.VersionInput{Steps: []application.StepInput{openField(1, arena2)}})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("concurrent version: %v", err)
			}
			numbers = append(numbers, v.Number)
		}()
	}
	wg.Wait()
	slices.Sort(numbers)
	if !slices.Equal(numbers, []int{3, 4, 5, 6, 7, 8}) {
		t.Fatalf("concurrent numbers: %v", numbers)
	}
	versions, err := f.s.Versions(ctx, manager, exp, p.ID)
	if err != nil || len(versions) != 8 || versions[0].Number != 1 || len(versions[0].Steps) != 2 {
		t.Fatalf("versions: %d %v", len(versions), err)
	}
	if got, err := f.s.Get(ctx, manager, exp, p.ID); err != nil || got.LatestVersion != 8 || got.Description != "Two paradigms" {
		t.Fatalf("protocol: %+v %v", got, err)
	}

	// Referenced versions, their steps and revisions cannot change, even with direct SQL.
	for _, sql := range []string{
		"UPDATE misko.protocol_versions SET notes = 'edited' WHERE protocol_id = $1",
		"DELETE FROM misko.protocol_versions WHERE protocol_id = $1",
		"UPDATE misko.protocol_steps SET trials = 9 WHERE protocol_version_id IN (SELECT id FROM misko.protocol_versions WHERE protocol_id = $1)",
		"DELETE FROM misko.protocol_steps WHERE protocol_version_id IN (SELECT id FROM misko.protocol_versions WHERE protocol_id = $1)",
		"INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session) " +
			"SELECT id, 3, 'OPEN_FIELD', 1, '" + arena1 + "', 'STANDARD', 1, 0, '{}' FROM misko.protocol_versions WHERE protocol_id = $1 AND number = 1",
		"DELETE FROM misko.protocols WHERE id = $1",
	} {
		if _, err := f.pool.Exec(ctx, sql, p.ID); err == nil {
			t.Errorf("%s succeeded", sql)
		}
	}
	if _, err := f.pool.Exec(ctx, "DELETE FROM misko.environment_revisions WHERE id = $1", arena1); err == nil {
		t.Error("a referenced environment revision was deleted")
	}
	_, err = f.pool.Exec(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 1, $3)`, exp, p.ID, manager.UserID)
	if _, constraint := pgtx.Violation(err); constraint != "protocol_versions_number_key" {
		t.Errorf("duplicate version number: %v", err)
	}
	if again, err := f.s.Version(ctx, manager, exp, p.ID, 1); err != nil || !reflect.DeepEqual(again.Steps, v1.Steps) {
		t.Fatalf("version 1 changed after direct SQL: %+v %v", again, err)
	}
}

func TestProtocolsAreValidatedAndScopedToTheirExperiment(t *testing.T) {
	f := newFixture(t)
	exp, other := f.experiment(), f.experiment()
	arena1 := f.revision(f.environment("OPEN_FIELD"), "OPEN_FIELD", 1, arenaApparatus)
	maze1 := f.revision(f.environment("EPM"), "EPM", 1, `{"arm_length_cm": 35, "arm_width_cm": 6}`)
	created, err := f.s.Create(ctx, manager, exp, protocol("Battery", openField(1, arena1)))
	if err != nil {
		t.Fatal(err)
	}
	id := created.Protocol.ID

	step := func(mutate func(*application.StepInput)) application.StepInput {
		s := openField(1, arena1)
		mutate(&s)
		return s
	}
	for name, tc := range map[string]struct {
		experiment string
		in         application.ProtocolInput
		want       error
	}{
		"name taken in experiment":        {exp, protocol("BATTERY", openField(1, arena1)), application.ErrNameTaken},
		"environment of another paradigm": {exp, protocol("P1", elevatedPlusMaze(1, arena1)), application.ErrIncompatibleEnvironment},
		"unknown environment revision":    {exp, protocol("P2", openField(1, unknown)), application.ErrEnvironmentRevisionNotFound},
		"malformed environment revision":  {exp, protocol("P3", openField(1, "arena")), application.ErrEnvironmentRevisionNotFound},
		"trial type of another paradigm":  {exp, protocol("P4", step(func(s *application.StepInput) { s.TrialType = "PROBE" })), application.ErrTrialTypeNotSupported},
		"unknown paradigm version":        {exp, protocol("P5", step(func(s *application.StepInput) { s.ParadigmVersion = 2 })), application.ErrUnknownParadigmVersion},
		"session outside range": {exp, protocol("P6", step(func(s *application.StepInput) { s.Session = map[string]float64{"max_sample_gap_s": 9} })),
			application.ErrInvalidSession},
		"apparatus value as session": {exp, protocol("P7", step(func(s *application.StepInput) { s.Session = map[string]float64{"arena_width_cm": 60} })),
			application.ErrInvalidSession},
		"unknown experiment":   {unknown, protocol("P8", openField(1, arena1)), application.ErrExperimentNotFound},
		"malformed experiment": {"study", protocol("P9", openField(1, arena1)), application.ErrExperimentNotFound},
	} {
		if _, err := f.s.Create(ctx, manager, tc.experiment, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	if list, err := f.s.List(ctx, manager, exp); err != nil || len(list) != 1 {
		t.Fatalf("a failed create left a protocol: %+v %v", list, err)
	}
	if _, err := f.s.Create(ctx, manager, other, protocol("Battery", elevatedPlusMaze(1, maze1))); err != nil {
		t.Fatalf("the same name in another experiment: %v", err)
	}

	renamed, empty := "Battery 2", ""
	for name, tc := range map[string]struct {
		err, want error
	}{
		"get from another experiment":      {second(f.s.Get(ctx, manager, other, id)), application.ErrProtocolNotFound},
		"update from another experiment":   {second(f.s.Update(ctx, manager, other, id, application.Patch{Name: &renamed})), application.ErrProtocolNotFound},
		"version from another experiment":  {second(f.s.AddVersion(ctx, manager, other, id, application.VersionInput{Steps: []application.StepInput{openField(1, arena1)}})), application.ErrProtocolNotFound},
		"versions from another experiment": {second(f.s.Versions(ctx, manager, other, id)), application.ErrProtocolNotFound},
		"read version elsewhere":           {second(f.s.Version(ctx, manager, other, id, 1)), application.ErrVersionNotFound},
		"missing version":                  {second(f.s.Version(ctx, manager, exp, id, 2)), application.ErrVersionNotFound},
		"malformed protocol":               {second(f.s.Get(ctx, manager, exp, "battery")), application.ErrProtocolNotFound},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, tc.err, tc.want)
		}
	}
	updated, err := f.s.Update(ctx, manager, exp, id, application.Patch{Name: &renamed, Description: &empty})
	if err != nil || updated.Name != "Battery 2" || updated.Description != "" || updated.LatestVersion != 1 {
		t.Fatalf("update: %+v %v", updated, err)
	}

	// The schema itself rejects a step whose paradigm differs from its revision.
	_, err = f.pool.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session)
		VALUES ($1, 2, 'OPEN_FIELD', 1, $2, 'STANDARD', 1, 0, '{}')`, created.Version.ID, maze1)
	if _, constraint := pgtx.Violation(err); constraint != "protocol_steps_environment_fkey" {
		t.Errorf("incompatible step through SQL: %v", err)
	}
}

func second[T any](_ T, err error) error { return err }
