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

// attempts bounds the retries. Contending transactions (for example several
// processes bootstrapping the first administrator at once) can conflict a few
// times in a row; the growing, jittered backoff spreads them apart.
const attempts = 10

// maxBackoff caps the wait between two attempts.
const maxBackoff = 200 * time.Millisecond

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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(attempt)):
		}
	}
	return err
}

// backoff is a full-jitter exponential wait after the given attempt: a random
// duration between 1 ms and min(maxBackoff, 5 ms x 2^(attempt-1)).
func backoff(attempt int) time.Duration {
	ceiling := min(maxBackoff, 5*time.Millisecond<<min(attempt-1, 16))
	return time.Millisecond + time.Duration(rand.Int64N(int64(ceiling)))
}

func retryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}
