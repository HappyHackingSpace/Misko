//go:build integration

package schema_test

import (
	"context"
	"crypto/rand"
	"errors"
	healthpostgres "github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/schema"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// emptyDatabase creates a uniquely named, empty database and drops it afterwards.
func emptyDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL must point to a disposable PostgreSQL server")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := pgx.Identifier{"misko_schema_" + strings.ToLower(rand.Text())}.Sanitize()
	if _, err := server.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = strings.Trim(name, `"`)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := server.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		_ = server.Close(ctx)
	})
	return pool
}

// installUntracked reproduces an installation made before migrations were
// tracked: the first n files applied in one transaction, nothing recorded.
func installUntracked(t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	migrations, err := schema.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var sql strings.Builder
	for _, m := range migrations[:n] {
		sql.WriteString(m.SQL)
		sql.WriteString("\n")
	}
	if _, err := pool.Exec(context.Background(), "BEGIN;\n"+sql.String()+"\nCOMMIT;"); err != nil {
		t.Fatal(err)
	}
}

func versions(t *testing.T, pool *pgxpool.Pool) []int {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT version FROM misko.schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func all() []int {
	out := make([]int, schema.Latest())
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func subjectColumns(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'misko' AND table_name = 'subjects' AND column_name IN ('strain', 'birth_date', 'notes')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFreshInstallIsRecordedAndRepeatable(t *testing.T) {
	ctx := context.Background()
	pool := emptyDatabase(t)
	dependency := healthpostgres.New(pool, schema.Latest())
	if dependency.Check(ctx) == nil {
		t.Fatal("uninitialized database marked ready")
	}
	applied, err := schema.Migrate(ctx, pool)
	if err != nil || len(applied) != schema.Latest() {
		t.Fatalf("fresh install applied %v: %v", applied, err)
	}
	if got := versions(t, pool); !slices.Equal(got, all()) {
		t.Fatalf("recorded versions %v", got)
	}
	if err := dependency.Check(ctx); err != nil {
		t.Fatalf("migrated database not ready: %v", err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if dependency.Check(cancelled) == nil {
		t.Fatal("cancelled query succeeded")
	}
	if _, err := pool.Exec(ctx, "CREATE TABLE misko.fixture (value text NOT NULL); INSERT INTO misko.fixture VALUES ('preserve')"); err != nil {
		t.Fatal(err)
	}
	if applied, err := schema.Migrate(ctx, pool); err != nil || len(applied) != 0 {
		t.Fatalf("repeated migration applied %v: %v", applied, err)
	}
	var value string
	if err := pool.QueryRow(ctx, "SELECT value FROM misko.fixture").Scan(&value); err != nil || value != "preserve" {
		t.Fatalf("existing data changed: %q %v", value, err)
	}
}

func TestUntrackedInstallationIsAdoptedAndUpgraded(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		installed int
		want      []string
	}{
		{"installed before environment calibrations", 12, []string{"013_environment_calibrations.sql", "014_drop_subject_optional_fields.sql"}},
		{"installed with environment calibrations", 13, []string{"014_drop_subject_optional_fields.sql"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := emptyDatabase(t)
			installUntracked(t, pool, tc.installed)
			if _, err := pool.Exec(ctx, "INSERT INTO misko.subjects (code, species, sex, strain, birth_date, notes) VALUES ('L-1', 'MOUSE', 'MALE', 'C57', '2026-01-01', 'tag')"); err != nil {
				t.Fatal(err)
			}
			if healthpostgres.New(pool, schema.Latest()).Check(ctx) == nil {
				t.Fatal("untracked installation marked ready")
			}
			applied, err := schema.Migrate(ctx, pool)
			if err != nil || !slices.Equal(applied, tc.want) {
				t.Fatalf("applied %v want %v: %v", applied, tc.want, err)
			}
			if got := versions(t, pool); !slices.Equal(got, all()) {
				t.Fatalf("recorded versions %v", got)
			}
			var code string
			if err := pool.QueryRow(ctx, "SELECT code FROM misko.subjects").Scan(&code); err != nil || code != "L-1" {
				t.Fatalf("subject lost: %q %v", code, err)
			}
			if n := subjectColumns(t, pool); n != 0 {
				t.Fatalf("%d removed subject columns remain", n)
			}
			if err := healthpostgres.New(pool, schema.Latest()).Check(ctx); err != nil {
				t.Fatalf("upgraded database not ready: %v", err)
			}
		})
	}
}

func TestUnknownSchemaIsRefusedUnchanged(t *testing.T) {
	ctx := context.Background()
	pool := emptyDatabase(t)
	if _, err := pool.Exec(ctx, "CREATE SCHEMA misko; CREATE TABLE misko.other (id int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Migrate(ctx, pool); !errors.Is(err, schema.ErrUnknownSchema) {
		t.Fatalf("unknown schema: %v", err)
	}
	var tracked bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('misko.schema_migrations') IS NOT NULL").Scan(&tracked); err != nil || tracked {
		t.Fatalf("refused migration left a migration table: %v %v", tracked, err)
	}
}

func TestChangedMigrationIsRefused(t *testing.T) {
	ctx := context.Background()
	pool := emptyDatabase(t)
	if _, err := schema.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE misko.schema_migrations SET checksum = 'edited' WHERE version = 3"); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Migrate(ctx, pool); !errors.Is(err, schema.ErrModified) || !strings.Contains(err.Error(), "003_subjects.sql") {
		t.Fatalf("changed migration: %v", err)
	}
}

func TestPendingMigrationRunsAndBlocksReadiness(t *testing.T) {
	ctx := context.Background()
	pool := emptyDatabase(t)
	if _, err := schema.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// Simulate a database one release behind: undo 014 and forget it.
	if _, err := pool.Exec(ctx, `ALTER TABLE misko.subjects ADD COLUMN strain text, ADD COLUMN birth_date date, ADD COLUMN notes text;
		DELETE FROM misko.schema_migrations WHERE version = 14`); err != nil {
		t.Fatal(err)
	}
	if healthpostgres.New(pool, schema.Latest()).Check(ctx) == nil {
		t.Fatal("database behind the code marked ready")
	}
	if applied, err := schema.Migrate(ctx, pool); err != nil || !slices.Equal(applied, []string{"014_drop_subject_optional_fields.sql"}) {
		t.Fatalf("pending migration: %v %v", applied, err)
	}
	if n := subjectColumns(t, pool); n != 0 {
		t.Fatalf("%d removed subject columns remain", n)
	}
	// A database ahead of the code (a rollback) still starts.
	if _, err := pool.Exec(ctx, "INSERT INTO misko.schema_migrations (version, name, checksum) VALUES (999, '999_future.sql', 'x')"); err != nil {
		t.Fatal(err)
	}
	if applied, err := schema.Migrate(ctx, pool); err != nil || len(applied) != 0 {
		t.Fatalf("newer database: %v %v", applied, err)
	}
	if err := healthpostgres.New(pool, schema.Latest()).Check(ctx); err != nil {
		t.Fatalf("newer database not ready: %v", err)
	}
}
