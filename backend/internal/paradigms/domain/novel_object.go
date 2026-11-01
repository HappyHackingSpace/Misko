package domain

// Object zones and the label their exploration bouts carry.
var objectLabels = []struct{ zone, label string }{{"novel_object", "novel"}, {"familiar_object", "familiar"}}

func novelObjectV1() definition {
	exploration := []string{"samples", "max_sample_gap_s", "min_bout_s", "novel_object zone", "familiar_object zone"}
	frame := T("Coordinates are in centimeters in the calibration's arena frame: origin at a corner of the arena floor, x to the right and y down the image.",
		"Koordinatlar kalibrasyonun arena çerçevesinde santimetre cinsindendir: başlangıç noktası arena zemininin bir köşesidir, x görüntüde sağa, y aşağıya doğrudur.")
	bout := T("An exploration bout is a run of contiguous counted intervals whose earlier sample lies in the object's zone and that lasts at least min_bout_s; shorter visits are not exploration.",
		"Keşif dönemi, önceki örneği nesnenin bölgesinde olan ve en az min_bout_s süren bitişik sayılan aralıklardır; daha kısa ziyaretler keşif sayılmaz.")
	zoneGeometry := func(en, tr string) Text {
		return T("Drawn on the calibration around the "+en+" object, enclosing the object's footprint plus the exploration distance (about 2 cm). One circle or polygon.",
			"Kalibrasyonda "+tr+" nesnenin çevresine çizilir; nesnenin kapladığı alanı ve keşif mesafesini (yaklaşık 2 cm) içerir. Bir daire ya da çokgen.")
	}
	missing := T("Missing (NO_VALID_INTERVALS) when there are no counted intervals.", "Sayılan aralık yoksa eksik (NO_VALID_INTERVALS).")
	return definition{
		paradigm: Paradigm{
			Key: "NOVEL_OBJECT", Category: LearningMemory, Version: 1,
			Name: T("Novel object recognition test", "Yeni nesne tanıma testi"),
			Description: T(
				"After a familiarization trial with two identical objects, the animal is tested with one familiar and one novel object. Assesses recognition memory: an animal that remembers the familiar object spends more time exploring the novel one.",
				"İki özdeş nesneyi keşfettikten sonra denek, biri tanıdık biri yeni iki nesneyle test edilir. Tanıma belleğini ölçer: tanıdık nesneyi hatırlayan denek yeni nesneyi daha uzun süre keşfeder.",
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
				param("min_bout_s", AnalysisScope, T("Minimum exploration bout", "En kısa keşif dönemi"),
					T("A visit to an object's zone counts as exploration only when the subject stays in the zone at least this long; shorter passes are ignored.",
						"Bir nesnenin bölgesine yapılan ziyaret, denek bölgede en az bu kadar kaldığında keşif sayılır; daha kısa geçişler sayılmaz."),
					Second, 0, 5, 0.3),
				param("min_total_exploration_s", AnalysisScope, T("Minimum total exploration time", "En az toplam keşif süresi"),
					T("No discrimination index is computed for subjects whose combined exploration of both objects falls below this value. Set to 0 to disable the criterion.",
						"İki nesneyi toplamda bundan daha az keşfeden denekler için ayırt etme indeksi hesaplanmaz. 0 bu ölçütü kapatır."),
					Second, 0, 120, 0),
				gapParameter,
			},
			Zones: []Zone{
				calibratedZone("novel_object", "OBJECT", T("Novel object", "Yeni nesne"), zoneGeometry("novel", "yeni")),
				calibratedZone("familiar_object", "OBJECT", T("Familiar object", "Tanıdık nesne"), zoneGeometry("familiar", "tanıdık")),
			},
			Events: []EventDefinition{{Type: "exploration", Kind: IntervalEvent, Labels: []string{"novel", "familiar"},
				Label: T("Object exploration", "Nesne keşfi"),
				Definition: J(bout, T("The label names the object: novel or familiar. Exploration is approximated by the body center in the zone, so sitting on the object counts too.",
					"Etiket nesneyi belirtir: novel (yeni) ya da familiar (tanıdık). Keşif, vücut merkezinin bölgede olmasıyla yaklaşık olarak belirlenir; bu nedenle nesnenin üzerinde oturmak da sayılır."))}},
			Metrics: []MetricDefinition{
				metric("novel_object_time_s", T("Novel object exploration time", "Yeni nesne keşif süresi"),
					T("Time spent exploring the novel object.", "Yeni nesneyi keşfetmekle geçen süre."),
					Second, 86400, exploration, J(frame, trackedPair, T("Total length of the novel object's exploration bouts.", "Yeni nesne keşif dönemlerinin toplam uzunluğu.")),
					"sum(novel bout lengths)", missing),
				metric("familiar_object_time_s", T("Familiar object exploration time", "Tanıdık nesne keşif süresi"),
					T("Time spent exploring the familiar object.", "Tanıdık nesneyi keşfetmekle geçen süre."),
					Second, 86400, exploration, T("Total length of the familiar object's exploration bouts.", "Tanıdık nesne keşif dönemlerinin toplam uzunluğu."),
					"sum(familiar bout lengths)", missing),
				metric("total_exploration_time_s", T("Total exploration time", "Toplam keşif süresi"),
					T("Time spent exploring either object; very low values make the discrimination index unreliable.",
						"Herhangi bir nesneyi keşfetmekle geçen süre; çok düşük değerler ayırt etme indeksini güvenilmez kılar."),
					Second, 86400, exploration,
					T("Sum of the novel and familiar object exploration times.", "Yeni ve tanıdık nesne keşfinin toplamı."), "novel_object_time_s + familiar_object_time_s", missing),
				func() MetricDefinition {
					m := metric("discrimination_index", T("Discrimination index", "Ayırt etme indeksi"),
						T("Preference for the novel object, from -1 to +1. Positive values indicate that the familiar object is remembered; values near 0 indicate no preference.",
							"Yeni nesneye tercih, -1 ile +1 arasında. Pozitif değerler tanıdık nesnenin hatırlandığına, 0 civarındaki değerler tercih olmadığına işaret eder."),
						Ratio, 1, append(exploration, "min_total_exploration_s"),
						T("Relative exploration of the novel object, from -1 (familiar object only) to +1 (novel object only).", "Tanıma belleği, -1 (tanıdık) ile +1 (yeni) arasında."),
						"(novel - familiar) / (novel + familiar)",
						T("Missing (NO_VALID_INTERVALS) when there are no counted intervals, (BELOW_EXPLORATION_CRITERION) when total exploration time is below min_total_exploration_s, and (ZERO_DENOMINATOR) when it is 0.",
							"Sayılan aralık yoksa eksik (NO_VALID_INTERVALS), toplam keşif min_total_exploration_s değerinin altındaysa eksik (BELOW_EXPLORATION_CRITERION), toplam keşif 0 ise eksik (ZERO_DENOMINATOR)."))
					m.Min = -1
					return m
				}(),
			},
			QC: trajectoryQC,
		},
		evaluate: evaluateNovelObject,
	}
}

func evaluateNovelObject(e evaluation) computation {
	var zones []zone
	for _, o := range objectLabels {
		zones = append(zones, shapeZone(e, o.zone))
	}
	t := walk(e.samples, validOnly, zones, gapUs(e.params), nil)
	if t.durationUs == 0 {
		return computation{reason: ReasonNoValidIntervals}
	}
	c := newComputation()
	minBoutUs := microseconds(e.params["min_bout_s"])
	total := map[string]int64{}
	for _, o := range objectLabels {
		for _, run := range t.runs(o.zone) {
			if run.EndUs-run.StartUs < minBoutUs {
				continue
			}
			total[o.label] += run.EndUs - run.StartUs
			c.intervals = append(c.intervals, Interval{Type: "exploration", Label: o.label, StartUs: run.StartUs, EndUs: run.EndUs})
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
}
