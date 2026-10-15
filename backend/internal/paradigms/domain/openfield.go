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
	zoneList := []Zone{
		{Key: "center", Role: "CENTER", Label: T("Center", "Merkez"),
			Geometry: "Axis-aligned rectangle centered in the arena; each side is center_fraction of the arena side. The boundary belongs to the center."},
		{Key: "periphery", Role: "PERIPHERY", Label: T("Periphery", "Çevre"), Geometry: "Arena area outside the center zone."},
	}
	return definition{
		paradigm: Paradigm{
			Key: "OPEN_FIELD", Category: Anxiety, Version: 1,
			Name: T("Open field", "Açık alan testi"),
			Description: T(
				"Free exploration of an empty square arena. Measures locomotor activity and anxiety-like behaviour: anxious animals stay near the walls and avoid the center.",
				"Boş, kare bir arenada serbest keşif. Lokomotor aktiviteyi ve anksiyete benzeri davranışı ölçer: kaygılı hayvanlar duvar dibinde kalır, merkezden kaçınır.",
			),
			Species: []string{"MOUSE", "RAT"}, TrialTypes: []TrialType{trialType("STANDARD", T("Standard", "Standart"))},
			Parameters: []Parameter{
				param("arena_width_cm", ApparatusScope, T("Arena width", "Arena genişliği"),
					T("Inside width of the arena floor, wall to wall, along the horizontal axis of the camera image.",
						"Arena zemininin iç genişliği; duvardan duvara, kamera görüntüsünün yatay ekseni boyunca."),
					Centimeter, 20, 150, 50),
				param("arena_height_cm", ApparatusScope, T("Arena depth", "Arena derinliği"),
					T("Inside depth of the arena floor, wall to wall, along the vertical axis of the camera image.",
						"Arena zemininin iç derinliği; duvardan duvara, kamera görüntüsünün dikey ekseni boyunca."),
					Centimeter, 20, 150, 50),
				param("center_fraction", AnalysisScope, T("Center zone size", "Merkez bölge boyutu"),
					T("Side of the center square as a share of the arena side. 50% makes the center half as wide and half as deep as the arena.",
						"Merkez karenin kenarının arena kenarına oranı. %50, merkezi arenanın yarı genişliğinde ve yarı derinliğinde yapar."),
					Ratio, 0.2, 0.8, 0.5),
				gapParameter,
				param("immobility_threshold_cm_s", AnalysisScope, T("Immobility speed threshold", "Hareketsizlik hız eşiği"),
					T("Below this speed the animal counts as immobile.", "Hayvan bu hızın altındayken hareketsiz sayılır."),
					CentimeterPerSecond, 0.1, 10, 2),
				param("min_immobility_bout_s", AnalysisScope, T("Shortest immobility period", "En kısa hareketsizlik süresi"),
					T("Immobile periods shorter than this are not counted.", "Bundan kısa hareketsiz dönemler sayılmaz."),
					Second, 0, 10, 1),
			},
			Zones: zoneList,
			Events: []EventDefinition{
				{Type: "in_center", Kind: IntervalEvent, Label: T("In center", "Merkezde"),
					Definition: "Contiguous counted intervals whose earlier sample is in the center."},
				{Type: "immobile", Kind: IntervalEvent, Label: T("Immobile", "Hareketsiz"),
					Definition: "Contiguous counted intervals with speed below immobility_threshold_cm_s, lasting at least min_immobility_bout_s."},
				{Type: "center_entry", Kind: PointEvent, Label: T("Center entry", "Merkeze giriş"),
					Definition: "Time of the later sample of a counted interval that moves from periphery to center."},
			},
			Metrics: []MetricDefinition{
				metric("duration_s", T("Analyzed duration", "Analiz edilen süre"),
					T("Time during which the animal could be tracked.", "Hayvanın izlenebildiği toplam süre."),
					Second, 86400, trajectory, "Time covered by counted intervals. "+arenaFrame+" "+countedPair, "sum(dt over counted intervals)", missingNoPairs),
				metric("distance_cm", T("Distance traveled", "Kat edilen mesafe"),
					T("Total length of the path the animal walked.", "Hayvanın yürüdüğü yolun toplam uzunluğu."),
					Centimeter, 1e6, trajectory, "Path length over counted intervals; lost frames are not interpolated.", "sum(hypot(dx, dy) over counted intervals)", missingNoPairs),
				metric("mean_speed_cm_s", T("Mean speed", "Ortalama hız"),
					T("Distance traveled divided by the analyzed duration.", "Kat edilen mesafenin analiz edilen süreye bölümü."),
					CentimeterPerSecond, 1e4, []string{"distance_cm", "duration_s"}, "Distance traveled divided by analyzed duration.", "distance_cm / duration_s", missingNoPairs),
				metric("center_time_s", T("Time in center", "Merkezde geçen süre"),
					T("Less time in the center suggests more anxiety-like behaviour.", "Merkezde daha az süre, daha fazla anksiyete benzeri davranışa işaret eder."),
					Second, 86400, zones, "Counted interval time whose earlier sample is in the center zone.", "sum(dt where earlier sample in center)", missingNoPairs),
				metric("periphery_time_s", T("Time in periphery", "Çevrede geçen süre"),
					T("Time spent near the walls, outside the center.", "Merkez dışında, duvarlara yakın geçen süre."),
					Second, 86400, zones, "Counted interval time whose earlier sample is outside the center zone.", "sum(dt where earlier sample in periphery)", missingNoPairs),
				metric("center_time_ratio", T("Share of time in center", "Merkezde geçen süre oranı"),
					T("Part of the analyzed duration spent in the center.", "Analiz edilen sürenin merkezde geçen kısmı."),
					Ratio, 1, []string{"center_time_s", "duration_s"}, "Share of analyzed duration spent in the center.", "center_time_s / duration_s", missingNoPairs),
				metric("periphery_time_ratio", T("Share of time in periphery", "Çevrede geçen süre oranı"),
					T("Part of the analyzed duration spent near the walls.", "Analiz edilen sürenin duvar yakınında geçen kısmı."),
					Ratio, 1, []string{"periphery_time_s", "duration_s"}, "Share of analyzed duration spent in the periphery.", "periphery_time_s / duration_s", missingNoPairs),
				metric("center_entries_count", T("Center entries", "Merkeze giriş sayısı"),
					T("How many times the animal walked into the center.", "Hayvanın merkeze kaç kez girdiği."),
					Count, 1e6, zones, "Counted intervals moving from periphery to center. A trajectory starting in the center does not count as an entry; no debounce is applied in version 1.",
					"count(counted intervals with earlier sample in periphery and later sample in center)", missingNoPairs),
				metric("immobility_s", T("Immobility time", "Hareketsizlik süresi"),
					T("Time spent not moving, such as freezing or resting.", "Donma ya da dinlenme gibi, hareket etmeden geçen süre."),
					Second, 86400, append(trajectory, "immobility_threshold_cm_s", "min_immobility_bout_s"),
					"Time in immobile bouts. An interval is immobile when hypot(dx, dy) / dt is strictly below the threshold; contiguous immobile intervals form a bout, and bouts shorter than min_immobility_bout_s are ignored.",
					"sum(bout duration where bout duration >= min_immobility_bout_s)", missingNoPairs),
			},
			QC: trajectoryQC,
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
