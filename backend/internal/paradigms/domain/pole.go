package domain

import (
	"cmp"
	"math"
	"slices"
)

func poleTestV1() definition {
	trajectory := []string{"samples", "max_sample_gap_s", "max_trial_duration_s", "pole zone", "base zone"}
	turn := slices.Concat(trajectory, []string{"turn_descent_cm"})
	base := slices.Concat(trajectory, []string{"fall_speed_cm_s"})
	descent := slices.Concat(turn, []string{"fall_speed_cm_s"})
	frame := T("Coordinates are in centimeters in the calibration's side view of the rig: x horizontal and y the height above the base (base at y = 0, top of the pole at y = pole_length_cm). Samples after max_trial_duration_s are ignored.",
		"Koordinatlar, düzeneğin yandan görünüşündeki kalibrasyon çerçevesinde santimetre cinsindendir: x yataydır, y tabandan yüksekliktir (taban y = 0, direğin tepesi y = pole_length_cm). max_trial_duration_s sonrasındaki örnekler dikkate alınmaz.")
	onPole := T("A sample is on the pole when it lies in the pole zone and not in the base zone.",
		"Bir örnek, direk bölgesinde olup taban bölgesinde değilse direk üzerindedir.")
	notSeen := T("Missing (NO_VALID_INTERVALS) when there are no counted intervals, or (EVENT_NOT_OBSERVED) when the event did not occur by max_trial_duration_s.",
		"Sayılan aralık yoksa eksik (NO_VALID_INTERVALS), olay max_trial_duration_s içinde gerçekleşmediyse eksik (EVENT_NOT_OBSERVED).")
	turnRule := J(onPole, T("The turn completes at the first tracked sample on the pole that is at least turn_descent_cm below the highest sample on the pole so far, before the base is reached or a fall.",
		"Dönüş, tabana ulaşmadan ya da düşmeden önce, o ana kadarki en yüksek direk örneğinin en az turn_descent_cm altında olan ilk izlenmiş direk örneğinde tamamlanır."))
	fallRule := T("A fall is a pair of consecutive tracked samples, at most max_sample_gap_s apart, from on the pole to the base or off the pole, with a downward speed above fall_speed_cm_s; it is placed at the later sample.",
		"Düşme, aralarında en fazla max_sample_gap_s olan, direk üzerinden tabana ya da direk dışına giden ve aşağı doğru hızı fall_speed_cm_s değerini aşan iki ardışık izlenmiş örnektir; zamanı sonraki örneğin zamanıdır.")
	baseRule := T("The base is reached at the first tracked sample in the base zone after a sample on the pole, unless the subject fell first.",
		"Tabana ulaşma, direk üzerindeki bir örnekten sonra taban bölgesindeki ilk izlenmiş örnektir; denek önce düştüyse tabana ulaşma sayılmaz.")
	return definition{
		paradigm: Paradigm{
			Key: "POLE", Category: Motor, Version: 1,
			Name: T("Pole test", "Direk testi"),
			Description: T(
				"The animal is placed head-upward at the top of a vertical pole and must turn and descend to the base. Assesses bradykinesia and motor coordination, commonly in models of Parkinson's disease.",
				"Denek dikey bir direğin tepesine başı yukarı bakacak şekilde konur; dönüp tabana inmesi gerekir. Özellikle Parkinson modellerinde bradikineziyi ve motor koordinasyonu ölçer.",
			),
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []TrialType{trialType("STANDARD", T("Standard trial", "Standart"))},
			Parameters: []Parameter{
				param("pole_length_cm", ApparatusScope, T("Pole length", "Direk uzunluğu"),
					T("Length of the pole from the top, where the subject is placed, to the base.",
						"Deneğin konduğu tepe noktasından tabana kadar direğin uzunluğu."),
					Centimeter, 30, 100, 50),
				param("turn_descent_cm", AnalysisScope, T("Turn descent", "Dönüş için iniş mesafesi"),
					T("Net downward movement from the highest point on the pole that marks the completed turn.",
						"Dönüşün tamamlandığını gösteren, direkteki en yüksek noktadan net aşağı iniş mesafesi."),
					Centimeter, 0.5, 20, 2),
				param("fall_speed_cm_s", AnalysisScope, T("Fall speed", "Düşme hızı"),
					T("Downward speed above which leaving the pole counts as a fall rather than a descent.",
						"Direkten ayrılmanın iniş değil düşme sayıldığı aşağı doğru hız sınırı."),
					CentimeterPerSecond, 10, 1000, 80),
				gapParameter,
				param("max_trial_duration_s", SessionScope, T("Cut-off time", "Deneme süresi sınırı"),
					T("Maximum trial duration; the trial is stopped at this time.", "Deneme durdurulmadan önce sürebileceği en uzun süre."),
					Second, 30, 120, 60),
			},
			Zones: []Zone{
				calibratedZone("pole", "POLE", T("Pole", "Direk"),
					T("Polygon drawn on the side view around the pole where the subject climbs, from the base to the top.",
						"Yandan görünüşte, deneğin tırmandığı direğin çevresine tabandan tepeye çizilen çokgen.")),
				calibratedZone("base", "BASE", T("Base", "Taban"),
					T("Polygon drawn on the side view over the floor area at the foot of the pole.",
						"Yandan görünüşte, direğin dibindeki zemin alanına çizilen çokgen.")),
			},
			Events: []EventDefinition{
				{Type: "turn_complete", Kind: PointEvent, Single: true, Label: T("Turn completed", "Dönüş tamamlandı"), Definition: turnRule},
				{Type: "base_reached", Kind: PointEvent, Single: true, Label: T("Base reached", "Tabana ulaştı"), Definition: baseRule},
				{Type: "fall", Kind: PointEvent, Single: true, Label: T("Fall", "Düşme"), Definition: fallRule},
			},
			Metrics: []MetricDefinition{
				metric("t_turn_s", T("Time to turn (T-turn)", "Dönme süresi"),
					T("Time taken to turn head-downward; longer times suggest bradykinesia.",
						"Deneğin başını aşağı çevirmesi için geçen süre. Daha uzun süre yavaşlamış harekete işaret eder."),
					Second, 120, turn, J(frame, trackedPair, T("Time of the turn_complete event, measured from the trial start.", "Deneme başlangıcından itibaren turn_complete olayının zamanı.")),
					"turn_complete time", notSeen),
				metric("t_total_s", T("Total time (T-total)", "Toplam süre"),
					T("Time from the start of the trial until the subject reaches the base.", "Deneme başlangıcından deneğin tabana ulaşmasına kadar geçen süre."),
					Second, 120, base, T("Time of the base_reached event, measured from the trial start. The cut-off time is not imputed.",
						"Deneme başlangıcından itibaren base_reached olayının zamanı. Deneme süresi sınırı değeri atanmaz."),
					"base_reached time", notSeen),
				metric("descent_time_s", T("Descent time", "İniş süresi"),
					T("Time from completion of the turn to arrival at the base.", "Dönüşü tamamlamaktan tabana ulaşmaya kadar geçen süre."),
					Second, 120, descent,
					T("Time from completion of the turn to arrival at the base.", "Dönüşü tamamlamaktan tabana ulaşmaya kadar geçen süre."),
					"t_total_s - t_turn_s", T("Missing, for the same reason as the first missing event.", "İlk eksik olayın gerekçesiyle eksik.")),
				metric("descent_speed_cm_s", T("Descent speed", "İniş hızı"),
					T("Pole length divided by descent time; lower speeds suggest impaired motor function.",
						"Direk uzunluğunun iniş süresine bölümü. Daha düşük hız bozulmuş motor işleve işaret eder."),
					CentimeterPerSecond, 1e4, slices.Concat(descent, []string{"pole_length_cm"}), T("Pole length divided by descent time.", "Direk uzunluğunun iniş süresine bölümü."),
					"pole_length_cm / descent_time_s",
					T("Missing (ZERO_DENOMINATOR) when the descent time is 0; otherwise missing for the same reason as descent_time_s.",
						"İniş süresi 0 ise eksik (ZERO_DENOMINATOR); aksi halde descent_time_s ile aynı gerekçeyle eksik.")),
				metric("fall_detected", T("Fall detected", "Düşme oldu"),
					T("Whether the subject fell off the pole.", "Deneğin direkten düşüp düşmediği."),
					Boolean, 1, base, T("1 if a fall event occurred by max_trial_duration_s, otherwise 0.", "max_trial_duration_s içinde düşme olayı olduysa 1, olmadıysa 0."),
					"fall observed", T("Missing (NO_VALID_INTERVALS) when there are no counted intervals.", "Sayılan aralık yoksa eksik (NO_VALID_INTERVALS).")),
			},
			QC: trajectoryQC,
		},
		evaluate: evaluatePole,
	}
}

