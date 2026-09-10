package domain

import (
	"fmt"
	"math"
)

func morrisWaterMazeV1() definition {
	trajectory := []string{"samples", "tank_diameter_cm", "max_sample_gap_s"}
	platform := append(trajectory, "platform_x_cm", "platform_y_cm", "platform_diameter_cm")
	const frame = "Coordinates are centimeters from the tank center, x east and y north; samples outside the tank count as lost. "
	return definition{
		paradigm: Paradigm{
			Key: "MWM", Name: "Morris water maze", Category: LearningMemory, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"ACQUISITION", "PROBE"},
			ApparatusParameters: []Parameter{
				param("tank_diameter_cm", "Tank diameter", Centimeter, 60, 250, 120),
				param("platform_diameter_cm", "Platform diameter", Centimeter, 4, 20, 10),
				param("platform_x_cm", "Platform center east of tank center", Centimeter, -125, 125, 30),
				param("platform_y_cm", "Platform center north of tank center", Centimeter, -125, 125, 30),
				param("wall_annulus_width_cm", "Wall annulus width for thigmotaxis", Centimeter, 4, 30, 12),
			},
			SessionParameters: []Parameter{
				gapParameter,
				param("min_platform_dwell_s", "Time the subject must stay on the platform to escape", Second, 0, 10, 1),
			},
			Zones: []Zone{
				{Key: "platform", Role: "TARGET", Geometry: "Closed disk of platform_diameter_cm at the platform center.", Required: true},
				{Key: "target_quadrant", Role: "TARGET", Geometry: "Quadrant containing the platform center; x >= 0 is east and y >= 0 is north.", Required: true},
				{Key: "wall_annulus", Role: "PERIPHERY", Geometry: "Ring with distance from the tank center >= tank radius - wall_annulus_width_cm.", Required: true},
			},
			Events: append(occupancyEvents("platform"), entryEvents("platform")...),
			Metrics: []MetricDefinition{
				metric("duration_s", "Analyzed duration", Second, 86400, trajectory, frame+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("escape_latency_s", "Escape latency", Second, 86400, append(platform, "min_platform_dwell_s"),
					"Start of the first platform dwell: consecutive tracked samples on the platform, joined by gaps no longer than max_sample_gap_s, spanning at least min_platform_dwell_s.",
					"first dwell start with last - first sample time >= min_platform_dwell_s", "Missing (EVENT_NOT_OBSERVED) when no dwell qualifies, as in probe trials."),
				metric("path_length_cm", "Path length", Centimeter, 1e6, append(platform, "min_platform_dwell_s"),
					"Distance over counted intervals ending at or before the escape, or over the whole trial without an escape.", "sum(hypot(dx, dy) for counted intervals ending <= escape)", missingNoPairs),
				metric("mean_swim_speed_cm_s", "Mean swim speed", CentimeterPerSecond, 1e4, []string{"path_length_cm"},
					"Path length divided by the counted time it covers.", "path_length_cm / counted time up to escape", "Missing (NO_VALID_INTERVALS) when no counted time precedes the escape."),
				metric("target_quadrant_time_ratio", "Target quadrant time ratio", Ratio, 1, platform, "Share of analyzed duration whose earlier sample is in the target quadrant.", "target quadrant time / duration_s", missingNoPairs),
				metric("thigmotaxis_time_ratio", "Thigmotaxis time ratio", Ratio, 1, append(trajectory, "wall_annulus_width_cm"), "Share of analyzed duration whose earlier sample is in the wall annulus.", "wall annulus time / duration_s", missingNoPairs),
				metric("platform_crossings_count", "Platform crossings", Count, 1e6, platform, "Counted intervals entering the platform disk; starting on the platform is not a crossing.", "count(platform entries)", missingNoPairs),
				metric("mean_distance_to_platform_cm", "Mean distance to platform", Centimeter, 1e4, platform, "Time-weighted mean distance from the platform center, using the earlier sample of each counted interval.", "sum(distance(earlier sample, platform) x dt) / sum(dt)", missingNoPairs),
			},
			QC: trajectoryQC,
		},
		validate: func(p map[string]float64) error {
			radius := p["tank_diameter_cm"] / 2
			if math.Hypot(p["platform_x_cm"], p["platform_y_cm"])+p["platform_diameter_cm"]/2 > radius || p["wall_annulus_width_cm"] >= radius {
				return fmt.Errorf("%w: the platform must lie inside the tank and the annulus must be narrower than the tank radius", ErrParameterOutOfRange)
			}
			return nil
		},
		evaluate: evaluateMorrisWaterMaze,
	}
}

func evaluateMorrisWaterMaze(e evaluation) computation {
	p := e.params
	radius, px, py := p["tank_diameter_cm"]/2, p["platform_x_cm"], p["platform_y_cm"]
	platformRadius, annulus := p["platform_diameter_cm"]/2, p["wall_annulus_width_cm"]
	maxGap, dwellUs := gapUs(p), int64(math.Round(p["min_platform_dwell_s"]*1e6))
	tracked := func(s Sample) bool { return s.Valid && math.Hypot(s.X, s.Y) <= radius }
	onPlatform := func(x, y float64) bool { return math.Hypot(x-px, y-py) <= platformRadius }
	east, north := px >= 0, py >= 0

	escapeUs, dwellStart := int64(-1), int64(-1)
	for i, s := range e.samples {
		if !tracked(s) || !onPlatform(s.X, s.Y) {
			dwellStart = -1
			continue
		}
		if dwellStart < 0 || s.TUs-e.samples[i-1].TUs > maxGap {
			dwellStart = s.TUs
		}
		if s.TUs-dwellStart >= dwellUs {
			escapeUs = dwellStart
			break
		}
	}

	var pathCm, weightedDistance float64
	var pathUs int64
	zones := []zone{
		{key: "platform", contains: onPlatform},
		{key: "target_quadrant", contains: func(x, y float64) bool { return (x >= 0) == east && (y >= 0) == north }},
		{key: "wall_annulus", contains: func(x, y float64) bool { return math.Hypot(x, y) >= radius-annulus }},
	}
	t := walk(e.samples, tracked, zones, maxGap, func(a, b Sample, dt int64) {
		if escapeUs < 0 || b.TUs <= escapeUs {
			pathCm += math.Hypot(b.X-a.X, b.Y-a.Y)
			pathUs += dt
		}
		weightedDistance += math.Hypot(a.X-px, a.Y-py) * float64(dt)
	})
	if t.durationUs == 0 {
		return computation{reason: ReasonNoValidIntervals}
	}
	c := newComputation()
	duration := float64(t.durationUs)
	c.values["duration_s"] = seconds(t.durationUs)
	c.values["path_length_cm"] = pathCm
	c.values["target_quadrant_time_ratio"] = float64(t.zoneUs["target_quadrant"]) / duration
	c.values["thigmotaxis_time_ratio"] = float64(t.zoneUs["wall_annulus"]) / duration
	c.values["platform_crossings_count"] = float64(t.entries["platform"])
	c.values["mean_distance_to_platform_cm"] = weightedDistance / duration
	if pathUs > 0 {
		c.values["mean_swim_speed_cm_s"] = pathCm / seconds(pathUs)
	} else {
		c.missing["mean_swim_speed_cm_s"] = ReasonNoValidIntervals
	}
	if escapeUs >= 0 {
		c.values["escape_latency_s"] = seconds(escapeUs)
	} else {
		c.missing["escape_latency_s"] = ReasonNotObserved
	}
	c.intervals, c.points = t.events([]string{"platform"}, []string{"platform"})
	return c
}
