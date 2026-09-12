package domain

import (
	"fmt"
	"strings"
)

func calibratedZone(key, role, geometry string, required bool) Zone {
	return Zone{Key: key, Role: role, Geometry: geometry, Calibrated: true, Required: required}
}

func elevatedPlusMazeV1() definition {
	trajectory := []string{"samples", "max_sample_gap_s"}
	zoned := append(trajectory, "calibrated zones")
	return definition{
		paradigm: Paradigm{
			Key: "EPM", Name: "Elevated plus maze", Category: Anxiety, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"STANDARD"},
			ApparatusParameters: []Parameter{
				param("arm_length_cm", "Arm length, used to review calibrated zones", Centimeter, 20, 80, 35),
				param("arm_width_cm", "Arm width, used to review calibrated zones", Centimeter, 3, 15, 6),
			},
			SessionParameters: []Parameter{gapParameter},
			Zones: []Zone{
				calibratedZone("center", "CENTER", "Central square where the arms meet.", true),
				calibratedZone("open_arms", "OPEN", "Both open arms, one shape per arm.", true),
				calibratedZone("closed_arms", "CLOSED", "Both closed arms, one shape per arm.", true),
			},
			InputEvents: []EventDefinition{{Type: "risk_assessment", Kind: PointEvent, Definition: "A scored stretched-attend or head-dip posture."}},
			Events: append(append(occupancyEvents("center", "open_arms", "closed_arms"), entryEvents("center", "open_arms", "closed_arms")...),
				EventDefinition{Type: "risk_assessment", Kind: PointEvent, Definition: "Scored risk assessment, passed through."}),
			Metrics: []MetricDefinition{
				metric("duration_s", "Analyzed duration", Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("distance_cm", "Distance traveled", Centimeter, 1e6, trajectory, "Path length over counted intervals.", "sum(hypot(dx, dy))", missingNoPairs),
				metric("center_time_s", "Time in center", Second, 86400, zoned, "Counted time whose earlier sample is in the center.", "sum(dt where earlier sample in center)", missingNoPairs),
				metric("open_arm_time_s", "Time in open arms", Second, 86400, zoned, "Counted time whose earlier sample is in an open arm.", "sum(dt where earlier sample in open arms)", missingNoPairs),
				metric("closed_arm_time_s", "Time in closed arms", Second, 86400, zoned, "Counted time whose earlier sample is in a closed arm.", "sum(dt where earlier sample in closed arms)", missingNoPairs),
				metric("open_arm_time_ratio", "Open arm time ratio", Ratio, 1, zoned, "Share of analyzed duration in open arms.", "open_arm_time_s / duration_s", missingNoPairs),
				metric("closed_arm_time_ratio", "Closed arm time ratio", Ratio, 1, zoned, "Share of analyzed duration in closed arms.", "closed_arm_time_s / duration_s", missingNoPairs),
				metric("open_arm_entries_count", "Open arm entries", Count, 1e6, zoned, entryRule, "count(open arm entries)", missingNoPairs),
				metric("closed_arm_entries_count", "Closed arm entries", Count, 1e6, zoned, entryRule, "count(closed arm entries)", missingNoPairs),
				metric("latency_to_open_arm_s", "Latency to open arm", Second, 86400, zoned, latencyRule, "first tracked sample time in open arms", missingNotSeen),
				metric("risk_assessment_count", "Risk assessments", Count, 1e6, []string{"risk_assessment events"}, "Scored risk assessment events.", "count(risk_assessment)", "Missing (NOT_SCORED) when risk assessment was not scored."),
			},
			QC: trajectoryQC,
		},
		evaluate: func(e evaluation) computation {
			keys := []string{"center", "open_arms", "closed_arms"}
			t := walk(e.samples, validOnly, zonesFor(e, keys...), gapUs(e.params), nil)
			c := newComputation()
			if e.scored["risk_assessment"] {
				c.values["risk_assessment_count"] = float64(len(echoPoints(e.events, "risk_assessment")))
			} else {
				c.missing["risk_assessment_count"] = ReasonNotScored
			}
			c.points = echoPoints(e.events, "risk_assessment")
			if t.durationUs == 0 {
				c.reason = ReasonNoValidIntervals
				return c
			}
			duration := float64(t.durationUs)
			c.values["duration_s"] = seconds(t.durationUs)
			c.values["distance_cm"] = t.distance
			c.values["center_time_s"] = seconds(t.zoneUs["center"])
			c.values["open_arm_time_s"] = seconds(t.zoneUs["open_arms"])
			c.values["closed_arm_time_s"] = seconds(t.zoneUs["closed_arms"])
			c.values["open_arm_time_ratio"] = float64(t.zoneUs["open_arms"]) / duration
			c.values["closed_arm_time_ratio"] = float64(t.zoneUs["closed_arms"]) / duration
			c.values["open_arm_entries_count"] = float64(t.entries["open_arms"])
			c.values["closed_arm_entries_count"] = float64(t.entries["closed_arms"])
			latency(c, t, "latency_to_open_arm_s", "open_arms")
			intervals, points := t.events(keys, keys)
			c.intervals, c.points = intervals, append(c.points, points...)
			return c
		},
	}
}

