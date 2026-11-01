package domain

import (
	"fmt"
	"slices"
)

func treadmillV1() definition {
	belt := []string{"accelerating", "start_speed_cm_s", "end_speed_cm_s", "acceleration_duration_s"}
	run := []string{"samples", "max_sample_gap_s", "shock_grid zone", "exhaustion_dwell_s", "max_trial_duration_s", "recording duration"}
	frame := T("Coordinates are in centimeters in the calibration's side view of the lane: x along the lane from the shock grid end (x = 0) to the front (x = lane_length_cm), y vertical. Samples after max_trial_duration_s are ignored.",
		"Koordinatlar, kulvarın yandan görünüşündeki kalibrasyon çerçevesinde santimetre cinsindendir: x kulvar boyunca şok ızgarası ucundan (x = 0) öne (x = lane_length_cm) doğru, y dikeydir. max_trial_duration_s sonrasındaki örnekler dikkate alınmaz.")
	stay := T("A stay on the grid is a run of contiguous counted intervals whose earlier sample lies in the shock grid zone.",
		"Izgarada kalma, önceki örneği şok ızgarası bölgesinde olan bitişik sayılan aralıklardır.")
	runEnd := T("The run ends at the exhaustion event or, without exhaustion, at the end of the recording, censored at max_trial_duration_s.",
		"Koşu, tükenme olayında ya da tükenme yoksa kaydın sonunda biter; en fazla max_trial_duration_s sürer.")
	noPairs := T("Missing (NO_VALID_INTERVALS) when there are no counted intervals.", "Sayılan aralık yoksa eksik (NO_VALID_INTERVALS).")
	return definition{
		paradigm: Paradigm{
			Key: "TREADMILL", Category: Motor, Version: 1,
			Name: T("Treadmill exhaustion test", "Koşu bandı testi"),
			Description: T(
				"The animal runs on a motorized treadmill at a constant or increasing speed until exhaustion. Assesses endurance and exercise capacity.",
				"Denek sabit ya da giderek artan hızdaki motorlu bir bant üzerinde, artık devam edemeyene kadar koşar. Dayanıklılığı ve egzersiz kapasitesini ölçer.",
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
				param("exhaustion_dwell_s", AnalysisScope, T("Exhaustion time on the grid", "Tükenme için ızgarada kalma süresi"),
					T("Continuous time on the shock grid that defines exhaustion: the subject no longer returns to the belt.",
						"Tükenmeyi tanımlayan, şok ızgarasında kesintisiz kalma süresi: denek artık banda geri dönmez."),
					Second, 1, 60, 5),
				param("min_shock_dwell_s", AnalysisScope, T("Minimum grid contact", "En kısa ızgara teması"),
					T("A stay on the shock grid counts as a shock only when it lasts at least this long; shorter contacts are ignored.",
						"Şok ızgarasında kalma, yalnızca en az bu kadar sürdüğünde şok sayılır; daha kısa temaslar sayılmaz."),
					Second, 0, 5, 0.2),
				gapParameter,
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
					T("The trial ends at this time even if the subject is still running.", "Denek hâlâ koşuyor olsa bile deneme bu sürede sona erer."),
					Second, 60, 7200, 1800),
			},
			Zones: []Zone{
				calibratedZone("shock_grid", "SHOCK_GRID", T("Shock grid", "Şok ızgarası"),
					T("Polygon drawn on the side view over the shock grid at the back of the lane.",
						"Yandan görünüşte, kulvarın arkasındaki şok ızgarasına çizilen çokgen.")),
			},
			Events: []EventDefinition{
				{Type: "exhaustion", Kind: PointEvent, Single: true, Label: T("Exhaustion", "Tükenme"),
					Definition: J(stay, T("Exhaustion is placed at the start of the first stay that lasts at least exhaustion_dwell_s.",
						"Tükenme, en az exhaustion_dwell_s süren ilk kalmanın başlangıcıdır."))},
				{Type: "shock", Kind: PointEvent, Label: T("Shock", "Şok"),
					Definition: J(stay, T("A shock is placed at the start of every stay that lasts at least min_shock_dwell_s.",
						"Şok, en az min_shock_dwell_s süren her kalmanın başlangıcıdır."))},
			},
			Metrics: []MetricDefinition{
				metric("latency_to_exhaustion_s", T("Time to exhaustion", "Tükenme latansı"),
					T("Time until the subject meets the exhaustion criterion; longer times indicate greater endurance.",
						"Deneğin tükenme ölçütünü karşılamasına kadar geçen süre. Daha uzun latans daha yüksek dayanıklılığa işaret eder."),
					Second, 7200, run,
					J(frame, trackedPair, T("Time of the exhaustion event, if it occurs by the cut-off time.", "Deneme süresi sınırına kadar gerçekleşen tükenme olayının zamanı.")), "exhaustion time",
					T("Missing (NO_VALID_INTERVALS) when there are no counted intervals, (EVENT_NOT_OBSERVED) when the recording covers max_trial_duration_s without exhaustion, and (INCOMPLETE_RECORDING) when a shorter recording ends without exhaustion.",
						"Sayılan aralık yoksa eksik (NO_VALID_INTERVALS), kayıt max_trial_duration_s süresini tükenme olmadan kapsıyorsa eksik (EVENT_NOT_OBSERVED), daha kısa bir kayıt tükenme olmadan bittiyse eksik (INCOMPLETE_RECORDING).")),
				metric("run_time_s", T("Running time", "Koşu süresi"),
					T("Time spent running, until exhaustion or the cut-off time.", "Deneğin tükenmeye ya da deneme süresi sınırına kadar koştuğu süre."),
					Second, 7200, run, runEnd,
					"exhaustion time, else min(recording duration, max_trial_duration_s)", noPairs),
				metric("run_distance_cm", T("Distance run", "Koşu mesafesi"),
					T("Distance covered on the treadmill during the run; longer distances indicate greater endurance.",
						"Koşu boyunca bant üzerinde kat edilen mesafe. Daha uzun mesafe daha yüksek dayanıklılığa işaret eder."),
					Centimeter, 1e7, slices.Concat(run, belt),
					T("Integral of belt speed over the running time; the speed rises linearly from the start speed to the end speed over acceleration_duration_s and then remains constant.",
						"Bant hızının koşu süresi boyunca integrali; hız acceleration_duration_s boyunca başlangıç değerinden bitiş değerine doğrusal olarak artar, sonra sabit kalır."),
					"integral of speed(t) dt over [0, run_time_s]", noPairs),
				metric("shock_count", T("Shock count", "Şok sayısı"),
					T("Number of shocks or rear-grid contacts during the run; more shocks suggest fatigue or low motivation.",
						"Koşu sırasında kaç şok ya da arka ızgara teması olduğu. Daha fazla şok düşük motivasyona ya da yorgunluğa işaret eder."),
					Count, 1e6, slices.Concat(run, []string{"min_shock_dwell_s"}),
					T("Shock events strictly before the end of the run.", "Koşunun bitişinden kesin olarak önce gerçekleşen şok olayları."), "count(shock time < run end)", noPairs),
			},
			QC: trajectoryQC,
		},
		validate: func(p map[string]float64) error {
			if p["end_speed_cm_s"] < p["start_speed_cm_s"] {
				return fmt.Errorf("%w: end_speed_cm_s must not be below start_speed_cm_s", ErrParameterOutOfRange)
			}
			return nil
		},
		evaluate: evaluateTreadmill,
	}
}

