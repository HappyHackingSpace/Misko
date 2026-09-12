//go:build integration

package postgres_test

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/catalog"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"sync"
	"testing"
)

var (
	ctx     = context.Background()
	manager = access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.LabManager}
	unknown = "01a00000-0000-7000-8000-00000000dead"
)

func newService(t *testing.T) (*application.Service, *pgxpool.Pool) {
	pool := pgtest.Database(t)
	return application.New(postgres.NewStore(pool), catalog.New()), pool
}

func arena(width float64) application.RevisionInput {
	return application.RevisionInput{ParadigmVersion: 1, Apparatus: map[string]float64{"arena_width_cm": width, "arena_height_cm": 40, "center_fraction": 0.5}}
}

func TestRevisionsAreNumberedAndNeverChange(t *testing.T) {
	s, pool := newService(t)
	created, err := s.Create(ctx, manager, application.EnvironmentInput{Name: "Arena A", ParadigmKey: "OPEN_FIELD", Notes: "north room", Revision: arena(50)})
	if err != nil {
		t.Fatal(err)
	}
	env, first := created.Environment, created.Revision
	if env.LatestRevision != 1 || first.Number != 1 || first.Apparatus["arena_width_cm"] != 50 || first.CreatedBy != manager.UserID || first.EnvironmentID != env.ID {
		t.Fatalf("created: %+v", created)
	}
	second, err := s.AddRevision(ctx, manager, env.ID, arena(60))
	if err != nil || second.Number != 2 || second.Apparatus["arena_width_cm"] != 60 {
		t.Fatalf("second revision: %+v %v", second, err)
	}
	if got, err := s.Get(ctx, manager, env.ID); err != nil || got.LatestRevision != 2 || got.Notes != "north room" {
		t.Fatalf("environment: %+v %v", got, err)
	}

	// Concurrent revisions are numbered one at a time.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var numbers []int
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.AddRevision(ctx, manager, env.ID, arena(float64(70+i)))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("concurrent revision: %v", err)
			}
			numbers = append(numbers, r.Number)
		}()
	}
	wg.Wait()
	slices.Sort(numbers)
	if !slices.Equal(numbers, []int{3, 4, 5, 6, 7, 8, 9, 10}) {
		t.Fatalf("concurrent numbers: %v", numbers)
	}
	revisions, err := s.Revisions(ctx, manager, env.ID)
	if err != nil || len(revisions) != 10 || revisions[0].Number != 1 || revisions[9].Number != 10 {
		t.Fatalf("revisions: %d %v", len(revisions), err)
	}

	// Revisions cannot be edited or removed, and the environment cannot move to
	// another paradigm, even with direct SQL.
	for _, sql := range []string{
		"UPDATE misko.environment_revisions SET notes = 'edited' WHERE environment_id = $1",
		"UPDATE misko.environment_revisions SET apparatus = '{}' WHERE environment_id = $1",
		"DELETE FROM misko.environment_revisions WHERE environment_id = $1",
		"UPDATE misko.environments SET paradigm_key = 'EPM' WHERE id = $1",
		"DELETE FROM misko.environments WHERE id = $1",
	} {
		if _, err := pool.Exec(ctx, sql, env.ID); err == nil {
			t.Errorf("%s succeeded", sql)
		}
	}
	_, err = pool.Exec(ctx, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, 'OPEN_FIELD', 2, 1, '{}', $2)`, env.ID, manager.UserID)
	if _, constraint := pgtx.Violation(err); constraint != "environment_revisions_number_key" {
		t.Errorf("duplicate revision number: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, 'EPM', 11, 1, '{}', $2)`, env.ID, manager.UserID)
	if _, constraint := pgtx.Violation(err); constraint != "environment_revisions_environment_fkey" {
		t.Errorf("revision of another paradigm: %v", err)
	}
	if again, err := s.Revision(ctx, manager, env.ID, 1); err != nil || again.ID != first.ID || again.Apparatus["arena_width_cm"] != 50 || again.Notes != "" {
		t.Fatalf("revision 1 changed: %+v %v", again, err)
	}
}