func yMazeV1() definition {
	trajectory := []string{"samples", "max_sample_gap_s"}
	arms := append(trajectory, "calibrated arm zones")
	return definition{
		paradigm: Paradigm{
			Key: "Y_MAZE", Name: "Y maze", Category: LearningMemory, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"SPONTANEOUS_ALTERNATION", "NOVEL_ARM"},
			ApparatusParameters: []Parameter{param("arm_length_cm", "Arm length, used to review calibrated zones", Centimeter, 20, 60, 35)},
			SessionParameters: []Parameter{
				gapParameter,
				integerParam("novel_arm", "Novel arm in a two-trial protocol: 0 none, 1 arm_a, 2 arm_b, 3 arm_c", Count, 0, 3, 0),
			},
			Zones: []Zone{
				calibratedZone("arm_a", "ARM", "Arm A.", true),
				calibratedZone("arm_b", "ARM", "Arm B.", true),
				calibratedZone("arm_c", "ARM", "Arm C.", true),
			},
			Events: append(occupancyEvents("arm_a", "arm_b", "arm_c"), entryEvents("arm_a", "arm_b", "arm_c")...),
			Metrics: []MetricDefinition{
				metric("duration_s", "Analyzed duration", Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("distance_cm", "Distance traveled", Centimeter, 1e6, trajectory, "Path length over counted intervals.", "sum(hypot(dx, dy))", missingNoPairs),
				metric("total_arm_entries_count", "Total arm entries", Count, 1e6, arms,
					"The arm holding the first tracked sample counts as the first entry; each later arm entry follows the entry rule, and re-entering the same arm counts again. "+entryRule,
					"1 (start arm) + count(arm entries)", missingNoPairs),
				metric("spontaneous_alternation_ratio", "Spontaneous alternation ratio", Ratio, 1, arms,
					"Share of overlapping entry triplets that visit three different arms.", "alternations / (total_arm_entries_count - 2)", "Missing (INSUFFICIENT_ENTRIES) with fewer than 3 entries; missing (NO_VALID_INTERVALS) without counted intervals."),
				metric("novel_arm_time_ratio", "Novel arm time ratio", Ratio, 1, append(arms, "novel_arm"),
					"Share of analyzed duration whose earlier sample is in the novel arm.", "novel arm time / duration_s", "Missing (NOT_APPLICABLE) when novel_arm is 0; missing (NO_VALID_INTERVALS) without counted intervals."),
			},
			QC: trajectoryQC,
		},
		evaluate: func(e evaluation) computation {
			keys := []string{"arm_a", "arm_b", "arm_c"}
			zones := zonesFor(e, keys...)
			t := walk(e.samples, validOnly, zones, gapUs(e.params), nil)
			if t.durationUs == 0 {
				return computation{reason: ReasonNoValidIntervals}
			}
			c := newComputation()
			var sequence []string
			for _, s := range e.samples {
				if !s.Valid {
					continue
				}
				for _, z := range zones {
					if z.contains(s.X, s.Y) {
						sequence = append(sequence, z.key)
						break
					}
				}
				break
			}
			for _, entry := range t.sequence {
				sequence = append(sequence, entry.key)
			}
			alternations := 0
			for i := 2; i < len(sequence); i++ {
				if sequence[i] != sequence[i-1] && sequence[i] != sequence[i-2] && sequence[i-1] != sequence[i-2] {
					alternations++
				}
			}
			c.values["duration_s"] = seconds(t.durationUs)
			c.values["distance_cm"] = t.distance
			c.values["total_arm_entries_count"] = float64(len(sequence))
			if len(sequence) >= 3 {
				c.values["spontaneous_alternation_ratio"] = float64(alternations) / float64(len(sequence)-2)
			} else {
				c.missing["spontaneous_alternation_ratio"] = ReasonInsufficientEntries
			}
			if novel := int(e.params["novel_arm"]); novel > 0 {
				c.values["novel_arm_time_ratio"] = float64(t.zoneUs[keys[novel-1]]) / float64(t.durationUs)
			} else {
				c.missing["novel_arm_time_ratio"] = ReasonNotApplicable
			}
			c.intervals, c.points = t.events(keys, keys)
			return c
		},
	}
}