func evaluatePole(e evaluation) computation {
	p := e.params
	samples := censor(e.samples, microseconds(p["max_trial_duration_s"]))
	maxGap := gapUs(p)
	if walk(samples, validOnly, nil, maxGap, nil).durationUs == 0 {
		return computation{reason: ReasonNoValidIntervals}
	}
	pole, baseZone := e.zones["pole"], e.zones["base"]
	inBase := func(s Sample) bool { return inAny(baseZone, s.X, s.Y) }
	onPole := func(s Sample) bool { return inAny(pole, s.X, s.Y) && !inBase(s) }
	turnUs, baseUs, fallUs := int64(-1), int64(-1), int64(-1)
	highest, climbed := math.Inf(-1), false
	var previous *Sample
	for i := range samples {
		s := samples[i]
		if !s.Valid {
			continue
		}
		if previous != nil && s.TUs-previous.TUs <= maxGap && onPole(*previous) && (inBase(s) || !inAny(pole, s.X, s.Y)) &&
			(previous.Y-s.Y)/seconds(s.TUs-previous.TUs) > p["fall_speed_cm_s"] {
			fallUs = s.TUs
			break
		}
		if inBase(s) && climbed {
			baseUs = s.TUs
			break
		}
		if onPole(s) {
			climbed = true
			highest = math.Max(highest, s.Y)
			if turnUs < 0 && highest-s.Y >= p["turn_descent_cm"] {
				turnUs = s.TUs
			}
		}
		previous = &samples[i]
	}

	c := newComputation()
	at := func(us int64) (float64, string) {
		if us < 0 {
			return 0, ReasonNotObserved
		}
		return seconds(us), ""
	}
	set := func(key string, value float64, reason string) {
		if reason != "" {
			c.missing[key] = reason
		} else {
			c.values[key] = value
		}
	}
	turn, turnReason := at(turnUs)
	total, baseReason := at(baseUs)
	set("t_turn_s", turn, turnReason)
	set("t_total_s", total, baseReason)
	descentReason := cmp.Or(turnReason, baseReason)
	descent := seconds(baseUs - turnUs)
	set("descent_time_s", descent, descentReason)
	if descentReason == "" && descent == 0 {
		descentReason = ReasonZeroDenominator
	}
	set("descent_speed_cm_s", p["pole_length_cm"]/descent, descentReason)
	c.values["fall_detected"] = 0
	for typ, us := range map[string]int64{"turn_complete": turnUs, "base_reached": baseUs, "fall": fallUs} {
		if us >= 0 {
			c.points = append(c.points, Point{Type: typ, AtUs: us})
		}
	}
	if fallUs >= 0 {
		c.values["fall_detected"] = 1
	}
	return c
}
