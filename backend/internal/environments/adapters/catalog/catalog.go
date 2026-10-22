// Package catalog checks environment measurements against the hardcoded
// paradigm catalog.
package catalog

import (
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/application"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

// CheckParameters requires every apparatus parameter of the paradigm version,
// accepts optional analysis parameters and rejects any other key; values must
// be within range and consistent under session defaults.
func (Catalog) CheckParameters(key string, version int, parameters map[string]float64) error {
	_, err := paradigms.ResolveSetup(key, version, parameters, nil)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, paradigms.ErrUnknownParadigm):
		return application.ErrUnknownParadigm
	case errors.Is(err, paradigms.ErrUnknownVersion):
		return application.ErrUnknownParadigmVersion
	case errors.Is(err, paradigms.ErrUnknownParameter), errors.Is(err, paradigms.ErrMissingParameter), errors.Is(err, paradigms.ErrParameterOutOfRange):
		return fmt.Errorf("%w: %v", application.ErrInvalidParameters, err)
	}
	return err
}
