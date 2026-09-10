package bootstrap

import (
	"context"
	"errors"
	experimentshttp "github.com/HappyHackingSpace/Misko/backend/internal/experiments/adapters/http"
	experimentspostgres "github.com/HappyHackingSpace/Misko/backend/internal/experiments/adapters/postgres"
	experimentsapp "github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	healthhttp "github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/http"
	healthpostgres "github.com/HappyHackingSpace/Misko/backend/internal/health/adapters/postgres"
	healthapp "github.com/HappyHackingSpace/Misko/backend/internal/health/application"
	identityhttp "github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/http"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/password"
	identitypostgres "github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/token"
	identityapp "github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	interventionshttp "github.com/HappyHackingSpace/Misko/backend/internal/interventions/adapters/http"
	interventionspostgres "github.com/HappyHackingSpace/Misko/backend/internal/interventions/adapters/postgres"
	interventionsapp "github.com/HappyHackingSpace/Misko/backend/internal/interventions/application"
	laboratoryhttp "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/adapters/http"
	laboratorypostgres "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/adapters/postgres"
	laboratoryapp "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpserver"
	subjectshttp "github.com/HappyHackingSpace/Misko/backend/internal/subjects/adapters/http"
	subjectspostgres "github.com/HappyHackingSpace/Misko/backend/internal/subjects/adapters/postgres"
	subjectsapp "github.com/HappyHackingSpace/Misko/backend/internal/subjects/application"
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
	subjects := subjectsapp.New(subjectspostgres.NewStore(pool), time.Now)
	experiments := experimentsapp.New(experimentspostgres.NewStore(pool))
	interventions := interventionsapp.New(interventionspostgres.NewStore(pool), time.Now)
	readiness := healthapp.New(healthpostgres.New(pool), cfg.ProbeTimeout)

	mux := http.NewServeMux()
	health := healthhttp.New(readiness)
	mux.Handle("/api/health", health)
	mux.Handle("/api/ready", health)
	identityhttp.Register(mux, identity, logger)
	authenticate := identityhttp.Authenticator(identity)
	laboratoryhttp.Register(mux, laboratory, authenticate, logger)
	subjectshttp.Register(mux, subjects, authenticate, logger)
	experimentshttp.Register(mux, experiments, authenticate, logger)
	interventionshttp.Register(mux, interventions, authenticate, logger)
	return API{Handler: mux, BeginDrain: readiness.BeginDrain}, nil
}
