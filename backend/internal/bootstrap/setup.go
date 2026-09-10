package bootstrap

import (
	"context"
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
)

type SetupResult struct {
	LaboratoryCreated    bool
	AdministratorCreated bool
	// AdminPassword is set only when the administrator was created by this run.
	// It is shown to the operator once and must never be logged.
	AdminPassword string
}

// Setup creates the laboratory singleton and the first SUPERADMIN. Inputs are
// validated before any write, and repeated runs create neither.
func Setup(ctx context.Context, pool *pgxpool.Pool, cfg config.Setup) (SetupResult, error) {
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
	return result, nil
}
