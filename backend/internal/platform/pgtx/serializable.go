// Package pgtx runs PostgreSQL transactions with bounded serialization retries.
package pgtx

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"math/rand/v2"
	"time"
)

const attempts = 5

// Serializable runs fn in a SERIALIZABLE transaction. Serialization failures
// and deadlocks, including those raised at commit, are retried a bounded number
// of times, so fn must be safe to re-run. Other errors are returned unchanged.
func Serializable(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.Serializable}, fn)
		if !retryable(err) || attempt == attempts {
			return err
		}
		backoff := time.Duration(1+rand.IntN(attempt*20)) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return err
}

func retryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}
