// Package catalog reads trial types and session rules from the hardcoded
// paradigm catalog for protocol steps.
package catalog

import (
	"errors"
	"fmt"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/application"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

func (Catalog) TrialTypes(key string, version int) ([]string, error) {
	p, err := paradigms.Version(key, version)
	if err != nil {
		return nil, translate(err)
	}
	return p.TrialTypes, nil
}

func (Catalog) ResolveSession(key string, version int, apparatus, session map[string]float64) (map[string]float64, error) {
	resolved, err := paradigms.ResolveSetup(key, version, apparatus, session)
	if err != nil {
		return nil, translate(err)
	}
	return resolved, nil
}

func translate(err error) error {
	switch {
	case errors.Is(err, paradigms.ErrUnknownParadigm):
		return application.ErrUnknownParadigm
	case errors.Is(err, paradigms.ErrUnknownVersion):
		return application.ErrUnknownParadigmVersion
	case errors.Is(err, paradigms.ErrUnknownParameter), errors.Is(err, paradigms.ErrMissingParameter), errors.Is(err, paradigms.ErrParameterOutOfRange):
		return fmt.Errorf("%w: %v", application.ErrInvalidSession, err)
	}
	return err
}