func barnesMazeV1() definition {
	trajectory := []string{"samples", "max_sample_gap_s"}
	holes := append(trajectory, "calibrated target_hole and holes zones")
	return definition{
		paradigm: Paradigm{
			Key: "BARNES_MAZE", Name: "Barnes maze", Category: LearningMemory, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"ACQUISITION", "PROBE"},
			ApparatusParameters: []Parameter{
				param("platform_diameter_cm", "Platform diameter, used to review calibrated zones", Centimeter, 60, 150, 92),
				integerParam("hole_count", "Number of holes, used to review calibrated zones", Count, 12, 40, 20),
			},
			SessionParameters: []Parameter{gapParameter},
			Zones: []Zone{
				calibratedZone("target_hole", "TARGET", "The escape hole.", true),
				calibratedZone("holes", "ERROR", "Every other hole, one shape per hole.", true),
			},
			Events: append(append(occupancyEvents("target_hole"), entryEvents("target_hole")...),
				EventDefinition{Type: "hole_error", Kind: PointEvent, Definition: "Later sample time of a counted interval entering a non-target hole."}),
			Metrics: []MetricDefinition{
				metric("duration_s", "Analyzed duration", Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("distance_cm", "Distance traveled", Centimeter, 1e6, trajectory, "Path length over counted intervals.", "sum(hypot(dx, dy))", missingNoPairs),
				metric("primary_latency_s", "Primary latency", Second, 86400, holes, latencyRule+" The zone is target_hole.", "first tracked sample time in target_hole", missingNotSeen),
				metric("primary_errors_count", "Primary errors", Count, 1e6, holes,
					"Entries into non-target holes before the primary latency. Each hole is a separate zone, so re-entering a hole counts again. "+entryRule,
					"count(hole entries before primary_latency_s)", "Missing (EVENT_NOT_OBSERVED) when the target is never reached; missing (NO_VALID_INTERVALS) without counted intervals."),
				metric("total_errors_count", "Total errors", Count, 1e6, holes, "Entries into non-target holes over the whole trial.", "count(hole entries)", missingNoPairs),
			},
			QC: trajectoryQC,
		},
		evaluate: func(e evaluation) computation {
			zones := []zone{shapesZone("target_hole", e.zones["target_hole"])}
			for i, s := range e.zones["holes"] {
				zones = append(zones, zone{key: fmt.Sprintf("hole_%d", i+1), contains: s.contains})
			}
			t := walk(e.samples, validOnly, zones, gapUs(e.params), nil)
			if t.durationUs == 0 {
				return computation{reason: ReasonNoValidIntervals}
			}
			c := newComputation()
			c.values["duration_s"] = seconds(t.durationUs)
			c.values["distance_cm"] = t.distance
			targetUs, reached := t.firstInsideUs["target_hole"]
			total, primary := 0, 0
			for _, entry := range t.sequence {
				if !strings.HasPrefix(entry.key, "hole_") {
					continue
				}
				total++
				if reached && entry.atUs < targetUs {
					primary++
				}
				c.points = append(c.points, Point{Type: "hole_error", AtUs: entry.atUs})
			}
			c.values["total_errors_count"] = float64(total)
			if reached {
				c.values["primary_latency_s"] = seconds(targetUs)
				c.values["primary_errors_count"] = float64(primary)
			} else {
				c.missing["primary_latency_s"] = ReasonNotObserved
				c.missing["primary_errors_count"] = ReasonNotObserved
			}
			intervals, points := t.events([]string{"target_hole"}, []string{"target_hole"})
			c.intervals, c.points = intervals, append(c.points, points...)
			return c
		},
	}
}

