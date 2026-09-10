package schema

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

//go:embed 001_initial.sql
var initialSQL string

// Install initializes a fresh database explicitly. API startup never changes schema.
func Install(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schema installation: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err := tx.Exec(ctx, initialSQL); err != nil {
		return fmt.Errorf("install schema: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schema: %w", err)
	}
	return nil
}
