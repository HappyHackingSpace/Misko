package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependency is ready once the database is migrated to at least the version
// the running code was built for. A newer database is accepted so a rollback
// of the code can still start.
type Dependency struct {
	pool     *pgxpool.Pool
	required int
}

func New(pool *pgxpool.Pool, required int) *Dependency {
	return &Dependency{pool: pool, required: required}
}

func (d *Dependency) Check(ctx context.Context) error {
	var tracked bool
	if err := d.pool.QueryRow(ctx, "SELECT to_regclass('misko.schema_migrations') IS NOT NULL").Scan(&tracked); err != nil {
		return err
	}
	if !tracked {
		return fmt.Errorf("application schema is not migrated; run bootstrap migrate")
	}
	var version int
	if err := d.pool.QueryRow(ctx, "SELECT coalesce(max(version), 0) FROM misko.schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version < d.required {
		return fmt.Errorf("schema version %d is older than the required %d; run bootstrap migrate", version, d.required)
	}
	return nil
}
