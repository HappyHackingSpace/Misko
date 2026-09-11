// Package catalog reads calibration needs from the hardcoded paradigm catalog.
package catalog

import (
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

func (Catalog) Requirement(key string, version int, apparatus map[string]float64) (domain.Requirement, error) {
	needs, err := paradigms.CalibrationRequirement(key, version, apparatus)
	if err != nil {
		return domain.Requirement{}, fmt.Errorf("calibration requirement of %s v%d: %w", key, version, err)
	}
	req := domain.Requirement{Required: needs.Required, ToleranceCm: needs.ToleranceCm}
	if needs.Bounds != nil {
		req.Bounds = &domain.Bounds{MinX: needs.Bounds.MinX, MinY: needs.Bounds.MinY, MaxX: needs.Bounds.MaxX, MaxY: needs.Bounds.MaxY}
	}
	return req, nil
}
