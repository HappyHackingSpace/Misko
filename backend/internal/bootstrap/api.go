package bootstrap

import (
	"context"
	"errors"
	analysishttp "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/http"
	calibrationcatalog "github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/catalog"
	calibrationhttp "github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/http"
	calibrationpostgres "github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/postgres"
	calibrationapp "github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	environmentscatalog "github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/catalog"
	environmentshttp "github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/http"
	environmentspostgres "github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/postgres"
	environmentsapp "github.com/HappyHackingSpace/Misko/backend/internal/environments/application"
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
	mediagcs "github.com/HappyHackingSpace/Misko/backend/internal/media/adapters/gcs"
	mediahttp "github.com/HappyHackingSpace/Misko/backend/internal/media/adapters/http"
	mediapostgres "github.com/HappyHackingSpace/Misko/backend/internal/media/adapters/postgres"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	paradigmshttp "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/adapters/http"
	paradigmsapp "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/httpserver"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/ratelimit"
	protocolscatalog "github.com/HappyHackingSpace/Misko/backend/internal/protocols/adapters/catalog"
	protocolshttp "github.com/HappyHackingSpace/Misko/backend/internal/protocols/adapters/http"
	protocolspostgres "github.com/HappyHackingSpace/Misko/backend/internal/protocols/adapters/postgres"
	protocolsapp "github.com/HappyHackingSpace/Misko/backend/internal/protocols/application"
	reportshttp "github.com/HappyHackingSpace/Misko/backend/internal/reports/adapters/http"
	reportspostgres "github.com/HappyHackingSpace/Misko/backend/internal/reports/adapters/postgres"
	reportsapp "github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	subjectshttp "github.com/HappyHackingSpace/Misko/backend/internal/subjects/adapters/http"
	subjectspostgres "github.com/HappyHackingSpace/Misko/backend/internal/subjects/adapters/postgres"
	subjectsapp "github.com/HappyHackingSpace/Misko/backend/internal/subjects/application"
	testshttp "github.com/HappyHackingSpace/Misko/backend/internal/tests/adapters/http"
	testspostgres "github.com/HappyHackingSpace/Misko/backend/internal/tests/adapters/postgres"
	testsapp "github.com/HappyHackingSpace/Misko/backend/internal/tests/application"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Run is the composition root. Readiness, rather than liveness, depends on PostgreSQL.
func Run(ctx context.Context, cfg config.Config, auth config.Auth, storage config.Storage, logger *slog.Logger) error {
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
	var options []Option
	if storage.Enabled() {
		objects, err := mediagcs.New(ctx, storage.Bucket, storage.SignerEmail)
		if err != nil {
			return errors.New("could not initialize Cloud Storage client")
		}
		defer objects.Close()
		options = append(options, WithStorage(objects, mediaapp.Settings{MaxBytes: storage.MaxVideoBytes, UploadTTL: storage.UploadURLTTL, ReadTTL: storage.ReadURLTTL}))
	} else {
		logger.Warn("GCS_BUCKET is not set; video upload and read routes return 503")
	}
	api, err := NewAPI(pool, cfg, auth, logger, options...)
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

// reportExportLimit caps rows read by one export or summary request.
const reportExportLimit = 100_000

type API struct {
	Handler    http.Handler
	BeginDrain func()
	// AnalysisTick runs one analysis scheduler tick; cmd/jobs runs it periodically.
	AnalysisTick func(context.Context) error
}

type options struct {
	objects       mediaapp.ObjectStore
	mediaSettings mediaapp.Settings
}

type Option func(*options)

// WithStorage enables video routes with an object store.
func WithStorage(objects mediaapp.ObjectStore, settings mediaapp.Settings) Option {
	return func(o *options) { o.objects, o.mediaSettings = objects, settings }
}

// NewAPI wires concrete adapters into use cases and mounts every HTTP route.
func NewAPI(pool *pgxpool.Pool, cfg config.Config, auth config.Auth, logger *slog.Logger, opts ...Option) (API, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
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
	environments := environmentsapp.New(environmentspostgres.NewStore(pool), environmentscatalog.New())
	protocols := protocolsapp.New(protocolspostgres.NewStore(pool), protocolscatalog.New())
	// One user may create or edit at most 20 comments per minute on this instance.
	tests := testsapp.New(testspostgres.NewStore(pool), ratelimit.New(20, time.Minute, time.Now), time.Now)
	media := mediaapp.New(mediapostgres.NewStore(pool), o.objects, o.mediaSettings, time.Now)
	calibration := calibrationapp.New(calibrationpostgres.NewStore(pool), calibrationcatalog.New(), time.Now)
	analysis := newAnalysis(pool, o.objects, o.mediaSettings)
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
	paradigmshttp.Register(mux, paradigmsapp.New(), authenticate, logger)
	environmentshttp.Register(mux, environments, authenticate, logger)
	protocolshttp.Register(mux, protocols, authenticate, logger)
	testshttp.Register(mux, tests, authenticate, logger)
	mediahttp.Register(mux, media, authenticate, logger)
	calibrationhttp.Register(mux, calibration, authenticate, logger)
	analysishttp.Register(mux, analysis, authenticate, logger)
	reportshttp.Register(mux, reportsapp.New(reportspostgres.NewStore(pool), reportExportLimit), authenticate, logger)
	tick := func(ctx context.Context) error {
		_, err := analysis.Tick(ctx)
		return err
	}
	return API{Handler: mux, BeginDrain: readiness.BeginDrain, AnalysisTick: tick}, nil
}
