package domain

import (
	"fmt"
	"math"
)

func morrisWaterMazeV1() definition {
	trajectory := []string{"samples", "tank_diameter_cm", "max_sample_gap_s"}
	platform := append(trajectory, "platform_x_cm", "platform_y_cm", "platform_diameter_cm")
	frame := T("Coordinates are centimeters from the tank center, x east and y north; samples outside the tank count as lost.", "Koordinatlar havuz merkezinden itibaren santimetre cinsindendir; x doğuyu, y kuzeyi gösterir. Havuz dışındaki örnekler kayıp sayılır.")
	zoneList := []Zone{
		{Key: "platform", Role: "TARGET", Label: T("Platform", "Platform"),
			Geometry: T("Closed disk of platform_diameter_cm at the platform center.", "Platform merkezinde, çapı platform_diameter_cm olan kapalı disk."), Required: true},
		{Key: "target_quadrant", Role: "TARGET", Label: T("Target quadrant", "Hedef çeyrek"),
			Geometry: T("Quadrant containing the platform center; x >= 0 is east and y >= 0 is north.", "Platform merkezini içeren çeyrek; x >= 0 doğu, y >= 0 kuzey tarafıdır."), Required: true},
		{Key: "wall_annulus", Role: "PERIPHERY", Label: T("Wall zone", "Duvar kenarı bölgesi"),
			Geometry: T("Ring with distance from the tank center >= tank radius - wall_annulus_width_cm.", "Havuz merkezine uzaklığı >= havuz yarıçapı - wall_annulus_width_cm olan halka."), Required: true},
	}
	return definition{
		paradigm: Paradigm{
			Key: "MWM", Category: LearningMemory, Version: 1,
			Name: T("Morris water maze", "Morris su labirenti"),
			Description: T(
				"The animal swims in a round pool of opaque water and learns to find a hidden platform using cues around the room. Measures spatial learning and memory.",
				"Hayvan, bulanık su dolu yuvarlak bir havuzda yüzer ve odadaki ipuçlarını kullanarak gizli bir platformu bulmayı öğrenir. Uzamsal öğrenmeyi ve hafızayı ölçer.",
			),
			Species: []string{"MOUSE", "RAT"},
			TrialTypes: []TrialType{
				trialType("ACQUISITION", T("Acquisition", "Öğrenme")),
				trialType("PROBE", T("Probe", "Hafıza testi (prob)")),
			},
			Parameters: []Parameter{
				param("tank_diameter_cm", ApparatusScope, T("Pool diameter", "Havuz çapı"),
					T("Inside diameter of the pool at the water surface, wall to wall.",
						"Havuzun su yüzeyindeki iç çapı; duvardan duvara."),
					Centimeter, 60, 250, 120),
				param("platform_diameter_cm", ApparatusScope, T("Platform diameter", "Platform çapı"),
					T("Diameter of the top surface of the escape platform.",
						"Kaçış platformunun üst yüzeyinin çapı."),
					Centimeter, 4, 20, 10),
				param("platform_x_cm", ApparatusScope, T("Platform position, left-right", "Platform konumu, sağ-sol"),
					T("How far the platform center is from the pool center, sideways in the camera image. Positive to the right, negative to the left.",
						"Platform merkezinin havuz merkezine yatay uzaklığı (kamera görüntüsüne göre). Sağa doğru pozitif, sola doğru negatif."),
					Centimeter, -125, 125, 30),
				param("platform_y_cm", ApparatusScope, T("Platform position, up-down", "Platform konumu, yukarı-aşağı"),
					T("How far the platform center is from the pool center, up or down in the camera image. Positive towards the top, negative towards the bottom.",
						"Platform merkezinin havuz merkezine dikey uzaklığı (kamera görüntüsüne göre). Görüntünün üstüne doğru pozitif, altına doğru negatif."),
					Centimeter, -125, 125, 30),
				param("wall_annulus_width_cm", AnalysisScope, T("Wall zone width", "Duvar kenarı bölge genişliği"),
					T("Width of the ring along the pool wall. Time spent in this ring counts as wall hugging (thigmotaxis).",
						"Havuz duvarı boyunca uzanan halkanın genişliği. Bu halkada geçen süre duvar dibinde kalma (tigmotaksis) olarak sayılır."),
					Centimeter, 4, 30, 12),
				param("min_platform_dwell_s", AnalysisScope, T("Time on platform to escape", "Kaçış için platformda kalma süresi"),
					T("The animal must stay on the platform at least this long for the trial to count as escaped. Short touches are ignored.",
						"Denemenin kaçışla bitmiş sayılması için hayvanın platformda en az bu kadar kalması gerekir. Kısa temaslar sayılmaz."),
					Second, 0, 10, 1),
				gapParameter,
			},
			Zones:  zoneList,
			Events: append(occupancyEvents(zoneList, "platform"), entryEvents(zoneList, "platform")...),
			Metrics: []MetricDefinition{
				metric("duration_s", T("Analyzed duration", "Analiz edilen süre"),
					T("Time during which the animal could be tracked.", "Hayvanın izlenebildiği toplam süre."),
					Second, 86400, trajectory, J(frame, trackedPair), "sum(dt over counted intervals)", missingNoPairs),
				metric("escape_latency_s", T("Escape latency", "Kaçış latansı"),
					T("Time to find the platform; shorter latencies across trials mean better learning.",
						"Platformu bulma süresi; denemeler boyunca kısalan süre daha iyi öğrenmeyi gösterir."),
					Second, 86400, append(platform, "min_platform_dwell_s"),
					T("Start of the first platform dwell: consecutive tracked samples on the platform, joined by gaps no longer than max_sample_gap_s, spanning at least min_platform_dwell_s.", "İlk platformda kalma döneminin başlangıcı: platform üzerindeki ardışık izlenmiş örneklerden oluşan, aralarındaki boşluklar max_sample_gap_s değerini aşmayan ve en az min_platform_dwell_s süren dönem."),
					"first dwell start with last - first sample time >= min_platform_dwell_s", T("Missing (EVENT_NOT_OBSERVED) when no dwell qualifies, as in probe trials.", "Koşulu sağlayan bir platformda kalma dönemi yoksa eksik (EVENT_NOT_OBSERVED); hafıza testi (prob) denemelerinde olduğu gibi.")),
				metric("path_length_cm", T("Path length", "Yüzme mesafesi"),
					T("Distance swum before reaching the platform; a shorter path means a more direct search.",
						"Platforma ulaşana kadar yüzülen mesafe; daha kısa yol daha doğrudan bir arama demektir."),
					Centimeter, 1e6, append(platform, "min_platform_dwell_s"),
					T("Distance over counted intervals ending at or before the escape, or over the whole trial without an escape.", "Kaçış anında ya da öncesinde biten sayılan aralıklar boyunca kat edilen mesafe; kaçış yoksa tüm deneme boyunca kat edilen mesafe."), "sum(hypot(dx, dy) for counted intervals ending <= escape)", missingNoPairs),
				metric("mean_swim_speed_cm_s", T("Mean swim speed", "Ortalama yüzme hızı"),
					T("Path length divided by swim time; helps rule out motor problems.",
						"Yüzme mesafesinin yüzme süresine bölümü; motor sorunları ayırt etmeye yardımcı olur."),
					CentimeterPerSecond, 1e4, []string{"path_length_cm"},
					T("Path length divided by the counted time it covers.", "Yüzme mesafesinin, kapsadığı sayılan süreye bölümü."), "path_length_cm / counted time up to escape", T("Missing (NO_VALID_INTERVALS) when no counted time precedes the escape.", "Kaçıştan önce sayılan süre yoksa eksik (NO_VALID_INTERVALS).")),
				metric("target_quadrant_time_ratio", T("Share of time in target quadrant", "Hedef çeyrekte geçen süre oranı"),
					T("More time in the target quadrant during a probe trial means better memory of the platform location.",
						"Hafıza testinde hedef çeyrekte daha fazla süre, platform yerinin daha iyi hatırlandığını gösterir."),
					Ratio, 1, platform, T("Share of analyzed duration whose earlier sample is in the target quadrant.", "Analiz edilen sürenin, önceki örneği hedef çeyrekte olan kısmının oranı."), "target quadrant time / duration_s", missingNoPairs),
				metric("thigmotaxis_time_ratio", T("Share of time near the wall", "Duvar kenarında geçen süre oranı"),
					T("Time spent swimming along the wall; high values suggest anxiety or a poor search strategy.",
						"Duvar boyunca yüzülen süre; yüksek değerler kaygıya ya da zayıf bir arama stratejisine işaret eder."),
					Ratio, 1, append(trajectory, "wall_annulus_width_cm"), T("Share of analyzed duration whose earlier sample is in the wall annulus.", "Analiz edilen sürenin, önceki örneği duvar kenarı bölgesinde olan kısmının oranı."), "wall annulus time / duration_s", missingNoPairs),
				metric("platform_crossings_count", T("Platform crossings", "Platform geçiş sayısı"),
					T("How many times the animal swam over the platform location; more crossings in a probe trial mean better memory.",
						"Hayvanın platform konumunun üzerinden kaç kez geçtiği; hafıza testinde daha fazla geçiş daha iyi hafızayı gösterir."),
					Count, 1e6, platform, T("Counted intervals entering the platform disk; starting on the platform is not a crossing.", "Platform diskine giren sayılan aralıklar; platform üzerinde başlamak geçiş sayılmaz."), "count(platform entries)", missingNoPairs),
				metric("mean_distance_to_platform_cm", T("Mean distance to platform", "Platforma ortalama uzaklık"),
					T("Average distance from the platform location; a smaller value means a more focused search.",
						"Platform konumuna ortalama uzaklık; daha küçük değer daha odaklı bir arama demektir."),
					Centimeter, 1e4, platform, T("Time-weighted mean distance from the platform center, using the earlier sample of each counted interval.", "Her sayılan aralığın önceki örneği kullanılarak hesaplanan, platform merkezine zamanla ağırlıklandırılmış ortalama uzaklık."), "sum(distance(earlier sample, platform) x dt) / sum(dt)", missingNoPairs),
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
