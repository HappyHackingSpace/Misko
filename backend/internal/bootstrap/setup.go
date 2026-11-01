package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/password"
	identitypostgres "github.com/HappyHackingSpace/Misko/backend/internal/identity/adapters/postgres"
	identityapp "github.com/HappyHackingSpace/Misko/backend/internal/identity/application"
	identitydomain "github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	laboratorypostgres "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/adapters/postgres"
	laboratoryapp "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/application"
	laboratorydomain "github.com/HappyHackingSpace/Misko/backend/internal/laboratory/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type SetupResult struct {
	LaboratoryCreated    bool
	AdministratorCreated bool
	// AdminPassword is set only when the administrator was created by this run.
	// It is shown to the operator once and must never be logged.
	AdminPassword string
	// WorkerSeeded reports that this run created the dev-worker from
	// WORKER_BOOTSTRAP_TOKEN. The token needs no printing: the operator set it.
	WorkerSeeded bool
}

// ErrWorkerSeedInProduction keeps the public development token out of real
// deployments: the seed only ever runs where APP_ENV is not production.
var ErrWorkerSeedInProduction = errors.New("WORKER_BOOTSTRAP_TOKEN must not be set when APP_ENV is production; use `bootstrap register-worker` instead")

// Setup creates the laboratory singleton, the first SUPERADMIN and, when a
// development seed token is configured, the dev-worker. Inputs are validated
// before any write, and repeated runs create none of them.
func Setup(ctx context.Context, pool *pgxpool.Pool, cfg config.Setup) (SetupResult, error) {
	if cfg.WorkerSeedToken != "" && production(cfg.Environment) {
		return SetupResult{}, ErrWorkerSeedInProduction
	}
	if _, err := identitydomain.NormalizeEmail(cfg.AdminEmail); err != nil {
		return SetupResult{}, fmt.Errorf("ADMIN_EMAIL: %w", err)
	}
	if _, err := laboratorydomain.NewLaboratory(cfg.LabName, "", cfg.LabTimezone); err != nil {
		return SetupResult{}, fmt.Errorf("laboratory: %w", err)
	}
	passwords, err := password.NewBcrypt(cfg.BcryptCost)
	if err != nil {
		return SetupResult{}, err
	}
	var result SetupResult
	laboratory := laboratoryapp.New(laboratorypostgres.NewStore(pool))
	if result.LaboratoryCreated, err = laboratory.Initialize(ctx, cfg.LabName, "", cfg.LabTimezone); err != nil {
		return SetupResult{}, fmt.Errorf("initialize laboratory: %w", err)
	}
	// Setup never issues tokens, so no token adapter is wired.
	identity := identityapp.New(identitypostgres.NewStore(pool), nil, passwords)
	if result.AdministratorCreated, result.AdminPassword, err = identity.BootstrapAdministrator(ctx, cfg.AdminEmail); err != nil {
		return SetupResult{}, fmt.Errorf("create administrator: %w", err)
	}
	if cfg.WorkerSeedToken != "" {
		if result.WorkerSeeded, err = seedWorker(ctx, pool, cfg.WorkerSeedToken); err != nil {
			return SetupResult{}, err
		}
	}
	return result, nil
}

// production matches APP_ENV values that mean a real deployment.
func production(environment string) bool {
	return strings.EqualFold(environment, "production") || strings.EqualFold(environment, "prod")
}
