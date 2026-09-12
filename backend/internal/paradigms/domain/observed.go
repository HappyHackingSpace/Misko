package domain

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

const notScoredOrIncomplete = "Missing (NOT_SCORED) when the event was not scored; missing (INCOMPLETE_RECORDING) when it was not observed and the recording is shorter than max_trial_duration_s."

func rotarodV1() definition {
	fall := []string{"fall event", "max_trial_duration_s", "recording duration"}
	return definition{
		paradigm: Paradigm{
			Key: "ROTAROD", Name: "Rotarod", Category: Motor, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"FIXED_SPEED", "ACCELERATING"},
			ApparatusParameters: []Parameter{
				param("start_rpm", "Rod speed at trial start", Rpm, 0, 80, 4),
				param("end_rpm", "Rod speed after acceleration", Rpm, 1, 100, 40),
			},
			SessionParameters: []Parameter{
				integerParam("accelerating", "Accelerating mode: 1 accelerating, 0 fixed at start_rpm", Boolean, 0, 1, 1),
				param("acceleration_duration_s", "Time from start_rpm to end_rpm", Second, 1, 600, 300),
				param("max_trial_duration_s", "Trial cut-off", Second, 30, 600, 300),
			},
			InputEvents: []EventDefinition{{Type: "fall", Kind: PointEvent, Single: true, Definition: "The subject falls or passively rotates a full turn."}},
			Events:      []EventDefinition{{Type: "fall", Kind: PointEvent, Definition: "Scored fall, passed through."}},
			Metrics: []MetricDefinition{
				metric("latency_to_fall_s", "Latency to fall", Second, 3600, fall,
					"Fall time; without a fall by the cut-off, the latency is censored at max_trial_duration_s.", "min(fall time, max_trial_duration_s)", notScoredOrIncomplete),
				metric("fall_detected", "Fall detected", Boolean, 1, fall, "1 when a fall occurs by the cut-off, 0 when the whole trial was recorded without one.", "fall time <= max_trial_duration_s", notScoredOrIncomplete),
				metric("rpm_at_fall", "Speed at fall", Rpm, 100, append(fall, "accelerating", "start_rpm", "end_rpm", "acceleration_duration_s"),
					"Rod speed at the fall time; accelerating linearly from start_rpm to end_rpm, then constant.",
					"accelerating: start + (end - start) x min(t, acceleration_duration_s) / acceleration_duration_s; fixed: start_rpm",
					"Missing (EVENT_NOT_OBSERVED) without a fall by the cut-off; also missing when not scored or incompletely recorded."),
			},
			QC: scoringQC,
		},
		validate: func(p map[string]float64) error {
			if p["end_rpm"] < p["start_rpm"] {
				return fmt.Errorf("%w: end_rpm must not be below start_rpm", ErrParameterOutOfRange)
			}
			return nil
		},
		evaluate: func(e evaluation) computation {
			if !e.scored["fall"] {
				return computation{reason: ReasonNotScored}
			}
			p := e.params
			c := newComputation()
			c.points = echoPoints(e.events, "fall")
			maxUs := int64(math.Round(p["max_trial_duration_s"] * 1e6))
			fall, fell := firstEvent(e.events, "fall")
			switch {
			case fell && fall.StartUs <= maxUs:
				t := seconds(fall.StartUs)
				c.values["latency_to_fall_s"] = t
				c.values["fall_detected"] = 1
				rpm := p["start_rpm"]
				if p["accelerating"] == 1 {
					rpm += (p["end_rpm"] - p["start_rpm"]) * math.Min(t, p["acceleration_duration_s"]) / p["acceleration_duration_s"]
				}
				c.values["rpm_at_fall"] = rpm
			case e.durationUs >= maxUs:
				c.values["latency_to_fall_s"] = p["max_trial_duration_s"]
				c.values["fall_detected"] = 0
				c.missing["rpm_at_fall"] = ReasonNotObserved
			default:
				c.reason = ReasonIncompleteRecording
			}
			return c
		},
	}
}

