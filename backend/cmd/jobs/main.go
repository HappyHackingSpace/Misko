package main

import (
	"context"
	"github.com/HappyHackingSpace/Misko/backend/internal/bootstrap"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // laboratory time zones must not depend on the container image
)

func main() { os.Exit(run()) }

// run ticks the analysis scheduler every JOB_INTERVAL (default 10s) until it is
// stopped. It serves no HTTP.
func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("configuration rejected", "error", err)
		return 1
	}
	interval := 10 * time.Second
	if raw := os.Getenv("JOB_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < time.Second {
			logger.Error("configuration rejected", "error", "JOB_INTERVAL must be a duration of at least 1s")
			return 1
		}
		interval = parsed
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("invalid PostgreSQL configuration")
		return 1
	}
	defer pool.Close()
	logger.Info("analysis scheduler started", "interval", interval.String())
	bootstrap.RunScheduler(ctx, pool, interval, logger)
	logger.Info("analysis scheduler stopped")
	return 0
}
