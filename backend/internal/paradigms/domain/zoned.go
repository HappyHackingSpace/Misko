package domain

import (
	"fmt"
	"strings"
)

func calibratedZone(key, role string, label Text, geometry string, required bool) Zone {
	return Zone{Key: key, Role: role, Label: label, Geometry: geometry, Calibrated: true, Required: required}
}

func elevatedPlusMazeV1() definition {
	trajectory := []string{"samples", "max_sample_gap_s"}
	zoned := append(trajectory, "calibrated zones")
	zoneList := []Zone{
		calibratedZone("center", "CENTER", T("Center", "Merkez"), "Central square where the arms meet.", true),
		calibratedZone("open_arms", "OPEN", T("Open arms", "Açık kollar"), "Both open arms, one shape per arm.", true),
		calibratedZone("closed_arms", "CLOSED", T("Closed arms", "Kapalı kollar"), "Both closed arms, one shape per arm.", true),
	}
	return definition{
		paradigm: Paradigm{
			Key: "EPM", Category: Anxiety, Version: 1,
			Name: T("Elevated plus maze", "Yükseltilmiş artı labirent"),
			Description: T(
				"A raised plus-shaped maze with two open and two walled arms. Measures anxiety-like behaviour: anxious animals avoid the open arms and stay in the closed arms.",
				"Yerden yükseltilmiş, ikisi açık ikisi duvarlı dört kollu artı biçimli bir labirent. Anksiyete benzeri davranışı ölçer: kaygılı hayvanlar açık kollardan kaçınır, kapalı kollarda kalır.",
			),
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []TrialType{trialType("STANDARD", T("Standard", "Standart"))},
			Parameters: []Parameter{
				param("arm_length_cm", ApparatusScope, T("Arm length", "Kol uzunluğu"),
					T("Length of one arm from the edge of the center square to the arm tip. It is only used to check the drawn zones, not in the calculations.",
						"Bir kolun merkez karenin kenarından kol ucuna kadar uzunluğu. Yalnızca çizilen bölgeleri kontrol etmek için kullanılır, hesaplamalara girmez."),
					Centimeter, 20, 80, 35),
				param("arm_width_cm", ApparatusScope, T("Arm width", "Kol genişliği"),
					T("Inside width of one arm. It is only used to check the drawn zones, not in the calculations.",
						"Bir kolun iç genişliği. Yalnızca çizilen bölgeleri kontrol etmek için kullanılır, hesaplamalara girmez."),
					Centimeter, 3, 15, 6),
				gapParameter,
			},
			Zones: zoneList,
			InputEvents: []EventDefinition{{Type: "risk_assessment", Kind: PointEvent, Label: T("Risk assessment", "Risk değerlendirme"),
				Definition: "A scored stretched-attend or head-dip posture."}},
			Events: append(append(occupancyEvents(zoneList, "center", "open_arms", "closed_arms"), entryEvents(zoneList, "center", "open_arms", "closed_arms")...),
				EventDefinition{Type: "risk_assessment", Kind: PointEvent, Label: T("Risk assessment", "Risk değerlendirme"),
					Definition: "Scored risk assessment, passed through."}),
			Metrics: []MetricDefinition{
				metric("duration_s", T("Analyzed duration", "Analiz edilen süre"),
					T("Time during which the animal could be tracked.", "Hayvanın izlenebildiği toplam süre."),
					Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("distance_cm", T("Distance traveled", "Kat edilen mesafe"),
					T("Total length of the path the animal walked.", "Hayvanın yürüdüğü yolun toplam uzunluğu."),
					Centimeter, 1e6, trajectory, "Path length over counted intervals.", "sum(hypot(dx, dy))", missingNoPairs),
				metric("center_time_s", T("Time in center", "Merkezde geçen süre"),
					T("Time spent in the central square where the arms meet.", "Kolların birleştiği merkez karede geçen süre."),
					Second, 86400, zoned, "Counted time whose earlier sample is in the center.", "sum(dt where earlier sample in center)", missingNoPairs),
				metric("open_arm_time_s", T("Time in open arms", "Açık kollarda geçen süre"),
					T("Less time in the open arms suggests more anxiety-like behaviour.", "Açık kollarda daha az süre, daha fazla anksiyete benzeri davranışa işaret eder."),
					Second, 86400, zoned, "Counted time whose earlier sample is in an open arm.", "sum(dt where earlier sample in open arms)", missingNoPairs),
				metric("closed_arm_time_s", T("Time in closed arms", "Kapalı kollarda geçen süre"),
					T("Time spent in the walled arms.", "Duvarlı kollarda geçen süre."),
					Second, 86400, zoned, "Counted time whose earlier sample is in a closed arm.", "sum(dt where earlier sample in closed arms)", missingNoPairs),
				metric("open_arm_time_ratio", T("Share of time in open arms", "Açık kollarda geçen süre oranı"),
					T("Part of the analyzed duration spent in the open arms; a lower share suggests more anxiety-like behaviour.",
						"Analiz edilen sürenin açık kollarda geçen kısmı; daha düşük oran daha fazla anksiyete benzeri davranışa işaret eder."),
					Ratio, 1, zoned, "Share of analyzed duration in open arms.", "open_arm_time_s / duration_s", missingNoPairs),
				metric("closed_arm_time_ratio", T("Share of time in closed arms", "Kapalı kollarda geçen süre oranı"),
					T("Part of the analyzed duration spent in the closed arms.", "Analiz edilen sürenin kapalı kollarda geçen kısmı."),
					Ratio, 1, zoned, "Share of analyzed duration in closed arms.", "closed_arm_time_s / duration_s", missingNoPairs),
				metric("open_arm_entries_count", T("Open arm entries", "Açık kola giriş sayısı"),
					T("How many times the animal walked into an open arm.", "Hayvanın açık kollara kaç kez girdiği."),
					Count, 1e6, zoned, entryRule, "count(open arm entries)", missingNoPairs),
				metric("closed_arm_entries_count", T("Closed arm entries", "Kapalı kola giriş sayısı"),
					T("How many times the animal walked into a closed arm; often used as a measure of general activity.",
						"Hayvanın kapalı kollara kaç kez girdiği; genellikle genel aktivite ölçüsü olarak kullanılır."),
					Count, 1e6, zoned, entryRule, "count(closed arm entries)", missingNoPairs),
				metric("latency_to_open_arm_s", T("Latency to open arm", "Açık kola ilk giriş süresi"),
					T("Time from the start of the trial until the animal first reaches an open arm.", "Deneme başından hayvanın ilk kez açık kola ulaşmasına kadar geçen süre."),
					Second, 86400, zoned, latencyRule, "first tracked sample time in open arms", missingNotSeen),
				metric("risk_assessment_count", T("Risk assessments", "Risk değerlendirme sayısı"),
					T("Number of scored stretched-attend postures and head dips.", "Puanlanan gerilerek uzanma duruşları ve baş daldırmaların sayısı."),
					Count, 1e6, []string{"risk_assessment events"}, "Scored risk assessment events.", "count(risk_assessment)", "Missing (NOT_SCORED) when risk assessment was not scored."),
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
	zoneList := []Zone{
		calibratedZone("arm_a", "ARM", T("Arm A", "A kolu"), "Arm A.", true),
		calibratedZone("arm_b", "ARM", T("Arm B", "B kolu"), "Arm B.", true),
		calibratedZone("arm_c", "ARM", T("Arm C", "C kolu"), "Arm C.", true),
	}
	return definition{
		paradigm: Paradigm{
			Key: "Y_MAZE", Category: LearningMemory, Version: 1,
			Name: T("Y maze", "Y labirenti"),
			Description: T(
				"Free exploration of a maze with three identical arms. Measures spatial working memory: animals with intact memory tend to visit the three arms in turn, and in a two-trial protocol prefer the newly opened arm.",
				"Üç özdeş kolu olan bir labirentte serbest keşif. Uzamsal çalışma belleğini ölçer: belleği sağlam hayvanlar üç kolu sırayla ziyaret etme eğilimindedir ve iki denemeli protokolde yeni açılan kolu tercih eder.",
			),
			Species: []string{"MOUSE", "RAT"},
			TrialTypes: []TrialType{
				trialType("SPONTANEOUS_ALTERNATION", T("Spontaneous alternation", "Spontan alternasyon")),
				trialType("NOVEL_ARM", T("Novel arm", "Yeni kol")),
			},
			Parameters: []Parameter{
				param("arm_length_cm", ApparatusScope, T("Arm length", "Kol uzunluğu"),
					T("Length of one arm from the center of the maze to the arm tip. It is only used to check the drawn zones, not in the calculations.",
						"Bir kolun labirent merkezinden kol ucuna kadar uzunluğu. Yalnızca çizilen bölgeleri kontrol etmek için kullanılır, hesaplamalara girmez."),
					Centimeter, 20, 60, 35),
				gapParameter,
				integerParam("novel_arm", SessionScope, T("Novel arm", "Yeni kol"),
					T("The arm that was closed in the first trial of a two-trial protocol: 0 for none, 1 for arm A, 2 for arm B, 3 for arm C.",
						"İki denemeli protokolün ilk denemesinde kapalı tutulan kol: yoksa 0, A kolu için 1, B kolu için 2, C kolu için 3."),
					Count, 0, 3, 0),
			},
			Zones:  zoneList,
			Events: append(occupancyEvents(zoneList, "arm_a", "arm_b", "arm_c"), entryEvents(zoneList, "arm_a", "arm_b", "arm_c")...),
			Metrics: []MetricDefinition{
				metric("duration_s", T("Analyzed duration", "Analiz edilen süre"),
					T("Time during which the animal could be tracked.", "Hayvanın izlenebildiği toplam süre."),
					Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("distance_cm", T("Distance traveled", "Kat edilen mesafe"),
					T("Total length of the path the animal walked.", "Hayvanın yürüdüğü yolun toplam uzunluğu."),
					Centimeter, 1e6, trajectory, "Path length over counted intervals.", "sum(hypot(dx, dy))", missingNoPairs),
				metric("total_arm_entries_count", T("Total arm entries", "Toplam kol girişi"),
					T("How many arms the animal entered, including the arm it started in; a measure of general activity.",
						"Başladığı kol dahil hayvanın kaç kola girdiği; genel aktivite ölçüsüdür."),
					Count, 1e6, arms,
					"The arm holding the first tracked sample counts as the first entry; each later arm entry follows the entry rule, and re-entering the same arm counts again. "+entryRule,
					"1 (start arm) + count(arm entries)", missingNoPairs),
				metric("spontaneous_alternation_ratio", T("Spontaneous alternation", "Spontan alternasyon oranı"),
					T("Share of three consecutive arm entries that visit three different arms; a lower value suggests impaired working memory.",
						"Art arda üç kol girişinden üç farklı kolu ziyaret edenlerin oranı; daha düşük değer çalışma belleğinde bozulmaya işaret eder."),
					Ratio, 1, arms,
					"Share of overlapping entry triplets that visit three different arms.", "alternations / (total_arm_entries_count - 2)", "Missing (INSUFFICIENT_ENTRIES) with fewer than 3 entries; missing (NO_VALID_INTERVALS) without counted intervals."),
				metric("novel_arm_time_ratio", T("Share of time in novel arm", "Yeni kolda geçen süre oranı"),
					T("Part of the analyzed duration spent in the novel arm; a higher share suggests the animal remembers the familiar arms.",
						"Analiz edilen sürenin yeni kolda geçen kısmı; daha yüksek oran hayvanın tanıdık kolları hatırladığına işaret eder."),
					Ratio, 1, append(arms, "novel_arm"),
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
	zoneList := []Zone{
		calibratedZone("target_hole", "TARGET", T("Target hole", "Hedef delik"), "The escape hole.", true),
		calibratedZone("holes", "ERROR", T("Other holes", "Diğer delikler"), "Every other hole, one shape per hole.", true),
	}
	return definition{
		paradigm: Paradigm{
			Key: "BARNES_MAZE", Category: LearningMemory, Version: 1,
			Name: T("Barnes maze", "Barnes labirenti"),
			Description: T(
				"A brightly lit round platform with holes around the edge, only one of which leads to an escape box. Measures spatial learning and memory: with training, animals find the escape hole faster and with fewer wrong holes.",
				"Kenarında delikler bulunan, iyi aydınlatılmış yuvarlak bir platform; deliklerden yalnızca biri kaçış kutusuna açılır. Uzamsal öğrenme ve belleği ölçer: eğitimle hayvanlar kaçış deliğini daha hızlı ve daha az yanlış delikle bulur.",
			),
			Species: []string{"MOUSE", "RAT"},
			TrialTypes: []TrialType{
				trialType("ACQUISITION", T("Acquisition", "Öğrenme")),
				trialType("PROBE", T("Probe", "Bellek testi")),
			},
			Parameters: []Parameter{
				param("platform_diameter_cm", ApparatusScope, T("Platform diameter", "Platform çapı"),
					T("Diameter of the round platform, edge to edge. It is only used to check the drawn zones, not in the calculations.",
						"Yuvarlak platformun kenardan kenara çapı. Yalnızca çizilen bölgeleri kontrol etmek için kullanılır, hesaplamalara girmez."),
					Centimeter, 60, 150, 92),
				integerParam("hole_count", ApparatusScope, T("Number of holes", "Delik sayısı"),
					T("Total number of holes around the platform edge, including the escape hole. It is only used to check the drawn zones, not in the calculations.",
						"Kaçış deliği dahil platform kenarındaki toplam delik sayısı. Yalnızca çizilen bölgeleri kontrol etmek için kullanılır, hesaplamalara girmez."),
					Count, 12, 40, 20),
				gapParameter,
			},
			Zones: zoneList,
			Events: append(append(occupancyEvents(zoneList, "target_hole"), entryEvents(zoneList, "target_hole")...),
				EventDefinition{Type: "hole_error", Kind: PointEvent, Label: T("Wrong hole visit", "Yanlış delik ziyareti"),
					Definition: "Later sample time of a counted interval entering a non-target hole."}),
			Metrics: []MetricDefinition{
				metric("duration_s", T("Analyzed duration", "Analiz edilen süre"),
					T("Time during which the animal could be tracked.", "Hayvanın izlenebildiği toplam süre."),
					Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("distance_cm", T("Distance traveled", "Kat edilen mesafe"),
					T("Total length of the path the animal walked.", "Hayvanın yürüdüğü yolun toplam uzunluğu."),
					Centimeter, 1e6, trajectory, "Path length over counted intervals.", "sum(hypot(dx, dy))", missingNoPairs),
				metric("primary_latency_s", T("Latency to target hole", "Hedef deliğe ulaşma süresi"),
					T("Time from the start of the trial until the animal first reaches the escape hole; shorter times indicate better learning.",
						"Deneme başından hayvanın kaçış deliğine ilk kez ulaşmasına kadar geçen süre; daha kısa süre daha iyi öğrenmeyi gösterir."),
					Second, 86400, holes, latencyRule+" The zone is target_hole.", "first tracked sample time in target_hole", missingNotSeen),
				metric("primary_errors_count", T("Errors before target", "Hedeften önceki hatalar"),
					T("Visits to wrong holes before the escape hole is first reached; fewer errors indicate better learning.",
						"Kaçış deliğine ilk ulaşmadan önce yanlış deliklere yapılan ziyaretler; daha az hata daha iyi öğrenmeyi gösterir."),
					Count, 1e6, holes,
					"Entries into non-target holes before the primary latency. Each hole is a separate zone, so re-entering a hole counts again. "+entryRule,
					"count(hole entries before primary_latency_s)", "Missing (EVENT_NOT_OBSERVED) when the target is never reached; missing (NO_VALID_INTERVALS) without counted intervals."),
				metric("total_errors_count", T("Total errors", "Toplam hata"),
					T("Visits to wrong holes over the whole trial.", "Deneme boyunca yanlış deliklere yapılan toplam ziyaret."),
					Count, 1e6, holes, "Entries into non-target holes over the whole trial.", "count(hole entries)", missingNoPairs),
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
	zoneList := []Zone{
		calibratedZone("social_chamber", "TARGET", T("Social chamber", "Sosyal bölme"), "Chamber with the stimulus animal.", true),
		calibratedZone("object_chamber", "CONTROL", T("Object chamber", "Nesne bölmesi"), "Chamber with the empty cage or object.", true),
		calibratedZone("center_chamber", "CENTER", T("Center chamber", "Orta bölme"), "Middle chamber.", true),
		calibratedZone("social_interaction", "TARGET", T("Social interaction area", "Sosyal etkileşim alanı"), "Close-interaction area around the stimulus cage.", false),
		calibratedZone("object_interaction", "CONTROL", T("Object interaction area", "Nesne etkileşim alanı"), "Close-interaction area around the object cage.", false),
	}
	return definition{
		paradigm: Paradigm{
			Key: "THREE_CHAMBER", Category: Social, Version: 1,
			Name: T("Three-chamber sociability", "Üç bölmeli sosyallik testi"),
			Description: T(
				"A box divided into three chambers, with an unfamiliar animal in a cage on one side and an empty cage or object on the other. Measures social preference: typical animals spend more time near the other animal than near the object.",
				"Üç bölmeye ayrılmış bir kutu; bir yanda kafes içinde tanımadık bir hayvan, diğer yanda boş kafes ya da nesne bulunur. Sosyal tercihi ölçer: tipik hayvanlar nesneden çok diğer hayvanın yanında zaman geçirir.",
			),
			Species: []string{"MOUSE", "RAT"},
			TrialTypes: []TrialType{
				trialType("SOCIABILITY", T("Sociability", "Sosyallik")),
				trialType("SOCIAL_NOVELTY", T("Social novelty", "Sosyal yenilik")),
			},
			Parameters: []Parameter{
				param("chamber_width_cm", ApparatusScope, T("Chamber width", "Bölme genişliği"),
					T("Inside width of one chamber, from the outer wall to the partition. It is only used to check the drawn zones, not in the calculations.",
						"Bir bölmenin dış duvardan ara bölmeye kadar iç genişliği. Yalnızca çizilen bölgeleri kontrol etmek için kullanılır, hesaplamalara girmez."),
					Centimeter, 15, 40, 20),
				param("chamber_height_cm", ApparatusScope, T("Chamber depth", "Bölme derinliği"),
					T("Inside depth of one chamber, front wall to back wall. It is only used to check the drawn zones, not in the calculations.",
						"Bir bölmenin ön duvardan arka duvara iç derinliği. Yalnızca çizilen bölgeleri kontrol etmek için kullanılır, hesaplamalara girmez."),
					Centimeter, 20, 60, 40),
				gapParameter,
			},
			Zones: zoneList,
			Events: append(occupancyEvents(zoneList, "social_chamber", "object_chamber", "center_chamber", "social_interaction", "object_interaction"),
				entryEvents(zoneList, "social_chamber", "object_chamber", "center_chamber")...),
			Metrics: []MetricDefinition{
				metric("duration_s", T("Analyzed duration", "Analiz edilen süre"),
					T("Time during which the animal could be tracked.", "Hayvanın izlenebildiği toplam süre."),
					Second, 86400, trajectory, calibratedFrame+" "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("social_chamber_time_s", T("Time in social chamber", "Sosyal bölmede geçen süre"),
					T("Time spent in the chamber with the other animal.", "Diğer hayvanın bulunduğu bölmede geçen süre."),
					Second, 86400, chambers, "Counted time whose earlier sample is in the social chamber.", "sum(dt in social_chamber)", missingNoPairs),
				metric("object_chamber_time_s", T("Time in object chamber", "Nesne bölmesinde geçen süre"),
					T("Time spent in the chamber with the empty cage or object.", "Boş kafesin ya da nesnenin bulunduğu bölmede geçen süre."),
					Second, 86400, chambers, "Counted time whose earlier sample is in the object chamber.", "sum(dt in object_chamber)", missingNoPairs),
				metric("center_chamber_time_s", T("Time in center chamber", "Orta bölmede geçen süre"),
					T("Time spent in the middle chamber.", "Orta bölmede geçen süre."),
					Second, 86400, chambers, "Counted time whose earlier sample is in the center chamber.", "sum(dt in center_chamber)", missingNoPairs),
				func() MetricDefinition {
					m := metric("sociability_index", T("Sociability index", "Sosyallik indeksi"),
						T("Preference for the social chamber over the object chamber, from -1 (only object) to +1 (only social); values near 0 mean no preference.",
							"Sosyal bölmenin nesne bölmesine tercihi; -1 (yalnızca nesne) ile +1 (yalnızca sosyal) arasındadır, 0 civarı tercih olmadığını gösterir."),
						Ratio, 1, []string{"social_chamber_time_s", "object_chamber_time_s"},
						"Preference for the social chamber, from -1 (object) to +1 (social).", "(social - object) / (social + object)",
						"Missing (ZERO_DENOMINATOR) when both chamber times are 0; missing (NO_VALID_INTERVALS) without counted intervals.")
					m.Min = -1
					return m
				}(),
				metric("social_interaction_time_s", T("Social interaction time", "Sosyal etkileşim süresi"),
					T("Time spent close to the cage with the other animal.", "Diğer hayvanın kafesine yakın geçen süre."),
					Second, 86400, append(trajectory, "social_interaction zone"), "Counted time whose earlier sample is in the social interaction zone.", "sum(dt in social_interaction)", zoneMissing),
				metric("object_interaction_time_s", T("Object interaction time", "Nesne etkileşim süresi"),
					T("Time spent close to the empty cage or object.", "Boş kafese ya da nesneye yakın geçen süre."),
					Second, 86400, append(trajectory, "object_interaction zone"), "Counted time whose earlier sample is in the object interaction zone.", "sum(dt in object_interaction)", zoneMissing),
				metric("social_chamber_entries_count", T("Social chamber entries", "Sosyal bölmeye giriş sayısı"),
					T("How many times the animal walked into the social chamber.", "Hayvanın sosyal bölmeye kaç kez girdiği."),
					Count, 1e6, chambers, entryRule, "count(social_chamber entries)", missingNoPairs),
				metric("object_chamber_entries_count", T("Object chamber entries", "Nesne bölmesine giriş sayısı"),
					T("How many times the animal walked into the object chamber.", "Hayvanın nesne bölmesine kaç kez girdiği."),
					Count, 1e6, chambers, entryRule, "count(object_chamber entries)", missingNoPairs),
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
	zoneList := []Zone{
		{Key: "light", Role: "RISK", Label: T("Light compartment", "Aydınlık bölme"), Geometry: "0 <= x < box_width_cm x light_fraction, within the box.", Required: true},
		{Key: "dark", Role: "CLOSED", Label: T("Dark compartment", "Karanlık bölme"), Geometry: "box_width_cm x light_fraction <= x <= box_width_cm; the dividing line belongs to the dark side.", Required: true},
	}
	return definition{
		paradigm: Paradigm{
			Key: "LIGHT_DARK", Category: Anxiety, Version: 1,
			Name: T("Light/dark box", "Aydınlık/karanlık kutu"),
			Description: T(
				"A box split into a brightly lit compartment and a dark, enclosed compartment connected by an opening. Measures anxiety-like behaviour: anxious animals avoid the light side and cross between the compartments less often.",
				"Bir açıklıkla birbirine bağlanan, iyi aydınlatılmış bir bölme ile kapalı, karanlık bir bölmeye ayrılmış kutu. Anksiyete benzeri davranışı ölçer: kaygılı hayvanlar aydınlık taraftan kaçınır ve bölmeler arasında daha az geçiş yapar.",
			),
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []TrialType{trialType("STANDARD", T("Standard", "Standart"))},
			Parameters: []Parameter{
				param("box_width_cm", ApparatusScope, T("Box length", "Kutu uzunluğu"),
					T("Inside length of the whole box from the light end wall to the dark end wall, along the horizontal axis of the camera image.",
						"Kutunun aydınlık uç duvarından karanlık uç duvarına iç uzunluğu; kamera görüntüsünün yatay ekseni boyunca."),
					Centimeter, 20, 60, 40),
				param("box_height_cm", ApparatusScope, T("Box depth", "Kutu derinliği"),
					T("Inside depth of the box, front wall to back wall, along the vertical axis of the camera image.",
						"Kutunun ön duvardan arka duvara iç derinliği; kamera görüntüsünün dikey ekseni boyunca."),
					Centimeter, 15, 40, 20),
				param("light_fraction", ApparatusScope, T("Light compartment share", "Aydınlık bölme oranı"),
					T("Length of the light compartment as a share of the box length, measured to the partition. 50% means both compartments are equally long.",
						"Ara bölmeye kadar ölçülen aydınlık bölme uzunluğunun kutu uzunluğuna oranı. %50, iki bölmenin eşit uzunlukta olduğunu gösterir."),
					Ratio, 0.4, 0.6, 0.5),
				gapParameter,
			},
			Zones:  zoneList,
			Events: append(occupancyEvents(zoneList, "light", "dark"), entryEvents(zoneList, "light", "dark")...),
			Metrics: []MetricDefinition{
				metric("duration_s", T("Analyzed duration", "Analiz edilen süre"),
					T("Time during which the animal could be tracked.", "Hayvanın izlenebildiği toplam süre."),
					Second, 86400, trajectory,
					"Samples are box-local centimeters with the origin at the light-side corner; samples outside the box count as lost. "+trackedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("light_time_s", T("Time in light", "Aydınlıkta geçen süre"),
					T("Less time in the light compartment suggests more anxiety-like behaviour.", "Aydınlık bölmede daha az süre, daha fazla anksiyete benzeri davranışa işaret eder."),
					Second, 86400, trajectory, "Counted time whose earlier sample is in the light compartment.", "sum(dt in light)", missingNoPairs),
				metric("dark_time_s", T("Time in dark", "Karanlıkta geçen süre"),
					T("Time spent in the dark compartment.", "Karanlık bölmede geçen süre."),
					Second, 86400, trajectory, "Counted time whose earlier sample is in the dark compartment.", "sum(dt in dark)", missingNoPairs),
				metric("light_time_ratio", T("Share of time in light", "Aydınlıkta geçen süre oranı"),
					T("Part of the analyzed duration spent in the light compartment.", "Analiz edilen sürenin aydınlık bölmede geçen kısmı."),
					Ratio, 1, trajectory, "Share of analyzed duration in the light compartment.", "light_time_s / duration_s", missingNoPairs),
				metric("light_entries_count", T("Light entries", "Aydınlığa giriş sayısı"),
					T("How many times the animal crossed from the dark into the light compartment.", "Hayvanın karanlık bölmeden aydınlık bölmeye kaç kez geçtiği."),
					Count, 1e6, trajectory, "Counted intervals from dark to light.", "count(dark to light)", missingNoPairs),
				metric("transitions_count", T("Transitions", "Geçiş sayısı"),
					T("Total crossings between the two compartments in either direction; fewer transitions suggest more anxiety-like behaviour or lower activity.",
						"İki bölme arasında her iki yöndeki toplam geçiş; daha az geçiş daha fazla anksiyete benzeri davranışa ya da düşük aktiviteye işaret eder."),
					Count, 1e6, trajectory, "Counted intervals whose samples are in different compartments.", "count(light to dark) + count(dark to light)", missingNoPairs),
				metric("latency_to_dark_s", T("Latency to dark", "Karanlığa ilk geçiş süresi"),
					T("Time from the start of the trial until the animal first enters the dark compartment.", "Deneme başından hayvanın karanlık bölmeye ilk kez girmesine kadar geçen süre."),
					Second, 86400, trajectory, latencyRule+" The zone is dark.", "first tracked sample time in dark", missingNotSeen),
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