func poleTestV1() definition {
	events := []string{"turn_complete event", "base_reached event"}
	notSeen := "Missing (NOT_SCORED) when the event was not scored and (EVENT_NOT_OBSERVED) when it did not occur."
	return definition{
		paradigm: Paradigm{
			Key: "POLE", Name: "Pole test", Category: Motor, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"STANDARD"},
			ApparatusParameters: []Parameter{param("pole_length_cm", "Pole length", Centimeter, 30, 100, 50)},
			SessionParameters:   []Parameter{param("max_trial_duration_s", "Trial cut-off", Second, 30, 120, 60)},
			InputEvents: []EventDefinition{
				{Type: "turn_complete", Kind: PointEvent, Single: true, Definition: "The subject has turned to face downward."},
				{Type: "base_reached", Kind: PointEvent, Single: true, Definition: "All paws reach the base."},
				{Type: "fall", Kind: PointEvent, Single: true, Definition: "The subject falls off the pole."},
			},
			Events: []EventDefinition{
				{Type: "turn_complete", Kind: PointEvent, Definition: "Scored turn, passed through."},
				{Type: "base_reached", Kind: PointEvent, Definition: "Scored arrival at the base, passed through."},
				{Type: "fall", Kind: PointEvent, Definition: "Scored fall, passed through."},
			},
			Metrics: []MetricDefinition{
				metric("t_turn_s", "Time to turn", Second, 120, events[:1], "Time of the turn_complete event from the trial start.", "turn_complete time", notSeen),
				metric("t_total_s", "Total time", Second, 120, events[1:], "Time of the base_reached event from the trial start. No cut-off value is imputed.", "base_reached time", notSeen),
				metric("descent_time_s", "Descent time", Second, 120, events, "Time from completing the turn to reaching the base.", "t_total_s - t_turn_s", "Missing with the reason of the first missing event."),
				metric("descent_speed_cm_s", "Descent speed", CentimeterPerSecond, 1e4, append(events, "pole_length_cm"), "Pole length divided by the descent time.", "pole_length_cm / descent_time_s",
					"Missing (ZERO_DENOMINATOR) when the descent time is 0; otherwise missing with the reason of descent_time_s."),
				metric("fall_detected", "Fall detected", Boolean, 1, []string{"fall event"}, "1 when a fall was scored, 0 when falls were scored and none occurred.", "fall observed", "Missing (NOT_SCORED) when falls were not scored."),
			},
			QC: scoringQC,
		},
		checkEvents: func(events []ObservedEvent) error {
			turn, turned := firstEvent(events, "turn_complete")
			base, reached := firstEvent(events, "base_reached")
			if turned && reached && turn.StartUs > base.StartUs {
				return fmt.Errorf("%w: turn_complete after base_reached", ErrInvalidEvent)
			}
			return nil
		},
		evaluate: func(e evaluation) computation {
			c := newComputation()
			c.points = echoPoints(e.events, "turn_complete", "base_reached", "fall")
			observed := func(typ string) (int64, string) {
				if !e.scored[typ] {
					return 0, ReasonNotScored
				}
				if ev, ok := firstEvent(e.events, typ); ok {
					return ev.StartUs, ""
				}
				return 0, ReasonNotObserved
			}
			turnUs, turnReason := observed("turn_complete")
			baseUs, baseReason := observed("base_reached")
			set := func(key string, value float64, reason string) {
				if reason != "" {
					c.missing[key] = reason
				} else {
					c.values[key] = value
				}
			}
			set("t_turn_s", seconds(turnUs), turnReason)
			set("t_total_s", seconds(baseUs), baseReason)
			descentReason := cmp.Or(turnReason, baseReason)
			descent := seconds(baseUs - turnUs)
			set("descent_time_s", descent, descentReason)
			if descentReason == "" && descent == 0 {
				descentReason = ReasonZeroDenominator
			}
			if descentReason == "" {
				c.values["descent_speed_cm_s"] = e.params["pole_length_cm"] / descent
			} else {
				c.missing["descent_speed_cm_s"] = descentReason
			}
			if _, reason := observed("fall"); reason == ReasonNotScored {
				c.missing["fall_detected"] = ReasonNotScored
			} else if reason == "" {
				c.values["fall_detected"] = 1
			} else {
				c.values["fall_detected"] = 0
			}
			return c
		},
	}
}