func threeChamberV1() definition {
	trajectory := []string{"samples", "max_sample_gap_s"}
	chambers := append(trajectory, "calibrated chamber zones")
	zoneMissing := "Missing (ZONE_NOT_PROVIDED) when the interaction zone is not calibrated; missing (NO_VALID_INTERVALS) without counted intervals."
	return definition{
		paradigm: Paradigm{
			Key: "THREE_CHAMBER", Name: "Three-chamber sociability", Category: Social, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"SOCIABILITY", "SOCIAL_NOVELTY"},
			ApparatusParameters: []Parameter{
				param("chamber_width_cm", "Chamber width, used to review calibrated zones", Centimeter, 15, 40, 20),
				param("chamber_height_cm", "Chamber depth, used to review calibrated zones", Centimeter, 20, 60, 40),
			},
			SessionParameters: []Parameter{gapParameter},
			Zones: []Zone{
				calibratedZone("social_chamber", "TARGET", "Chamber with the stimulus animal.", true),
				calibratedZone("object_chamber", "CONTROL", "Chamber with the empty cage or object.", true),
				calibratedZone("center_chamber", "CENTER", "Middle chamber.", true),
				calibratedZone("social_interaction", "TARGET", "Close-interaction area around the stimulus cage.", false),
				calibratedZone("object_interaction", "CONTROL", "Close-interaction area around the object cage.", false),
			},
			Events: append(occupancyEvents("social_chamber", "object_chamber", "center_chamber", "social_interaction", "object_interaction"),
				entryEvents("social_chamber", "object_chamber", "center_chamber")...),
			Metrics: []MetricDefinition{
				metric("duration_s", "Analyzed duration", Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("social_chamber_time_s", "Time in social chamber", Second, 86400, chambers, "Counted time whose earlier sample is in the social chamber.", "sum(dt in social_chamber)", missingNoPairs),
				metric("object_chamber_time_s", "Time in object chamber", Second, 86400, chambers, "Counted time whose earlier sample is in the object chamber.", "sum(dt in object_chamber)", missingNoPairs),
				metric("center_chamber_time_s", "Time in center chamber", Second, 86400, chambers, "Counted time whose earlier sample is in the center chamber.", "sum(dt in center_chamber)", missingNoPairs),
				func() MetricDefinition {
					m := metric("sociability_index", "Sociability index", Ratio, 1, []string{"social_chamber_time_s", "object_chamber_time_s"},
						"Preference for the social chamber, from -1 (object) to +1 (social).", "(social - object) / (social + object)",
						"Missing (ZERO_DENOMINATOR) when both chamber times are 0; missing (NO_VALID_INTERVALS) without counted intervals.")
					m.Min = -1
					return m
				}(),
				metric("social_interaction_time_s", "Social interaction time", Second, 86400, append(trajectory, "social_interaction zone"), "Counted time whose earlier sample is in the social interaction zone.", "sum(dt in social_interaction)", zoneMissing),
				metric("object_interaction_time_s", "Object interaction time", Second, 86400, append(trajectory, "object_interaction zone"), "Counted time whose earlier sample is in the object interaction zone.", "sum(dt in object_interaction)", zoneMissing),
				metric("social_chamber_entries_count", "Social chamber entries", Count, 1e6, chambers, entryRule, "count(social_chamber entries)", missingNoPairs),
				metric("object_chamber_entries_count", "Object chamber entries", Count, 1e6, chambers, entryRule, "count(object_chamber entries)", missingNoPairs),
			},
			QC: trajectoryQC,
		},
		evaluate: func(e evaluation) computation {
			chambers := []string{"social_chamber", "object_chamber", "center_chamber"}
			keys := chambers
			for _, optional := range []string{"social_interaction", "object_interaction"} {
				if len(e.zones[optional]) > 0 {
					keys = append(keys, optional)
				}
			}
			t := walk(e.samples, validOnly, zonesFor(e, keys...), gapUs(e.params), nil)
			if t.durationUs == 0 {
				return computation{reason: ReasonNoValidIntervals}
			}
			c := newComputation()
			social, object := t.zoneUs["social_chamber"], t.zoneUs["object_chamber"]
			c.values["duration_s"] = seconds(t.durationUs)
			c.values["social_chamber_time_s"] = seconds(social)
			c.values["object_chamber_time_s"] = seconds(object)
			c.values["center_chamber_time_s"] = seconds(t.zoneUs["center_chamber"])
			c.values["social_chamber_entries_count"] = float64(t.entries["social_chamber"])
			c.values["object_chamber_entries_count"] = float64(t.entries["object_chamber"])
			if social+object > 0 {
				c.values["sociability_index"] = float64(social-object) / float64(social+object)
			} else {
				c.missing["sociability_index"] = ReasonZeroDenominator
			}
			for zoneKey, metricKey := range map[string]string{"social_interaction": "social_interaction_time_s", "object_interaction": "object_interaction_time_s"} {
				if len(e.zones[zoneKey]) > 0 {
					c.values[metricKey] = seconds(t.zoneUs[zoneKey])
				} else {
					c.missing[metricKey] = ReasonZoneNotProvided
				}
			}
			c.intervals, c.points = t.events(keys, chambers)
			return c
		},
	}
}

