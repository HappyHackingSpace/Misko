package schema

import (
	"context"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"slices"
	"time"
)

// Files are applied in lexical order inside one transaction.
//
//go:embed *.sql
var files embed.FS

// Install initializes a fresh database explicitly. API startup never changes schema.
func Install(ctx context.Context, pool *pgxpool.Pool) error {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return fmt.Errorf("list schema files: %w", err)
	}
	slices.Sort(names)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schema installation: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	for _, name := range names {
		sql, err := files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("install %s: %w", name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schema: %w", err)
	}
	return nil
}