func treadmillV1() definition {
	belt := []string{"accelerating", "start_speed_cm_s", "end_speed_cm_s", "acceleration_duration_s"}
	run := []string{"exhaustion event", "max_trial_duration_s", "recording duration"}
	return definition{
		paradigm: Paradigm{
			Key: "TREADMILL", Name: "Treadmill", Category: Motor, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"ENDURANCE", "FIXED_SPEED"},
			ApparatusParameters: []Parameter{param("lane_length_cm", "Lane length", Centimeter, 20, 80, 40)},
			SessionParameters: []Parameter{
				integerParam("accelerating", "Accelerating belt: 1 accelerating, 0 fixed at start_speed_cm_s", Boolean, 0, 1, 1),
				param("start_speed_cm_s", "Belt speed at start", CentimeterPerSecond, 1, 50, 5),
				param("end_speed_cm_s", "Belt speed after acceleration", CentimeterPerSecond, 5, 120, 40),
				param("acceleration_duration_s", "Time from start to end speed", Second, 1, 7200, 1800),
				param("max_trial_duration_s", "Trial cut-off", Second, 60, 7200, 1800),
			},
			InputEvents: []EventDefinition{
				{Type: "exhaustion", Kind: PointEvent, Single: true, Definition: "The exhaustion criterion is met."},
				{Type: "shock", Kind: PointEvent, Definition: "A shock or rear-grid contact."},
			},
			Events: []EventDefinition{
				{Type: "exhaustion", Kind: PointEvent, Definition: "Scored exhaustion, passed through."},
				{Type: "shock", Kind: PointEvent, Definition: "Scored shock, passed through."},
			},
			Metrics: []MetricDefinition{
				metric("latency_to_exhaustion_s", "Latency to exhaustion", Second, 7200, run, "Exhaustion time by the cut-off.", "exhaustion time",
					"Missing (NOT_SCORED) when not scored, (EVENT_NOT_OBSERVED) when the full trial was recorded without exhaustion, (INCOMPLETE_RECORDING) otherwise."),
				metric("run_time_s", "Run time", Second, 7200, run, "Exhaustion time, or max_trial_duration_s when the full trial was recorded without exhaustion.", "min(exhaustion time, max_trial_duration_s)", notScoredOrIncomplete),
				metric("run_distance_cm", "Run distance", Centimeter, 1e7, append(run, belt...),
					"Integral of the belt speed over the run time; the speed rises linearly from start to end over acceleration_duration_s, then stays constant.",
					"integral of speed(t) dt over [0, run_time_s]", notScoredOrIncomplete),
				metric("shock_count", "Shocks", Count, 1e6, append(run, "shock events"), "Shocks strictly before the end of the run.", "count(shock time < run end)",
					"Missing (NOT_SCORED) when shocks were not scored; otherwise missing with the reason of run_time_s."),
			},
			QC: scoringQC,
		},
		validate: func(p map[string]float64) error {
			if p["end_speed_cm_s"] < p["start_speed_cm_s"] {
				return fmt.Errorf("%w: end_speed_cm_s must not be below start_speed_cm_s", ErrParameterOutOfRange)
			}
			return nil
		},
		evaluate: func(e evaluation) computation {
			p := e.params
			c := newComputation()
			c.points = echoPoints(e.events, "exhaustion", "shock")
			maxUs := int64(math.Round(p["max_trial_duration_s"] * 1e6))
			complete := e.durationUs >= maxUs
			runUs, runReason := int64(0), ""
			exhaustion, exhausted := firstEvent(e.events, "exhaustion")
			exhausted = exhausted && exhaustion.StartUs <= maxUs
			switch {
			case !e.scored["exhaustion"]:
				runReason = ReasonNotScored
				c.missing["latency_to_exhaustion_s"] = ReasonNotScored
			case exhausted:
				runUs = exhaustion.StartUs
				c.values["latency_to_exhaustion_s"] = seconds(runUs)
			case complete:
				runUs = maxUs
				c.missing["latency_to_exhaustion_s"] = ReasonNotObserved
			default:
				runReason = ReasonIncompleteRecording
				c.missing["latency_to_exhaustion_s"] = ReasonIncompleteRecording
			}
			if runReason != "" {
				c.missing["run_time_s"], c.missing["run_distance_cm"] = runReason, runReason
			} else {
				c.values["run_time_s"] = seconds(runUs)
				c.values["run_distance_cm"] = beltDistance(p, seconds(runUs))
			}
			switch {
			case !e.scored["shock"]:
				c.missing["shock_count"] = ReasonNotScored
			case runReason != "":
				c.missing["shock_count"] = runReason
			default:
				n := 0
				for _, ev := range e.events {
					if ev.Type == "shock" && ev.StartUs < runUs {
						n++
					}
				}
				c.values["shock_count"] = float64(n)
			}
			return c
		},
	}
}

// beltDistance integrates the belt speed over [0, t] seconds.
func beltDistance(p map[string]float64, t float64) float64 {
	start, end, ramp := p["start_speed_cm_s"], p["end_speed_cm_s"], p["acceleration_duration_s"]
	if p["accelerating"] != 1 {
		return start * t
	}
	if t <= ramp {
		return start*t + (end-start)*t*t/(2*ramp)
	}
	return start*ramp + (end-start)*ramp/2 + end*(t-ramp)
}

