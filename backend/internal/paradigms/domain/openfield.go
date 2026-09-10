package domain

import "math"

// Shared wording for trajectory-based metric definitions.
const (
	arenaFrame     = "Samples are arena-local centimeters with the origin at one arena corner; x in [0, width], y in [0, height]. Samples outside the arena count as lost."
	countedPair    = "A counted interval is a pair of consecutive samples that are both tracked and inside the arena, with 0 < dt <= max_sample_gap_s."
	missingNoPairs = "Missing (NO_VALID_INTERVALS) when there is no counted interval."
)

func openFieldV1() definition {
	trajectory := []string{"samples: t (us), x (cm), y (cm), tracked", "arena_width_cm", "arena_height_cm", "max_sample_gap_s"}
	zones := append(trajectory, "center_fraction")
	return definition{
		paradigm: Paradigm{
			Key: "OPEN_FIELD", Name: "Open field", Category: Anxiety, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"STANDARD"},
			ApparatusParameters: []Parameter{
				{Key: "arena_width_cm", Label: "Arena width", Unit: Centimeter, Min: 20, Max: 150, Default: 50},
				{Key: "arena_height_cm", Label: "Arena depth", Unit: Centimeter, Min: 20, Max: 150, Default: 50},
				{Key: "center_fraction", Label: "Center zone side as a fraction of the arena side", Unit: Ratio, Min: 0.2, Max: 0.8, Default: 0.5},
			},
			SessionParameters: []Parameter{
				{Key: "max_sample_gap_s", Label: "Longest gap between samples that still counts as tracked", Unit: Second, Min: 0.01, Max: 5, Default: 0.5},
				{Key: "immobility_threshold_cm_s", Label: "Speed below which an interval is immobile", Unit: CentimeterPerSecond, Min: 0.1, Max: 10, Default: 2},
				{Key: "min_immobility_bout_s", Label: "Shortest immobile bout that counts", Unit: Second, Min: 0, Max: 10, Default: 1},
			},
			Zones: []Zone{
				{Key: "center", Role: "CENTER", Geometry: "Axis-aligned rectangle centered in the arena; each side is center_fraction of the arena side. The boundary belongs to the center."},
				{Key: "periphery", Role: "PERIPHERY", Geometry: "Arena area outside the center zone."},
			},
			Events: []EventDefinition{
				{Type: "in_center", Kind: IntervalEvent, Definition: "Contiguous counted intervals whose earlier sample is in the center."},
				{Type: "immobile", Kind: IntervalEvent, Definition: "Contiguous counted intervals with speed below immobility_threshold_cm_s, lasting at least min_immobility_bout_s."},
				{Type: "center_entry", Kind: PointEvent, Definition: "Time of the later sample of a counted interval that moves from periphery to center."},
			},
			Metrics: []MetricDefinition{
				{Key: "duration_s", Label: "Analyzed duration", Unit: Second, Min: 0, Max: 86400, Tolerance: 1e-6, Inputs: trajectory, MissingWhen: missingNoPairs,
					Definition: "Time covered by counted intervals. " + arenaFrame + " " + countedPair, Formula: "sum(dt over counted intervals)"},
				{Key: "distance_cm", Label: "Distance traveled", Unit: Centimeter, Min: 0, Max: 1e6, Tolerance: 1e-6, Inputs: trajectory, MissingWhen: missingNoPairs,
					Definition: "Path length over counted intervals; lost frames are not interpolated.", Formula: "sum(hypot(dx, dy) over counted intervals)"},
				{Key: "mean_speed_cm_s", Label: "Mean speed", Unit: CentimeterPerSecond, Min: 0, Max: 1e4, Tolerance: 1e-6, Inputs: []string{"distance_cm", "duration_s"}, MissingWhen: missingNoPairs,
					Definition: "Distance traveled divided by analyzed duration.", Formula: "distance_cm / duration_s"},
				{Key: "center_time_s", Label: "Time in center", Unit: Second, Min: 0, Max: 86400, Tolerance: 1e-6, Inputs: zones, MissingWhen: missingNoPairs,
					Definition: "Counted interval time whose earlier sample is in the center zone.", Formula: "sum(dt where earlier sample in center)"},
				{Key: "periphery_time_s", Label: "Time in periphery", Unit: Second, Min: 0, Max: 86400, Tolerance: 1e-6, Inputs: zones, MissingWhen: missingNoPairs,
					Definition: "Counted interval time whose earlier sample is outside the center zone.", Formula: "sum(dt where earlier sample in periphery)"},
				{Key: "center_time_ratio", Label: "Center time ratio", Unit: Ratio, Min: 0, Max: 1, Tolerance: 1e-9, Inputs: []string{"center_time_s", "duration_s"}, MissingWhen: missingNoPairs,
					Definition: "Share of analyzed duration spent in the center.", Formula: "center_time_s / duration_s"},
				{Key: "periphery_time_ratio", Label: "Periphery time ratio", Unit: Ratio, Min: 0, Max: 1, Tolerance: 1e-9, Inputs: []string{"periphery_time_s", "duration_s"}, MissingWhen: missingNoPairs,
					Definition: "Share of analyzed duration spent in the periphery.", Formula: "periphery_time_s / duration_s"},
				{Key: "center_entries_count", Label: "Center entries", Unit: Count, Integer: true, Min: 0, Max: 1e6, Tolerance: 1e-9, Inputs: zones, MissingWhen: missingNoPairs,
					Definition: "Counted intervals moving from periphery to center. A trajectory starting in the center does not count as an entry; no debounce is applied in version 1.",
					Formula:    "count(counted intervals with earlier sample in periphery and later sample in center)"},
				{Key: "immobility_s", Label: "Immobility time", Unit: Second, Min: 0, Max: 86400, Tolerance: 1e-6,
					Inputs: append(trajectory, "immobility_threshold_cm_s", "min_immobility_bout_s"), MissingWhen: missingNoPairs,
					Definition: "Time in immobile bouts. An interval is immobile when hypot(dx, dy) / dt is strictly below the threshold; contiguous immobile intervals form a bout, and bouts shorter than min_immobility_bout_s are ignored.",
					Formula:    "sum(bout duration where bout duration >= min_immobility_bout_s)"},
			},
			QC: []QCRule{
				{Key: "min_tracking_confidence", Operator: ">=", Value: 0.7, Unit: Ratio},
				{Key: "max_lost_frame_ratio", Operator: "<=", Value: 0.1, Unit: Ratio},
				{Key: "max_calibration_error_cm", Operator: "<=", Value: 2, Unit: Centimeter},
			},
		},
		evaluate: func(e evaluation) computation { return evaluateOpenField(e.params, e.samples) },
	}
}

