package bootstrap

import (
	"context"
	"errors"
	"testing"

	analysisdomain "github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
)

// TestSetupRefusesTheWorkerSeedInProduction proves the refusal happens before
// anything is touched: the nil pool would fail any database access.
func TestSetupRefusesTheWorkerSeedInProduction(t *testing.T) {
	for _, environment := range []string{"production", "PRODUCTION", "Production", "prod"} {
		_, err := Setup(context.Background(), nil, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "UTC",
			WorkerSeedToken: "mw_dev_local_only_not_a_secret", Environment: environment})
		if !errors.Is(err, ErrWorkerSeedInProduction) {
			t.Errorf("APP_ENV=%s: err=%v, want the seed refused", environment, err)
		}
	}
}

func TestParseCapability(t *testing.T) {
	for raw, want := range map[string]analysisdomain.Capability{
		"MWM":            {ParadigmKey: "MWM", ParadigmVersion: 1},
		" TREADMILL:3 ":  {ParadigmKey: "TREADMILL", ParadigmVersion: 3},
		"NOVEL_OBJECT:1": {ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1},
	} {
		if got, err := ParseCapability(raw); err != nil || got != want {
			t.Errorf("ParseCapability(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", ":", ":1", "MWM:", "MWM:0", "MWM:-1", "MWM:one", "MWM:1:2"} {
		if got, err := ParseCapability(raw); err == nil {
			t.Errorf("ParseCapability(%q) = %+v, want an error", raw, got)
		}
	}
}
