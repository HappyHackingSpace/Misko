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
	// The side views of the pole and the treadmill have no fixed extent on
	// both axes, so their calibration chooses its own frame.
	switch key {
	case "MWM":
		r := apparatus["tank_diameter_cm"] / 2
		needs.Bounds = &Bounds{-r, -r, r, r}
	case "NOVEL_OBJECT":
		needs.Bounds = &Bounds{0, 0, apparatus["arena_width_cm"], apparatus["arena_height_cm"]}
	}
	return needs, nil
}
