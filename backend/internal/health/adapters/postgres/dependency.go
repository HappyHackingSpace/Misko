package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependency struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Dependency { return &Dependency{pool: pool} }

func (d *Dependency) Check(ctx context.Context) error {
	var exists bool
	if err := d.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'misko')").Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("application schema is not installed")
	}
	return nil
}