func TestEnvironmentsAreValidatedAgainstTheCatalog(t *testing.T) {
	s, _ := newService(t)
	a, err := s.Create(ctx, manager, application.EnvironmentInput{Name: "Arena A", ParadigmKey: "OPEN_FIELD", Revision: arena(50)})
	if err != nil {
		t.Fatal(err)
	}
	tank := map[string]float64{"tank_diameter_cm": 150, "platform_diameter_cm": 10, "platform_x_cm": 70, "platform_y_cm": -30, "wall_annulus_width_cm": 15}
	for name, tc := range map[string]struct {
		in   application.EnvironmentInput
		want error
	}{
		"name taken ignoring case": {application.EnvironmentInput{Name: "arena a", ParadigmKey: "OPEN_FIELD", Revision: arena(50)}, application.ErrNameTaken},
		"geometry out of range":    {application.EnvironmentInput{Name: "Small", ParadigmKey: "OPEN_FIELD", Revision: arena(10)}, application.ErrInvalidApparatus},
		"missing measurement": {application.EnvironmentInput{Name: "Partial", ParadigmKey: "OPEN_FIELD",
			Revision: application.RevisionInput{ParadigmVersion: 1, Apparatus: map[string]float64{"arena_width_cm": 50}}}, application.ErrInvalidApparatus},
		"session value as apparatus": {application.EnvironmentInput{Name: "Session", ParadigmKey: "OPEN_FIELD",
			Revision: application.RevisionInput{ParadigmVersion: 1, Apparatus: map[string]float64{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5, "max_sample_gap_s": 1}}}, application.ErrInvalidApparatus},
		"platform outside the tank": {application.EnvironmentInput{Name: "Tank", ParadigmKey: "MWM",
			Revision: application.RevisionInput{ParadigmVersion: 1, Apparatus: tank}}, application.ErrInvalidApparatus},
		"unknown paradigm": {application.EnvironmentInput{Name: "Maze", ParadigmKey: "RADIAL_MAZE", Revision: arena(50)}, application.ErrUnknownParadigm},
		"unknown version": {application.EnvironmentInput{Name: "Future", ParadigmKey: "OPEN_FIELD",
			Revision: application.RevisionInput{ParadigmVersion: 9, Apparatus: arena(50).Apparatus}}, application.ErrUnknownParadigmVersion},
	} {
		if _, err := s.Create(ctx, manager, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	if _, err := s.AddRevision(ctx, manager, a.Environment.ID, arena(151)); !errors.Is(err, application.ErrInvalidApparatus) {
		t.Errorf("invalid revision: %v", err)
	}
	maze, err := s.Create(ctx, manager, application.EnvironmentInput{Name: "Maze", ParadigmKey: "EPM",
		Revision: application.RevisionInput{ParadigmVersion: 1, Apparatus: map[string]float64{"arm_length_cm": 35, "arm_width_cm": 6}}})
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.List(ctx, manager, "")
	if err != nil || len(all) != 2 || all[0].Name != "Arena A" || all[1].Name != "Maze" {
		t.Fatalf("list keeps no failed environment: %+v %v", all, err)
	}
	if revisions, _ := s.Revisions(ctx, manager, a.Environment.ID); len(revisions) != 1 {
		t.Fatalf("a rejected revision was stored: %+v", revisions)
	}
	if mazes, err := s.List(ctx, manager, "EPM"); err != nil || len(mazes) != 1 || mazes[0].ID != maze.Environment.ID {
		t.Fatalf("filtered list: %+v %v", mazes, err)
	}

	renamed, empty := "Arena B", ""
	updated, err := s.Update(ctx, manager, a.Environment.ID, application.Patch{Name: &renamed, Notes: &empty})
	if err != nil || updated.Name != "Arena B" || updated.Notes != "" || updated.ParadigmKey != "OPEN_FIELD" || updated.LatestRevision != 1 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	taken := "MAZE"
	if _, err := s.Update(ctx, manager, a.Environment.ID, application.Patch{Name: &taken}); !errors.Is(err, application.ErrNameTaken) {
		t.Errorf("rename to a taken name: %v", err)
	}
	for name, err := range map[string]error{
		"get unknown":            second(s.Get(ctx, manager, unknown)),
		"get malformed":          second(s.Get(ctx, manager, "arena")),
		"update unknown":         second(s.Update(ctx, manager, unknown, application.Patch{Name: &renamed})),
		"revise unknown":         second(s.AddRevision(ctx, manager, unknown, arena(50))),
		"revisions of unknown":   second(s.Revisions(ctx, manager, unknown)),
		"revision of malformed":  second(s.Revision(ctx, manager, "arena", 1)),
		"revision out of bounds": second(s.Revision(ctx, manager, a.Environment.ID, 2)),
	} {
		want := application.ErrEnvironmentNotFound
		if name == "revision out of bounds" || name == "revision of malformed" {
			want = application.ErrRevisionNotFound
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: err=%v want %v", name, err, want)
		}
	}
}

func second[T any](_ T, err error) error { return err }
