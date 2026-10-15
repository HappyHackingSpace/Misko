//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"testing"
	"time"
)

// recorder stands in for a user id; weights and allocations keep it without a
// foreign key.
const recorder = "01900000-0000-7000-8000-000000000001"

// allocate creates a draft experiment with one healthy control group and
// allocates the subject to it, the way the experiments context will.
func allocate(t *testing.T, pool *pgxpool.Pool, subjectID, experimentCode, groupCode string) (experimentID, groupID string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		WITH e AS (INSERT INTO misko.experiments (code, title) VALUES ($2, 'Study') RETURNING id),
		     g AS (INSERT INTO misko.experiment_groups (experiment_id, code, name, role) SELECT e.id, $3, 'Healthy', 'HEALTHY_CONTROL' FROM e RETURNING id, experiment_id)
		INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		SELECT g.experiment_id, g.id, $1, now() - interval '1 hour', $4 FROM g
		RETURNING experiment_id, group_id`, subjectID, experimentCode, groupCode, recorder).Scan(&experimentID, &groupID)
	if err != nil {
		t.Fatal(err)
	}
	return experimentID, groupID
}

func subject(t *testing.T, code, species, sex string) domain.Subject {
	t.Helper()
	s, err := domain.NewSubject(code, species, sex)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSubjectStoreConstraintsAndUpdates(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	created, err := store.Create(ctx, subject(t, "F-001", "MOUSE", "FEMALE"))
	if err != nil || created.ID == "" || created.Code != "F-001" {
		t.Fatalf("create: %+v %v", created, err)
	}
	if _, err := store.Create(ctx, subject(t, "f-001", "RAT", "MALE")); !errors.Is(err, application.ErrCodeTaken) {
		t.Fatalf("code duplicate ignoring case: %v", err)
	}
	for _, id := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		if _, err := store.Subject(ctx, id); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("Subject(%q): %v", id, err)
		}
		if _, err := store.Update(ctx, id, application.Changes{Code: new(string)}); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("Update(%q): %v", id, err)
		}
		if err := store.Delete(ctx, id); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("Delete(%q): %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO misko.subjects (code, species, sex) VALUES ('X1', 'CAT', 'MALE')"); err == nil {
		t.Fatal("database accepted an unknown species")
	}

	rat := domain.Rat
	updated, err := store.Update(ctx, created.ID, application.Changes{Species: &rat})
	if err != nil || updated.Species != domain.Rat || updated.Code != "F-001" || updated.Sex != domain.Female {
		t.Fatalf("partial update: %+v %v", updated, err)
	}
	var removed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'misko' AND table_name = 'subjects' AND column_name IN ('strain', 'birth_date', 'notes')`).Scan(&removed); err != nil || removed != 0 {
		t.Fatalf("removed subject columns still installed: %d %v", removed, err)
	}

	if created.Allocation != nil {
		t.Fatalf("new subject has an allocation: %+v", created.Allocation)
	}
	experimentID, groupID := allocate(t, pool, created.ID, "E1", "G1")
	got, err := store.Subject(ctx, created.ID)
	want := domain.Allocation{ExperimentID: experimentID, ExperimentCode: "E1", GroupID: groupID, GroupCode: "G1", GroupRole: "HEALTHY_CONTROL"}
	if err != nil || got.Allocation == nil || *got.Allocation != want {
		t.Fatalf("allocation summary: %+v %v", got.Allocation, err)
	}
	male := domain.Male
	if updated, err := store.Update(ctx, created.ID, application.Changes{Sex: &male}); err != nil || updated.Allocation == nil || *updated.Allocation != want {
		t.Fatalf("update keeps the allocation summary: %+v %v", updated, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1`, experimentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE misko.allocations SET excluded_at = now(), exclusion_reason = 'DIED' WHERE subject_id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Subject(ctx, created.ID); err != nil || got.Allocation == nil || !got.Allocation.Excluded {
		t.Fatalf("excluded allocation: %+v %v", got.Allocation, err)
	}
	if err := store.Delete(ctx, created.ID); !errors.Is(err, application.ErrInUse) {
		t.Fatalf("allocated subject deleted: %v", err)
	}
	weighed, err := store.Create(ctx, subject(t, "W-1", "RAT", "MALE"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWeight(ctx, domain.Weight{SubjectID: weighed.ID, Milligrams: 250000, MeasuredAt: time.Now(), RecordedBy: recorder}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, weighed.ID); !errors.Is(err, application.ErrInUse) {
		t.Fatalf("weighed subject deleted: %v", err)
	}
	free, err := store.Create(ctx, subject(t, "M-2", "MOUSE", "MALE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, free.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Subject(ctx, free.ID); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("deleted subject readable: %v", err)
	}
}

func TestListSubjectsFiltersEscapesAndPages(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	ids := map[string]string{}
	for _, s := range []domain.Subject{
		subject(t, "A1", "MOUSE", "FEMALE"),
		subject(t, "b_2", "MOUSE", "MALE"),
		subject(t, "C3", "RAT", "FEMALE"),
		subject(t, "D.4", "RAT", "MALE"),
	} {
		created, err := store.Create(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		ids[created.Code] = created.ID
	}
	allocate(t, pool, ids["b_2"], "E1", "G1")
	allocate(t, pool, ids["C3"], "E2", "G1")
	for _, tc := range []struct {
		filter application.Filter
		codes  []string
		total  int
	}{
		{application.Filter{Search: "%", Sort: "code", Limit: 10}, nil, 0},
		{application.Filter{Search: "_", Sort: "code", Limit: 10}, []string{"b_2"}, 1},
		{application.Filter{Search: "c3", Sort: "code", Limit: 10}, []string{"C3"}, 1},
		{application.Filter{Species: domain.Rat, Sort: "code", Limit: 10}, []string{"C3", "D.4"}, 2},
		{application.Filter{Sex: domain.Male, Sort: "code", Descending: true, Limit: 10}, []string{"D.4", "b_2"}, 2},
		{application.Filter{Sort: "code", Limit: 2, Offset: 1}, []string{"b_2", "C3"}, 4},
		{application.Filter{Unallocated: true, Sort: "code", Limit: 10}, []string{"A1", "D.4"}, 2},
		{application.Filter{Unallocated: true, Species: domain.Rat, Sort: "code", Limit: 10}, []string{"D.4"}, 1},
	} {
		subjects, total, err := store.List(ctx, tc.filter)
		var codes []string
		for _, s := range subjects {
			codes = append(codes, s.Code)
		}
		if err != nil || !slices.Equal(codes, tc.codes) || total != tc.total {
			t.Errorf("%+v: codes=%v total=%d err=%v want %v %d", tc.filter, codes, total, err, tc.codes, tc.total)
		}
	}
}

func TestWeightsAreAppendOnlyAndNewestFirst(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	m, err := store.Create(ctx, subject(t, "M-1", "MOUSE", "MALE"))
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for i, mg := range []int64{25000, 25500, 24750} {
		w, err := store.CreateWeight(ctx, domain.Weight{SubjectID: m.ID, Milligrams: mg, MeasuredAt: day.AddDate(0, 0, i), RecordedBy: recorder})
		if err != nil || w.ID == "" || w.Milligrams != mg || !w.MeasuredAt.Equal(day.AddDate(0, 0, i)) {
			t.Fatalf("create weight %d: %+v %v", i, w, err)
		}
	}
	weights, err := store.Weights(ctx, m.ID)
	if err != nil || len(weights) != 3 || weights[0].Milligrams != 24750 || weights[2].Milligrams != 25000 {
		t.Fatalf("weights: %+v %v", weights, err)
	}
	for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		if _, err := store.CreateWeight(ctx, domain.Weight{SubjectID: id, Milligrams: 1, MeasuredAt: day, RecordedBy: recorder}); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("weight of %q: %v", id, err)
		}
		if list, err := store.Weights(ctx, id); err != nil || len(list) != 0 {
			t.Errorf("weights of %q: %v %v", id, list, err)
		}
	}
	for _, sql := range []string{
		"UPDATE misko.weight_measurements SET body_milligrams = 1 WHERE subject_id = $1",
		"DELETE FROM misko.weight_measurements WHERE subject_id = $1",
	} {
		if _, err := pool.Exec(ctx, sql, m.ID); err == nil {
			t.Errorf("%s succeeded", sql)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO misko.weight_measurements (subject_id, body_milligrams, measured_at, recorded_by) VALUES ($1, 5000001, now(), $2)`, m.ID, recorder); err == nil {
		t.Error("database accepted a weight above 5 kg")
	}
}