func evaluateTreadmill(e evaluation) computation {
	p := e.params
	cutoffUs := microseconds(p["max_trial_duration_s"])
	samples := censor(e.samples, cutoffUs)
	t := walk(samples, validOnly, []zone{shapeZone(e, "shock_grid")}, gapUs(p), nil)
	if t.durationUs == 0 {
		return computation{reason: ReasonNoValidIntervals}
	}
	c := newComputation()
	shockUs, exhaustionDwellUs := microseconds(p["min_shock_dwell_s"]), microseconds(p["exhaustion_dwell_s"])
	exhaustionUs := int64(-1)
	var shocks []int64
	for _, stay := range t.runs("shock_grid") {
		length := stay.EndUs - stay.StartUs
		if length >= shockUs {
			shocks = append(shocks, stay.StartUs)
			c.points = append(c.points, Point{Type: "shock", AtUs: stay.StartUs})
		}
		if exhaustionUs < 0 && length >= exhaustionDwellUs {
			exhaustionUs = stay.StartUs
			c.points = append(c.points, Point{Type: "exhaustion", AtUs: stay.StartUs})
		}
	}
	recordedUs := e.durationUs
	if recordedUs == 0 {
		recordedUs = samples[len(samples)-1].TUs
	}
	endUs := min(recordedUs, cutoffUs)
	switch {
	case exhaustionUs >= 0:
		endUs = exhaustionUs
		c.values["latency_to_exhaustion_s"] = seconds(exhaustionUs)
	case recordedUs >= cutoffUs:
		c.missing["latency_to_exhaustion_s"] = ReasonNotObserved
	default:
		c.missing["latency_to_exhaustion_s"] = ReasonIncompleteRecording
	}
	c.values["run_time_s"] = seconds(endUs)
	c.values["run_distance_cm"] = beltDistance(p, seconds(endUs))
	n := 0
	for _, at := range shocks {
		if at < endUs {
			n++
		}
	}
	c.values["shock_count"] = float64(n)
	return c
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
