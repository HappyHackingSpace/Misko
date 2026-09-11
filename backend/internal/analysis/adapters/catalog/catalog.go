// Package catalog reads the analysis contract of a paradigm version from the
// hardcoded paradigm catalog.
package catalog

import (
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

func (Catalog) Contract(key string, version int) (domain.Contract, error) {
	p, err := paradigms.Version(key, version)
	if errors.Is(err, paradigms.ErrUnknownParadigm) || errors.Is(err, paradigms.ErrUnknownVersion) {
		return domain.Contract{}, fmt.Errorf("%w: %s v%d", application.ErrUnknownCapability, key, version)
	}
	if err != nil {
		return domain.Contract{}, err
	}
	needs, err := paradigms.CalibrationRequirement(key, version, nil)
	if err != nil {
		return domain.Contract{}, err
	}
	c := domain.Contract{
		MetricEngineVersion: paradigms.MetricEngineVersion, ResultSchemaVersion: paradigms.ResultSchemaVersion,
		CalibrationRequired: needs.Required, MissingReasons: paradigms.MissingReasons(),
	}
	for _, m := range p.Metrics {
		c.Metrics = append(c.Metrics, domain.MetricSpec{Key: m.Key, Unit: string(m.Unit), Integer: m.Integer, Min: m.Min, Max: m.Max})
	}
	for _, e := range p.Events {
		c.Events = append(c.Events, domain.EventSpec{Type: e.Type, Kind: string(e.Kind)})
	}
	return c, nil
}
