// Package catalog reads calibration needs from the hardcoded paradigm catalog,
// the same source internal/calibration/adapters/catalog reads, so the
// dashboard's count of recordings waiting for calibration never disagrees
// with the per-recording calibration status.
package catalog

import (
	"fmt"

	"github.com/HappyHackingSpace/Misko/backend/internal/dashboard/application"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

func (Catalog) CalibrationRequired(key string, version int, apparatus map[string]float64) (bool, error) {
	needs, err := paradigms.CalibrationRequirement(key, version, apparatus)
	if err != nil {
		return false, fmt.Errorf("calibration requirement of %s v%d: %w", key, version, err)
	}
	return needs.Required, nil
}

var _ application.Catalog = Catalog{}
