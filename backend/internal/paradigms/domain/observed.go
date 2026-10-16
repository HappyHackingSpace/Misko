package domain

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

var notScoredOrIncomplete = T("Missing (NOT_SCORED) when the event was not scored, or (INCOMPLETE_RECORDING) when it was not observed and the recording is shorter than max_trial_duration_s.",
	"Olay puanlanmadıysa eksik (NOT_SCORED); olay gözlenmediyse ve kayıt max_trial_duration_s değerinden kısaysa eksik (INCOMPLETE_RECORDING).")

func poleTestV1() definition {
	events := []string{"turn_complete event", "base_reached event"}
	notSeen := T("Missing (NOT_SCORED) when the event was not scored, or (EVENT_NOT_OBSERVED) when it did not occur.",
		"Olay puanlanmadıysa eksik (NOT_SCORED), olay gerçekleşmediyse eksik (EVENT_NOT_OBSERVED).")
	return definition{
		paradigm: Paradigm{
			Key: "POLE", Category: Motor, Version: 1,
			Name: T("Pole test", "Direk testi"),
			Description: T(
				"The animal is placed head-upward at the top of a vertical pole and must turn and descend to the base. Assesses bradykinesia and motor coordination, commonly in models of Parkinson's disease.",
				"Hayvan dikey bir direğin tepesine başı yukarı bakacak şekilde konur; dönüp tabana inmesi gerekir. Özellikle Parkinson modellerinde bradikineziyi ve motor koordinasyonu ölçer.",
			),
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []TrialType{trialType("STANDARD", T("Standard trial", "Standart"))},
			Parameters: []Parameter{
				param("pole_length_cm", ApparatusScope, T("Pole length", "Direk uzunluğu"),
					T("Length of the pole from the top, where the animal is placed, to the base.",
						"Hayvanın konduğu tepe noktasından tabana kadar direğin uzunluğu."),
					Centimeter, 30, 100, 50),
				param("max_trial_duration_s", SessionScope, T("Cut-off time", "Deneme süresi sınırı"),
					T("Maximum trial duration; the trial is stopped at this time.", "Deneme durdurulmadan önce sürebileceği en uzun süre."),
					Second, 30, 120, 60),
			},
			InputEvents: []EventDefinition{
				{Type: "turn_complete", Kind: PointEvent, Single: true, Label: T("Turn completed", "Dönüş tamamlandı"),
					Definition: T("The animal has turned to face downward.", "Denek dönüp başını aşağıya çevirmiştir.")},
				{Type: "base_reached", Kind: PointEvent, Single: true, Label: T("Base reached", "Tabana ulaştı"),
					Definition: T("All four paws have reached the base.", "Tüm patiler tabana ulaşır.")},
				{Type: "fall", Kind: PointEvent, Single: true, Label: T("Fall", "Düşme"), Definition: T("The animal falls off the pole.", "Denek direkten düşer.")},
			},
			Events: []EventDefinition{
				{Type: "turn_complete", Kind: PointEvent, Label: T("Turn completed", "Dönüş tamamlandı"), Definition: T("Scored turn, passed through unchanged.", "Puanlanan dönüş, olduğu gibi aktarılır.")},
				{Type: "base_reached", Kind: PointEvent, Label: T("Base reached", "Tabana ulaştı"), Definition: T("Scored arrival at the base, passed through unchanged.", "Puanlanan tabana ulaşma, olduğu gibi aktarılır.")},
				{Type: "fall", Kind: PointEvent, Label: T("Fall", "Düşme"), Definition: T("Scored fall, passed through unchanged.", "Puanlanan düşme, olduğu gibi aktarılır.")},
			},
			Metrics: []MetricDefinition{
				metric("t_turn_s", T("Time to turn (T-turn)", "Dönme süresi"),
					T("Time taken to turn head-downward; longer times suggest bradykinesia.",
						"Hayvanın başını aşağı çevirmesi için geçen süre. Daha uzun süre yavaşlamış harekete işaret eder."),
					Second, 120, events[:1], T("Time of the turn_complete event, measured from the trial start.", "Deneme başlangıcından itibaren turn_complete olayının zamanı."),
					"turn_complete time", notSeen),
				metric("t_total_s", T("Total time (T-total)", "Toplam süre"),
					T("Time from the start of the trial until the animal reaches the base.", "Deneme başlangıcından hayvanın tabana ulaşmasına kadar geçen süre."),
					Second, 120, events[1:], T("Time of the base_reached event, measured from the trial start. The cut-off time is not imputed.",
						"Deneme başlangıcından itibaren base_reached olayının zamanı. Deneme süresi sınırı değeri atanmaz."),
					"base_reached time", notSeen),
				metric("descent_time_s", T("Descent time", "İniş süresi"),
					T("Time from completion of the turn to arrival at the base.", "Dönüşü tamamlamaktan tabana ulaşmaya kadar geçen süre."),
					Second, 120, events,
					T("Time from completion of the turn to arrival at the base.", "Dönüşü tamamlamaktan tabana ulaşmaya kadar geçen süre."),
					"t_total_s - t_turn_s", T("Missing, for the same reason as the first missing event.", "İlk eksik olayın gerekçesiyle eksik.")),
				metric("descent_speed_cm_s", T("Descent speed", "İniş hızı"),
					T("Pole length divided by descent time; lower speeds suggest impaired motor function.",
						"Direk uzunluğunun iniş süresine bölümü. Daha düşük hız bozulmuş motor işleve işaret eder."),
					CentimeterPerSecond, 1e4, append(events, "pole_length_cm"), T("Pole length divided by descent time.", "Direk uzunluğunun iniş süresine bölümü."),
					"pole_length_cm / descent_time_s",
					T("Missing (ZERO_DENOMINATOR) when the descent time is 0; otherwise missing for the same reason as descent_time_s.",
						"İniş süresi 0 ise eksik (ZERO_DENOMINATOR); aksi halde descent_time_s ile aynı gerekçeyle eksik.")),
				metric("fall_detected", T("Fall detected", "Düşme oldu"),
					T("Whether the animal fell off the pole.", "Hayvanın direkten düşüp düşmediği."),
					Boolean, 1, []string{"fall event"}, T("1 if a fall was recorded; 0 if falls were scored and none occurred.", "Düşme puanlandıysa 1, düşmeler puanlandığı halde hiç düşme olmadıysa 0."),
					"fall observed", T("Missing (NOT_SCORED) when falls were not scored.", "Düşmeler puanlanmadıysa eksik (NOT_SCORED).")),
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
			Key: "TREADMILL", Category: Motor, Version: 1,
			Name: T("Treadmill exhaustion test", "Koşu bandı testi"),
			Description: T(
				"The animal runs on a motorized treadmill at a constant or increasing speed until exhaustion. Assesses endurance and exercise capacity.",
				"Hayvan sabit ya da giderek artan hızdaki motorlu bir bant üzerinde, artık devam edemeyene kadar koşar. Dayanıklılığı ve egzersiz kapasitesini ölçer.",
			),
			Species: []string{"MOUSE", "RAT"},
			TrialTypes: []TrialType{
				trialType("ENDURANCE", T("Endurance", "Dayanıklılık")),
				trialType("FIXED_SPEED", T("Constant speed", "Sabit hız")),
			},
			Parameters: []Parameter{
				param("lane_length_cm", ApparatusScope, T("Lane length", "Kulvar uzunluğu"),
					T("Length of the running lane, from the front wall to the rear grid.",
						"Bant üzerindeki koşu kulvarının ön duvardan arka ızgaraya kadar uzunluğu."),
					Centimeter, 20, 80, 40),
				integerParam("accelerating", SessionScope, T("Accelerating protocol", "Hızlanan bant"),
					T("On: the belt accelerates from the start speed to the end speed. Off: the belt runs at the start speed throughout the trial.",
						"Açık: bant başlangıç hızından bitiş hızına doğru hızlanır. Kapalı: bant deneme boyunca başlangıç hızında çalışır."),
					Boolean, 0, 1, 1),
				param("start_speed_cm_s", SessionScope, T("Start speed", "Bant başlangıç hızı"),
					T("Belt speed at the start of the trial.", "Deneme başladığında bandın hızı."),
					CentimeterPerSecond, 1, 50, 5),
				param("end_speed_cm_s", SessionScope, T("End speed", "Bant bitiş hızı"),
					T("Belt speed reached at the end of the acceleration phase and held thereafter. Must not be lower than the start speed.",
						"Bandın hızlanma sonunda ulaştığı ve sonra koruduğu hız. Başlangıç hızından düşük olamaz."),
					CentimeterPerSecond, 5, 120, 40),
				param("acceleration_duration_s", SessionScope, T("Acceleration duration", "Hızlanma süresi"),
					T("Time over which the belt speed rises linearly from the start speed to the end speed.", "Bandın başlangıç hızından bitiş hızına çıkması için geçen süre."),
					Second, 1, 7200, 1800),
				param("max_trial_duration_s", SessionScope, T("Cut-off time", "Deneme süresi sınırı"),
					T("The trial ends at this time even if the animal is still running.", "Hayvan hâlâ koşuyor olsa bile deneme bu sürede sona erer."),
					Second, 60, 7200, 1800),
			},
			InputEvents: []EventDefinition{
				{Type: "exhaustion", Kind: PointEvent, Single: true, Label: T("Exhaustion", "Tükenme"), Definition: T("The animal meets the exhaustion criterion.", "Tükenme ölçütü karşılanır.")},
				{Type: "shock", Kind: PointEvent, Label: T("Shock", "Şok"), Definition: T("A shock or contact with the rear grid.", "Bir şok ya da arka ızgara teması.")},
			},
			Events: []EventDefinition{
				{Type: "exhaustion", Kind: PointEvent, Label: T("Exhaustion", "Tükenme"), Definition: T("Scored exhaustion, passed through unchanged.", "Puanlanan tükenme, olduğu gibi aktarılır.")},
				{Type: "shock", Kind: PointEvent, Label: T("Shock", "Şok"), Definition: T("Scored shock, passed through unchanged.", "Puanlanan şok, olduğu gibi aktarılır.")},
			},
			Metrics: []MetricDefinition{
				metric("latency_to_exhaustion_s", T("Time to exhaustion", "Tükenme latansı"),
					T("Time until the animal meets the exhaustion criterion; longer times indicate greater endurance.",
						"Hayvanın tükenme ölçütünü karşılamasına kadar geçen süre. Daha uzun latans daha yüksek dayanıklılığa işaret eder."),
					Second, 7200, run,
					T("Time of exhaustion, if it occurs by the cut-off time.", "Deneme süresi sınırına kadar gerçekleşen tükenmenin zamanı."), "exhaustion time",
					T("Missing (NOT_SCORED) when exhaustion was not scored, (EVENT_NOT_OBSERVED) when the whole trial was recorded without exhaustion, and (INCOMPLETE_RECORDING) otherwise.",
						"Puanlanmadıysa eksik (NOT_SCORED), deneme tükenme olmadan tümüyle kaydedildiyse eksik (EVENT_NOT_OBSERVED), aksi halde eksik (INCOMPLETE_RECORDING).")),
				metric("run_time_s", T("Running time", "Koşu süresi"),
					T("Time spent running, until exhaustion or the cut-off time.", "Hayvanın tükenmeye ya da deneme süresi sınırına kadar koştuğu süre."),
					Second, 7200, run,
					T("Time of exhaustion, or max_trial_duration_s when the whole trial was recorded without exhaustion.",
						"Tükenme zamanı; deneme tükenme olmadan tümüyle kaydedildiyse max_trial_duration_s."),
					"min(exhaustion time, max_trial_duration_s)", notScoredOrIncomplete),
				metric("run_distance_cm", T("Distance run", "Koşu mesafesi"),
					T("Distance covered on the treadmill during the run; longer distances indicate greater endurance.",
						"Koşu boyunca bant üzerinde kat edilen mesafe. Daha uzun mesafe daha yüksek dayanıklılığa işaret eder."),
					Centimeter, 1e7, append(run, belt...),
					T("Integral of belt speed over the running time; the speed rises linearly from the start speed to the end speed over acceleration_duration_s and then remains constant.",
						"Bant hızının koşu süresi boyunca integrali; hız acceleration_duration_s boyunca başlangıç değerinden bitiş değerine doğrusal olarak artar, sonra sabit kalır."),
					"integral of speed(t) dt over [0, run_time_s]", notScoredOrIncomplete),
				metric("shock_count", T("Shock count", "Şok sayısı"),
					T("Number of shocks or rear-grid contacts during the run; more shocks suggest fatigue or low motivation.",
						"Koşu sırasında kaç şok ya da arka ızgara teması olduğu. Daha fazla şok düşük motivasyona ya da yorgunluğa işaret eder."),
					Count, 1e6, append(run, "shock events"),
					T("Shocks occurring strictly before the end of the run.", "Koşunun bitişinden kesin olarak önce gerçekleşen şoklar."), "count(shock time < run end)",
					T("Missing (NOT_SCORED) when shocks were not scored; otherwise missing for the same reason as run_time_s.",
						"Şoklar puanlanmadıysa eksik (NOT_SCORED); aksi halde run_time_s ile aynı gerekçeyle eksik.")),
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
	notScored := T("Missing (NOT_SCORED) when exploration was not scored.", "Keşif puanlanmadıysa eksik (NOT_SCORED).")
	return definition{
		paradigm: Paradigm{
			Key: "NOVEL_OBJECT", Category: LearningMemory, Version: 1,
			Name: T("Novel object recognition test", "Yeni nesne tanıma testi"),
			Description: T(
				"After a familiarization trial with two identical objects, the animal is tested with one familiar and one novel object. Assesses recognition memory: an animal that remembers the familiar object spends more time exploring the novel one.",
				"İki özdeş nesneyi keşfettikten sonra hayvan, biri tanıdık biri yeni iki nesneyle test edilir. Tanıma belleğini ölçer: tanıdık nesneyi hatırlayan hayvan yeni nesneyi daha uzun süre keşfeder.",
			),
			Species: []string{"MOUSE", "RAT"},
			TrialTypes: []TrialType{
				trialType("FAMILIARIZATION", T("Familiarization trial", "Alıştırma")),
				trialType("TEST", T("Test trial", "Test")),
			},
			Parameters: []Parameter{
				param("arena_width_cm", ApparatusScope, T("Arena width", "Arena genişliği"),
					T("Internal width of the arena floor, measured wall to wall along the horizontal axis of the camera image.",
						"Arena zemininin iç genişliği; duvardan duvara, kamera görüntüsünün yatay ekseni boyunca."),
					Centimeter, 30, 100, 50),
				param("arena_height_cm", ApparatusScope, T("Arena depth", "Arena derinliği"),
					T("Internal depth of the arena floor, measured wall to wall along the vertical axis of the camera image.",
						"Arena zemininin iç derinliği; duvardan duvara, kamera görüntüsünün dikey ekseni boyunca."),
					Centimeter, 30, 100, 50),
				param("object_zone_radius_cm", AnalysisScope, T("Exploration radius", "Keşif mesafesi"),
					T("Sniffing directed at an object counts as exploration only within this distance of the object.",
						"Burnu nesneye yönelik koklama, yalnızca nesneye bu mesafe içindeyken keşif sayılır."),
					Centimeter, 1, 10, 3),
				param("min_total_exploration_s", AnalysisScope, T("Minimum total exploration time", "En az toplam keşif süresi"),
					T("No discrimination index is computed for animals whose combined exploration of both objects falls below this value. Set to 0 to disable the criterion.",
						"İki nesneyi toplamda bundan daha az keşfeden hayvanlar için ayırt etme indeksi hesaplanmaz. 0 bu ölçütü kapatır."),
					Second, 0, 120, 0),
			},
			InputEvents: []EventDefinition{{Type: "exploration", Kind: IntervalEvent, Labels: []string{"novel", "familiar"}, Label: T("Object exploration", "Nesne keşfi"),
				Definition: T("Nose directed toward an object within object_zone_radius_cm; climbing or sitting on the object does not count as exploration.",
					"Burun, object_zone_radius_cm mesafesi içinde nesneye yöneliktir; nesneye tırmanmak ya da üzerinde oturmak keşif sayılmaz.")}},
			Events: []EventDefinition{
				{Type: "exploration_novel", Kind: IntervalEvent, Label: T("Novel object exploration", "Yeni nesne keşfi"), Definition: T("Novel object exploration bouts, with overlapping intervals merged.", "Birleştirilmiş yeni nesne keşfi.")},
				{Type: "exploration_familiar", Kind: IntervalEvent, Label: T("Familiar object exploration", "Tanıdık nesne keşfi"), Definition: T("Familiar object exploration bouts, with overlapping intervals merged.", "Birleştirilmiş tanıdık nesne keşfi.")},
			},
			Metrics: []MetricDefinition{
				metric("novel_object_time_s", T("Novel object exploration time", "Yeni nesne keşif süresi"),
					T("Time spent exploring the novel object.", "Yeni nesneyi keşfetmekle geçen süre."),
					Second, 86400, exploration,
					T("Total length of the union of novel object exploration intervals; overlapping time is counted once.",
						"Yeni nesne keşif aralıklarının birleşiminin uzunluğu; çakışmalar bir kez sayılır."),
					"union length(novel intervals)", notScored),
				metric("familiar_object_time_s", T("Familiar object exploration time", "Tanıdık nesne keşif süresi"),
					T("Time spent exploring the familiar object.", "Tanıdık nesneyi keşfetmekle geçen süre."),
					Second, 86400, exploration,
					T("Total length of the union of familiar object exploration intervals.", "Tanıdık nesne keşif aralıklarının birleşiminin uzunluğu."),
					"union length(familiar intervals)", notScored),
				metric("total_exploration_time_s", T("Total exploration time", "Toplam keşif süresi"),
					T("Time spent exploring either object; very low values make the discrimination index unreliable.",
						"Herhangi bir nesneyi keşfetmekle geçen süre; çok düşük değerler ayırt etme indeksini güvenilmez kılar."),
					Second, 86400, exploration,
					T("Sum of the novel and familiar object exploration times.", "Yeni ve tanıdık nesne keşfinin toplamı."), "novel_object_time_s + familiar_object_time_s", notScored),
				func() MetricDefinition {
					m := metric("discrimination_index", T("Discrimination index", "Ayırt etme indeksi"),
						T("Preference for the novel object, from -1 to +1. Positive values indicate that the familiar object is remembered; values near 0 indicate no preference.",
							"Yeni nesneye tercih, -1 ile +1 arasında. Pozitif değerler tanıdık nesnenin hatırlandığına, 0 civarındaki değerler tercih olmadığına işaret eder."),
						Ratio, 1, append(exploration, "min_total_exploration_s"),
						T("Relative exploration of the novel object, from -1 (familiar object only) to +1 (novel object only).", "Tanıma belleği, -1 (tanıdık) ile +1 (yeni) arasında."),
						"(novel - familiar) / (novel + familiar)",
						T("Missing (NOT_SCORED) when exploration was not scored, (BELOW_EXPLORATION_CRITERION) when total exploration time is below min_total_exploration_s, and (ZERO_DENOMINATOR) when it is 0.",
							"Puanlanmadıysa eksik (NOT_SCORED), toplam keşif min_total_exploration_s değerinin altındaysa eksik (BELOW_EXPLORATION_CRITERION), toplam keşif 0 ise eksik (ZERO_DENOMINATOR)."))
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