func lightDarkV1() definition {
	trajectory := []string{"samples", "box_width_cm", "box_height_cm", "light_fraction", "max_sample_gap_s"}
	return definition{
		paradigm: Paradigm{
			Key: "LIGHT_DARK", Name: "Light/dark box", Category: Anxiety, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"STANDARD"},
			ApparatusParameters: []Parameter{
				param("box_width_cm", "Box width along the light/dark axis", Centimeter, 20, 60, 40),
				param("box_height_cm", "Box depth", Centimeter, 15, 40, 20),
				param("light_fraction", "Light compartment share of the width", Ratio, 0.4, 0.6, 0.5),
			},
			SessionParameters: []Parameter{gapParameter},
			Zones: []Zone{
				{Key: "light", Role: "RISK", Geometry: "0 <= x < box_width_cm x light_fraction, within the box.", Required: true},
				{Key: "dark", Role: "CLOSED", Geometry: "box_width_cm x light_fraction <= x <= box_width_cm; the dividing line belongs to the dark side.", Required: true},
			},
			Events: append(occupancyEvents("light", "dark"), entryEvents("light", "dark")...),
			Metrics: []MetricDefinition{
				metric("duration_s", "Analyzed duration", Second, 86400, trajectory,
					"Samples are box-local centimeters with the origin at the light-side corner; samples outside the box count as lost. "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("light_time_s", "Time in light", Second, 86400, trajectory, "Counted time whose earlier sample is in the light compartment.", "sum(dt in light)", missingNoPairs),
				metric("dark_time_s", "Time in dark", Second, 86400, trajectory, "Counted time whose earlier sample is in the dark compartment.", "sum(dt in dark)", missingNoPairs),
				metric("light_time_ratio", "Light time ratio", Ratio, 1, trajectory, "Share of analyzed duration in the light compartment.", "light_time_s / duration_s", missingNoPairs),
				metric("light_entries_count", "Light entries", Count, 1e6, trajectory, "Counted intervals from dark to light.", "count(dark to light)", missingNoPairs),
				metric("transitions_count", "Transitions", Count, 1e6, trajectory, "Counted intervals whose samples are in different compartments.", "count(light to dark) + count(dark to light)", missingNoPairs),
				metric("latency_to_dark_s", "Latency to dark", Second, 86400, trajectory, latencyRule+" The zone is dark.", "first tracked sample time in dark", missingNotSeen),
			},
			QC: trajectoryQC,
		},
		evaluate: func(e evaluation) computation {
			width, height := e.params["box_width_cm"], e.params["box_height_cm"]
			split := width * e.params["light_fraction"]
			tracked := func(s Sample) bool { return s.Valid && s.X >= 0 && s.X <= width && s.Y >= 0 && s.Y <= height }
			zones := []zone{
				{key: "light", contains: func(x, _ float64) bool { return x < split }},
				{key: "dark", contains: func(x, _ float64) bool { return x >= split }},
			}
			t := walk(e.samples, tracked, zones, gapUs(e.params), nil)
			if t.durationUs == 0 {
				return computation{reason: ReasonNoValidIntervals}
			}
			c := newComputation()
			c.values["duration_s"] = seconds(t.durationUs)
			c.values["light_time_s"] = seconds(t.zoneUs["light"])
			c.values["dark_time_s"] = seconds(t.zoneUs["dark"])
			c.values["light_time_ratio"] = float64(t.zoneUs["light"]) / float64(t.durationUs)
			c.values["light_entries_count"] = float64(t.entries["light"])
			c.values["transitions_count"] = float64(t.entries["light"] + t.entries["dark"])
			latency(c, t, "latency_to_dark_s", "dark")
			c.intervals, c.points = t.events([]string{"light", "dark"}, []string{"light", "dark"})
			return c
		},
	}
}

func zonesFor(e evaluation, keys ...string) []zone {
	zones := make([]zone, 0, len(keys))
	for _, key := range keys {
		zones = append(zones, shapesZone(key, e.zones[key]))
	}
	return zones
}

func latency(c computation, t track, metricKey, zoneKey string) {
	if us, ok := t.firstInsideUs[zoneKey]; ok {
		c.values[metricKey] = seconds(us)
	} else {
		c.missing[metricKey] = ReasonNotObserved
	}
}
