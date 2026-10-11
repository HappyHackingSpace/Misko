//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/domain"
	"slices"
	"testing"
)

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

	if _, err := pool.Exec(ctx, `
		WITH e AS (INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id)
		INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at) SELECT e.id, $1, now() FROM e`, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, created.ID); !errors.Is(err, application.ErrInUse) {
		t.Fatalf("enrolled subject deleted: %v", err)
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
	store := postgres.NewStore(pgtest.Database(t))
	for _, s := range []domain.Subject{
		subject(t, "A1", "MOUSE", "FEMALE"),
		subject(t, "b_2", "MOUSE", "MALE"),
		subject(t, "C3", "RAT", "FEMALE"),
		subject(t, "D.4", "RAT", "MALE"),
	} {
		if _, err := store.Create(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
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