func evaluateOpenField(p map[string]float64, samples []Sample) computation {
	width, height := p["arena_width_cm"], p["arena_height_cm"]
	centerW, centerH := width*p["center_fraction"], height*p["center_fraction"]
	left, top := (width-centerW)/2, (height-centerH)/2
	maxGapUs := int64(math.Round(p["max_sample_gap_s"] * 1e6))
	minBoutUs := int64(math.Round(p["min_immobility_bout_s"] * 1e6))
	threshold := p["immobility_threshold_cm_s"]

	tracked := func(s Sample) bool { return s.Valid && s.X >= 0 && s.X <= width && s.Y >= 0 && s.Y <= height }
	inCenter := func(s Sample) bool { return s.X >= left && s.X <= left+centerW && s.Y >= top && s.Y <= top+centerH }

	var c computation
	var durationUs, centerUs, peripheryUs, immobileUs int64
	var distance float64
	var entries int
	var center, bout *Interval
	closeCenter := func() {
		if center != nil {
			c.intervals = append(c.intervals, *center)
			center = nil
		}
	}
	closeBout := func() {
		if bout != nil && bout.EndUs-bout.StartUs >= minBoutUs {
			immobileUs += bout.EndUs - bout.StartUs
			c.intervals = append(c.intervals, *bout)
		}
		bout = nil
	}
	extend := func(run **Interval, kind string, from, to int64) {
		if *run == nil || (*run).EndUs != from {
			*run = &Interval{Type: kind, StartUs: from}
		}
		(*run).EndUs = to
	}

	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1], samples[i]
		dt := b.TUs - a.TUs
		if !tracked(a) || !tracked(b) || dt > maxGapUs {
			closeCenter()
			closeBout()
			continue
		}
		step := math.Hypot(b.X-a.X, b.Y-a.Y)
		durationUs += dt
		distance += step
		if inCenter(a) {
			centerUs += dt
			extend(&center, "in_center", a.TUs, b.TUs)
		} else {
			peripheryUs += dt
			closeCenter()
			if inCenter(b) {
				entries++
				c.points = append(c.points, Point{Type: "center_entry", AtUs: b.TUs})
			}
		}
		if step/(float64(dt)/1e6) < threshold {
			extend(&bout, "immobile", a.TUs, b.TUs)
		} else {
			closeBout()
		}
	}
	closeCenter()
	closeBout()

	if durationUs == 0 {
		return computation{reason: ReasonNoValidIntervals}
	}
	seconds := float64(durationUs) / 1e6
	c.values = map[string]float64{
		"duration_s":           seconds,
		"distance_cm":          distance,
		"mean_speed_cm_s":      distance / seconds,
		"center_time_s":        float64(centerUs) / 1e6,
		"periphery_time_s":     float64(peripheryUs) / 1e6,
		"center_time_ratio":    float64(centerUs) / float64(durationUs),
		"periphery_time_ratio": float64(peripheryUs) / float64(durationUs),
		"center_entries_count": float64(entries),
		"immobility_s":         float64(immobileUs) / 1e6,
	}
	return c
}
