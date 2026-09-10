package bootstrap

import (
	"context"
	"errors"
	healthhttp "github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/http"
	"github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/health/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpserver"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net"
)

// Run is the composition root. Readiness, rather than liveness, depends on PostgreSQL.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid PostgreSQL connection configuration")
	}
	poolCfg.MaxConns = 10
	poolCfg.ConnConfig.ConnectTimeout = cfg.ProbeTimeout
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return errors.New("could not initialize PostgreSQL pool")
	}
	defer pool.Close()
	readiness := application.New(postgres.New(pool), cfg.ProbeTimeout)
	server := httpserver.New(healthhttp.New(readiness))
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("could not bind HTTP listener")
	}
	logger.Info("HTTP server listening", "address", listener.Addr().String())
	return httpserver.Serve(ctx, listener, server, cfg.ShutdownTimeout, readiness.BeginDrain)
}
