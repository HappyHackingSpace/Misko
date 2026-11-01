// Package catalog reads what a paradigm version requires before an animal
// can be analysed from the hardcoded paradigm catalog.
package catalog

import (
	"fmt"
	"strconv"

	paradigms "github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/domain"
)

type Catalog struct{}

func New() Catalog { return Catalog{} }

// Requirement tells whether the version needs a calibration and which
// calibrated zones it must carry.
func (Catalog) Requirement(key string, version int) (domain.Requirement, error) {
	p, err := paradigms.Version(key, version)
	if err != nil {
		return domain.Requirement{}, fmt.Errorf("paradigm %s v%d: %w", key, version, err)
	}
	needs, err := paradigms.CalibrationRequirement(key, version, nil)
	if err != nil {
		return domain.Requirement{}, fmt.Errorf("calibration requirement of %s v%d: %w", key, version, err)
	}
	req := domain.Requirement{CalibrationRequired: needs.Required}
	for _, z := range p.Zones {
		if needs.Required && z.Calibrated && z.Required {
			req.RequiredZones = append(req.RequiredZones, z.Key)
		}
	}
	return req, nil
}

// Calibrated lists every published paradigm version that needs a calibration.
func (Catalog) Calibrated() []string {
	var out []string
	for _, key := range paradigms.Keys() {
		versions, err := paradigms.Versions(key)
		if err != nil {
			continue
		}
		for _, v := range versions {
			if needs, err := paradigms.CalibrationRequirement(key, v, nil); err == nil && needs.Required {
				out = append(out, key+"@"+strconv.Itoa(v))
			}
		}
	}
	return out
}
