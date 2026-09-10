package bootstrap

import (
	"context"
	"errors"
	healthhttp "github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/http"
	healthpostgres "github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/postgres"
	healthapp "github.com/HappyHackingSpace/Misko/backend/internal/health/application"
	identityhttp "github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/http"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/password"
	identitypostgres "github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/token"
	identityapp "github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	laboratoryhttp "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/adapters/http"
	laboratorypostgres "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/adapters/postgres"
	laboratoryapp "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpserver"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Run is the composition root. Readiness, rather than liveness, depends on PostgreSQL.
func Run(ctx context.Context, cfg config.Config, auth config.Auth, logger *slog.Logger) error {
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
	api, err := NewAPI(pool, cfg, auth, logger)
	if err != nil {
		return err
	}
	server := httpserver.New(api.Handler)
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("could not bind HTTP listener")
	}
	logger.Info("HTTP server listening", "address", listener.Addr().String())
	return httpserver.Serve(ctx, listener, server, cfg.ShutdownTimeout, api.BeginDrain)
}

type API struct {
	Handler    http.Handler
	BeginDrain func()
}

// NewAPI wires concrete adapters into use cases and mounts every HTTP route.
func NewAPI(pool *pgxpool.Pool, cfg config.Config, auth config.Auth, logger *slog.Logger) (API, error) {
	tokens, err := token.NewJWT(auth.JWTSecret, auth.JWTIssuer, auth.JWTAudience, auth.TokenTTL, time.Now)
	if err != nil {
		return API{}, err
	}
	passwords, err := password.NewBcrypt(auth.BcryptCost)
	if err != nil {
		return API{}, err
	}
	identity := identityapp.New(identitypostgres.NewStore(pool), tokens, passwords)
	laboratory := laboratoryapp.New(laboratorypostgres.NewStore(pool))
	readiness := healthapp.New(healthpostgres.New(pool), cfg.ProbeTimeout)

	mux := http.NewServeMux()
	health := healthhttp.New(readiness)
	mux.Handle("/api/health", health)
	mux.Handle("/api/ready", health)
	identityhttp.Register(mux, identity, logger)
	laboratoryhttp.Register(mux, laboratory, identityhttp.Authenticator(identity), logger)
	return API{Handler: mux, BeginDrain: readiness.BeginDrain}, nil
}
