// Package schema owns the database schema as an ordered list of migrations.
package schema

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"time"
)

// Files are named NNN_name.sql and numbered from 001 without gaps. An applied
// file must never change: its checksum is recorded and verified on every run.
//
//go:embed *.sql
var files embed.FS

var (
	// ErrModified means an applied migration file no longer matches the database.
	ErrModified = errors.New("an applied migration was changed")
	// ErrUnknownSchema means the database has a misko schema this code cannot place.
	ErrUnknownSchema = errors.New("the existing misko schema is not a known installation")
)

// lockKey serializes concurrent migrators; it is an arbitrary constant.
const lockKey = 7_314_520_261

// Databases installed before migrations were tracked always hold 001-012, which
// shipped together; 013 shipped later and is recorded only if its objects exist.
const (
	legacyBaseline = 12
	legacyMarker   = "misko.report_runs"
)

var legacyOptional = map[int]string{13: "misko.effective_calibrations"}

type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

var fileName = regexp.MustCompile(`^(\d{3})_[a-z0-9_]+\.sql$`)

// Migrations returns the embedded migrations in order.
func Migrations() ([]Migration, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	slices.Sort(names)
	migrations := make([]Migration, 0, len(names))
	for i, name := range names {
		match := fileName.FindStringSubmatch(name)
		if match == nil {
			return nil, fmt.Errorf("migration %s: name must look like 001_name.sql", name)
		}
		version, _ := strconv.Atoi(match[1])
		if version != i+1 {
			return nil, fmt.Errorf("migration %s: expected version %03d", name, i+1)
		}
		sql, err := files.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		sum := sha256.Sum256(sql)
		migrations = append(migrations, Migration{Version: version, Name: name, SQL: string(sql), Checksum: hex.EncodeToString(sum[:])})
	}
	return migrations, nil
}

// Latest is the version the code expects the database to be at.
func Latest() int {
	migrations, err := Migrations()
	if err != nil || len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

// Migrate installs a fresh database or upgrades an existing one. All pending
// migrations run in one transaction, so a failure leaves the database unchanged.
// It returns the names of the migrations it applied.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	migrations, err := Migrations()
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin migration: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		return nil, fmt.Errorf("lock migrations: %w", err)
	}
	recorded, err := appliedVersions(ctx, tx, migrations)
	if err != nil {
		return nil, err
	}
	var applied []string
	for _, m := range migrations {
		if _, ok := recorded[m.Version]; ok {
			continue
		}
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			return nil, fmt.Errorf("apply %s: %w", m.Name, err)
		}
		applied = append(applied, m.Name)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS misko.schema_migrations (
		version    integer PRIMARY KEY,
		name       text NOT NULL,
		checksum   text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("create migration table: %w", err)
	}
	for _, m := range migrations {
		if _, err := tx.Exec(ctx, `INSERT INTO misko.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)
			ON CONFLICT (version) DO NOTHING`, m.Version, m.Name, m.Checksum); err != nil {
			return nil, fmt.Errorf("record %s: %w", m.Name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit migrations: %w", err)
	}
	return applied, nil
}

// appliedVersions reads the migration table, or infers it for a database that
// predates it. Versions newer than this code are ignored so a rollback can start.
func appliedVersions(ctx context.Context, tx pgx.Tx, migrations []Migration) (map[int]struct{}, error) {
	var installed, tracked bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'misko'),
		to_regclass('misko.schema_migrations') IS NOT NULL`).Scan(&installed, &tracked); err != nil {
		return nil, fmt.Errorf("inspect schema: %w", err)
	}
	applied := map[int]struct{}{}
	if !installed {
		return applied, nil
	}
	if !tracked {
		return legacyVersions(ctx, tx)
	}
	rows, err := tx.Query(ctx, "SELECT version, checksum FROM misko.schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	checksums := map[int]string{}
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read migrations: %w", err)
		}
		checksums[version] = checksum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	for _, m := range migrations {
		checksum, ok := checksums[m.Version]
		if !ok {
			continue
		}
		if checksum != m.Checksum {
			return nil, fmt.Errorf("%w: %s", ErrModified, m.Name)
		}
		applied[m.Version] = struct{}{}
	}
	return applied, nil
}

func legacyVersions(ctx context.Context, tx pgx.Tx) (map[int]struct{}, error) {
	if found, err := exists(ctx, tx, legacyMarker); err != nil || !found {
		return nil, errors.Join(ErrUnknownSchema, err)
	}
	applied := map[int]struct{}{}
	for v := 1; v <= legacyBaseline; v++ {
		applied[v] = struct{}{}
	}
	for version, marker := range legacyOptional {
		found, err := exists(ctx, tx, marker)
		if err != nil {
			return nil, err
		}
		if found {
			applied[version] = struct{}{}
		}
	}
	return applied, nil
}

func exists(ctx context.Context, tx pgx.Tx, relation string) (bool, error) {
	var found bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", relation).Scan(&found); err != nil {
		return false, fmt.Errorf("inspect %s: %w", relation, err)
	}
	return found, nil
}