func novelObjectV1() definition {
	exploration := []string{"exploration events"}
	notScored := "Missing (NOT_SCORED) when exploration was not scored."
	return definition{
		paradigm: Paradigm{
			Key: "NOVEL_OBJECT", Name: "Novel object recognition", Category: LearningMemory, Version: 1,
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []string{"FAMILIARIZATION", "TEST"},
			ApparatusParameters: []Parameter{
				param("arena_width_cm", "Arena width", Centimeter, 30, 100, 50),
				param("arena_height_cm", "Arena depth", Centimeter, 30, 100, 50),
				param("object_zone_radius_cm", "Distance from an object within which directed sniffing counts as exploration", Centimeter, 1, 10, 3),
			},
			SessionParameters: []Parameter{param("min_total_exploration_s", "Minimum total exploration for a valid index", Second, 0, 120, 0)},
			InputEvents: []EventDefinition{{Type: "exploration", Kind: IntervalEvent, Labels: []string{"novel", "familiar"},
				Definition: "Nose directed at an object within object_zone_radius_cm; climbing or sitting on the object is not exploration."}},
			Events: []EventDefinition{
				{Type: "exploration_novel", Kind: IntervalEvent, Definition: "Merged novel-object exploration."},
				{Type: "exploration_familiar", Kind: IntervalEvent, Definition: "Merged familiar-object exploration."},
			},
			Metrics: []MetricDefinition{
				metric("novel_object_time_s", "Novel object exploration", Second, 86400, exploration, "Length of the union of novel exploration intervals; overlaps count once.", "union length(novel intervals)", notScored),
				metric("familiar_object_time_s", "Familiar object exploration", Second, 86400, exploration, "Length of the union of familiar exploration intervals.", "union length(familiar intervals)", notScored),
				metric("total_exploration_time_s", "Total exploration", Second, 86400, exploration, "Novel plus familiar exploration.", "novel_object_time_s + familiar_object_time_s", notScored),
				func() MetricDefinition {
					m := metric("discrimination_index", "Discrimination index", Ratio, 1, append(exploration, "min_total_exploration_s"),
						"Recognition memory, from -1 (familiar) to +1 (novel).", "(novel - familiar) / (novel + familiar)",
						"Missing (NOT_SCORED) when not scored, (BELOW_EXPLORATION_CRITERION) when total exploration is below min_total_exploration_s, (ZERO_DENOMINATOR) when it is 0.")
					m.Min = -1
					return m
				}(),
			},
			QC: scoringQC,
		},
		evaluate: func(e evaluation) computation {
			if !e.scored["exploration"] {
				return computation{reason: ReasonNotScored}
			}
			c := newComputation()
			total := map[string]int64{}
			for _, label := range []string{"novel", "familiar"} {
				for _, i := range merge(e.events, label) {
					total[label] += i.EndUs - i.StartUs
					c.intervals = append(c.intervals, Interval{Type: "exploration_" + label, StartUs: i.StartUs, EndUs: i.EndUs})
				}
			}
			novel, familiar := total["novel"], total["familiar"]
			sum := novel + familiar
			c.values["novel_object_time_s"] = seconds(novel)
			c.values["familiar_object_time_s"] = seconds(familiar)
			c.values["total_exploration_time_s"] = seconds(sum)
			switch {
			case seconds(sum) < e.params["min_total_exploration_s"]:
				c.missing["discrimination_index"] = ReasonBelowCriterion
			case sum == 0:
				c.missing["discrimination_index"] = ReasonZeroDenominator
			default:
				c.values["discrimination_index"] = float64(novel-familiar) / float64(sum)
			}
			return c
		},
	}
}

// merge returns the union of exploration intervals with a label; touching
// intervals are joined.
func merge(events []ObservedEvent, label string) []Interval {
	var spans []Interval
	for _, e := range events {
		if e.Type == "exploration" && e.Label == label {
			spans = append(spans, Interval{StartUs: e.StartUs, EndUs: e.EndUs})
		}
	}
	slices.SortFunc(spans, func(a, b Interval) int { return cmp.Compare(a.StartUs, b.StartUs) })
	var out []Interval
	for _, s := range spans {
		if n := len(out); n > 0 && s.StartUs <= out[n-1].EndUs {
			out[n-1].EndUs = max(out[n-1].EndUs, s.EndUs)
			continue
		}
		out = append(out, s)
	}
	return out
}
