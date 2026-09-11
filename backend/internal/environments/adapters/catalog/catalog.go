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

// CheckApparatus requires every apparatus parameter of the paradigm version,
// within range and consistent under session defaults.
func (Catalog) CheckApparatus(key string, version int, apparatus map[string]float64) error {
	_, err := paradigms.ResolveSetup(key, version, apparatus, nil)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, paradigms.ErrUnknownParadigm):
		return application.ErrUnknownParadigm
	case errors.Is(err, paradigms.ErrUnknownVersion):
		return application.ErrUnknownParadigmVersion
	case errors.Is(err, paradigms.ErrUnknownParameter), errors.Is(err, paradigms.ErrMissingParameter), errors.Is(err, paradigms.ErrParameterOutOfRange):
		return fmt.Errorf("%w: %v", application.ErrInvalidApparatus, err)
	}
	return err
}
