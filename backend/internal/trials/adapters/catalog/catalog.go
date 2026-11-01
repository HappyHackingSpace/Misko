// Package catalog checks a trial's ANALYSIS overrides against the hardcoded
// paradigm catalog.
package catalog

import (
	"fmt"
	"slices"

	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

// CheckOverrides accepts ANALYSIS parameters of the version whose values,
// applied to the arena's parameters, stay within range and consistent.
func (Catalog) CheckOverrides(key string, version int, parameters, overrides map[string]float64) error {
	if len(overrides) == 0 {
		return nil
	}
	p, err := paradigms.Version(key, version)
	if err != nil {
		return fmt.Errorf("paradigm %s v%d: %w", key, version, err)
	}
	analysis := p.In(paradigms.AnalysisScope)
	for k := range overrides {
		if !slices.ContainsFunc(analysis, func(param paradigms.Parameter) bool { return param.Key == k }) {
			return fmt.Errorf("%w: %s is not an ANALYSIS parameter", domain.ErrInvalidOverrides, k)
		}
	}
	if _, err := paradigms.ResolveSetup(key, version, domain.Merge(parameters, overrides), nil); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalidOverrides, err)
	}
	return nil
}
