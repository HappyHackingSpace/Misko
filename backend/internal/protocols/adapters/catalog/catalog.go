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
	return p.TrialTypeKeys(), nil
}

// ResolveSession validates session values without an arena. A step no longer
// names an environment, but ResolveSetup requires every apparatus value, so
// the paradigm's apparatus defaults stand in for them; analysis values fall
// back to their defaults inside ResolveSetup.
func (Catalog) ResolveSession(key string, version int, session map[string]float64) (map[string]float64, error) {
	p, err := paradigms.Version(key, version)
	if err != nil {
		return nil, translate(err)
	}
	apparatus := map[string]float64{}
	for _, param := range p.In(paradigms.ApparatusScope) {
		apparatus[param.Key] = param.Default
	}
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
