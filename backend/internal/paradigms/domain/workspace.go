package domain

// Bounds is an axis-aligned rectangle in the paradigm's centimeter coordinates.
type Bounds struct {
	MinX, MinY, MaxX, MaxY float64
}

// CalibrationNeeds tells whether videos of a paradigm need a pixel-to-centimeter
// calibration, the largest accepted calibration error and, when zones come
// from parameters, the area calibration points must lie in.
type CalibrationNeeds struct {
	Required    bool
	ToleranceCm float64
	// Bounds is nil when the paradigm uses calibrated zones in a frame the
	// calibration itself chooses.
	Bounds *Bounds
}

// CalibrationRequirement derives calibration needs from the version's
// max_calibration_error_cm QC rule and its apparatus parameters.
func CalibrationRequirement(key string, version int, apparatus map[string]float64) (CalibrationNeeds, error) {
	d, err := lookup(key, version)
	if err != nil {
		return CalibrationNeeds{}, err
	}
	var needs CalibrationNeeds
	for _, rule := range d.paradigm.QC {
		if rule.Key == "max_calibration_error_cm" {
			needs.Required, needs.ToleranceCm = true, rule.Value
		}
	}
	if !needs.Required {
		return CalibrationNeeds{}, nil
	}
	switch key {
	case "OPEN_FIELD":
		needs.Bounds = &Bounds{0, 0, apparatus["arena_width_cm"], apparatus["arena_height_cm"]}
	case "LIGHT_DARK":
		needs.Bounds = &Bounds{0, 0, apparatus["box_width_cm"], apparatus["box_height_cm"]}
	case "MWM":
		r := apparatus["tank_diameter_cm"] / 2
		needs.Bounds = &Bounds{-r, -r, r, r}
	}
	return needs, nil
}
