//go:build integration

package schema

import (
	"context"
	healthpostgres "github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestCleanInstallAndNonDestructiveRepeat(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL must point to a disposable empty PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	dependency := healthpostgres.New(pool)
	if dependency.Check(ctx) == nil {
		t.Fatal("uninitialized database marked ready")
	}
	// A fresh database is a precondition: never drop schemas or tables in tests.
	if err := Install(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := dependency.Check(ctx); err != nil {
		t.Fatalf("installed database not ready: %v", err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if dependency.Check(cancelled) == nil {
		t.Fatal("cancelled query succeeded")
	}
	if _, err := pool.Exec(ctx, "CREATE TABLE misko.fixture (value text NOT NULL); INSERT INTO misko.fixture VALUES ('preserve')"); err != nil {
		t.Fatal(err)
	}
	if err := Install(ctx, pool); err == nil {
		t.Fatal("repeated installation should refuse an existing schema")
	}
	var value string
	if err := pool.QueryRow(ctx, "SELECT value FROM misko.fixture").Scan(&value); err != nil || value != "preserve" {
		t.Fatalf("existing data changed: %q %v", value, err)
	}
}
