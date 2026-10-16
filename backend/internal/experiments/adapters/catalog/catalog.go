// Package catalog reads the metrics of paradigm versions from the hardcoded
// paradigm catalog, so endpoints name only metrics the engine computes.
package catalog

import (
	"fmt"

	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

func (Catalog) Metrics(key string, version int) ([]string, error) {
	p, err := paradigms.Version(key, version)
	if err != nil {
		return nil, fmt.Errorf("metrics of %s v%d: %w", key, version, err)
	}
	keys := make([]string, 0, len(p.Metrics))
	for _, m := range p.Metrics {
		keys = append(keys, m.Key)
	}
	return keys, nil
}

var _ application.Catalog = Catalog{}
