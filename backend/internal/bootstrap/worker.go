package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	analysiscatalog "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/catalog"
	analysispostgres "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres"
	analysistoken "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/token"
	analysisapp "github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	analysisdomain "github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoAdministrator means setup has not run yet, so no SUPERADMIN exists to
// record as the worker's creator.
var ErrNoAdministrator = errors.New("no administrator exists yet; run `bootstrap setup` first")

// The seed worker matches the bundled Python worker: its name is fixed, its
// model version and capabilities mirror worker/misko_worker/paradigms.py
// (SUPPORTED) and worker/README.md.
const (
	devWorkerName  = "dev-worker"
	devWorkerModel = "misko-tracker-bgsub 2.2.0"
)

func devWorkerCapabilities() []analysisdomain.Capability {
	return []analysisdomain.Capability{
		{ParadigmKey: "MWM", ParadigmVersion: 1},
		{ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1},
		{ParadigmKey: "POLE", ParadigmVersion: 1},
		{ParadigmKey: "TREADMILL", ParadigmVersion: 1},
	}
}

// RegisterWorker registers an analysis worker directly against the database,
// so an operator needs no admin JWT. It goes through the same service as
// POST /api/analysis/workers: the same validation, a generated token, and only
// the token's SHA-256 hash stored. The administrator setup created is recorded
// as the creator.
func RegisterWorker(ctx context.Context, pool *pgxpool.Pool, in analysisapp.WorkerInput) (analysisapp.Registered, error) {
	admin, err := administratorID(ctx, pool)
	if err != nil {
		return analysisapp.Registered{}, err
	}
	return workerService(pool).RegisterWorker(ctx, access.Actor{UserID: admin, Role: access.SuperAdmin}, in)
}

// seedWorker creates the dev-worker from the configured seed token. Like the
// rest of setup, repeated runs create nothing: an existing dev-worker (even one
// seeded from an older token) is left as it is, and a disabled one stays
// revoked rather than being revived.
func seedWorker(ctx context.Context, pool *pgxpool.Pool, token string) (bool, error) {
	admin, err := administratorID(ctx, pool)
	if err != nil {
		return false, err
	}
	in := analysisapp.WorkerInput{Name: devWorkerName, ModelVersion: devWorkerModel, Capabilities: devWorkerCapabilities()}
	if _, err := workerService(pool).SeedWorker(ctx, in, token, admin); err != nil {
		if errors.Is(err, analysisapp.ErrWorkerNameTaken) || errors.Is(err, analysisapp.ErrWorkerTokenTaken) {
			return false, nil
		}
		return false, fmt.Errorf("seed dev worker: %w", err)
	}
	return true, nil
}

// workerService wires only what registration needs; storage and the listener
// stay unconfigured because the bootstrap commands never touch jobs.
func workerService(pool *pgxpool.Pool) *analysisapp.Service {
	return analysisapp.New(analysisapp.Dependencies{
		Store: analysispostgres.NewStore(pool), Catalog: analysiscatalog.New(), Tokens: analysistoken.New(),
	}, analysisapp.Settings{}, time.Now)
}

// administratorID returns the first SUPERADMIN, created by setup.
func administratorID(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `SELECT id::text FROM misko.users WHERE role = 'SUPERADMIN' ORDER BY created_at, id LIMIT 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoAdministrator
	}
	if err != nil {
		return "", fmt.Errorf("find administrator: %w", err)
	}
	return id, nil
}

// ParseCapability reads a -paradigm argument: a paradigm key, optionally with
// ":VERSION" (1 when omitted), for example "MWM" or "MWM:1".
func ParseCapability(raw string) (analysisdomain.Capability, error) {
	key, version, versioned := strings.Cut(strings.TrimSpace(raw), ":")
	c := analysisdomain.Capability{ParadigmKey: key, ParadigmVersion: 1}
	if versioned {
		v, err := strconv.Atoi(version)
		if err != nil || v < 1 {
			return analysisdomain.Capability{}, fmt.Errorf("%q must name its paradigm version as KEY:VERSION with VERSION >= 1", raw)
		}
		c.ParadigmVersion = v
	}
	if c.ParadigmKey == "" {
		return analysisdomain.Capability{}, fmt.Errorf("%q must name a paradigm key", raw)
	}
	return c, nil
}
