/**
 * Scientific dictionary localization (Step 2).
 *
 * Paradigm/metric specs carry Turkish labels and definitions inline
 * (the default language). This module lets the read-only review API return
 * localized labels via `?lang=`: for `tr` the source text as-is, for `en`
 * the translation from the catalog below (falls back to Turkish if missing).
 *
 * Single-source principle: the backend owns the structure; the frontend only
 * keeps a UI dictionary translating small fixed enums (zone type/role, species).
 */
export const SUPPORTED_LANGS = Object.freeze(["tr", "en"]);
export const DEFAULT_LANG = "tr";

/** Reduces an invalid/missing language to the default. */
export function normalizeLang(lang) {
  return SUPPORTED_LANGS.includes(lang) ? lang : DEFAULT_LANG;
}

/**
 * Turkish -> English for short labels (paradigm names, trial/parameter/zone/metric
 * labels). Because the same Turkish label always means the same concept, a single
 * flat map is safe and collision-free.
 */
const LABELS_EN = Object.freeze({
  // Paradigm names
  "Morris Su Tankı": "Morris Water Maze",
  "Açık Alan": "Open Field",
  "Yükseltilmiş Artı Labirent": "Elevated Plus Maze",
  Rotarod: "Rotarod",
  "Y Labirenti": "Y Maze",
  "Yeni Nesne Tanıma": "Novel Object Recognition",
  "Barnes Labirenti": "Barnes Maze",
  "Üç Bölmeli Sosyallık": "Three-Chamber Sociability",
  "Aydınlık/Karanlık Kutu": "Light/Dark Box",
  "Çubuk (Pole) Testi": "Pole Test",
  "Koşu Bandı (Treadmill)": "Treadmill",

  // Trial labels
  "Öğrenme (platform var)": "Acquisition (platform present)",
  "Prob (platform yok)": "Probe (no platform)",
  Standart: "Standard",
  "Sabit hız": "Fixed speed",
  Hizlanan: "Accelerating",
  "Spontan değişim": "Spontaneous alternation",
  "Yeni kol (2 denemeli)": "Novel arm (two trials)",
  Alistirma: "Familiarization",
  "Test (yeni nesne)": "Test (novel object)",
  Ogrenme: "Acquisition",
  "Prob (kaçış kutusu yok)": "Probe (no escape box)",
  Sosyallık: "Sociability",
  "Sosyal yenilik": "Social novelty",
  "Dayanıklılık (hızlanan)": "Endurance (accelerating)",

  // Apparatus / session parameter labels
  "Tank çapı": "Tank diameter",
  "Platform çapı": "Platform diameter",
  "Platform merkezi X": "Platform center X",
  "Platform merkezi Y": "Platform center Y",
  "Platform çeyreği": "Platform quadrant",
  "Duvar halkası genişliği": "Wall annulus width",
  "Su opaklığı": "Water opacity",
  "Su sıcaklığı": "Water temperature",
  "Maks deneme süresi": "Max trial duration",
  "Başlangıç konumu": "Start position",
  "Deneme indeksi": "Trial index",
  "Arena genişliği": "Arena width",
  "Arena derinliği": "Arena depth",
  "Merkez oranı": "Center fraction",
  "Hareketsizlik eşiği": "Immobility threshold",
  "Kol uzunluğu": "Arm length",
  "Kol genişliği": "Arm width",
  "Merkez kare kenarı": "Center square side",
  "Kapalı kol duvar yüksekliği": "Closed arm wall height",
  "Çubuk çapı": "Rod diameter",
  "Min devir": "Min rpm",
  "Maks devir": "Max rpm",
  "Hızlanma süresi": "Acceleration duration",
  "Dönüş modu": "Rotation mode",
  "Kollar arası açı": "Angle between arms",
  "Nesne keşif yarıçapı": "Object exploration radius",
  "Yeni nesne konumu": "Novel object position",
  "Delik sayısı": "Hole count",
  "Delik çapı": "Hole diameter",
  "Hedef delik indeksi": "Target hole index",
  "Bölme genişliği": "Chamber width",
  "Bölme derinliği": "Chamber depth",
  "Etkileşim bölgesi yarıçapı": "Interaction zone radius",
  "Sosyal bölme tarafı": "Social chamber side",
  "Kutu genişliği": "Box width",
  "Kutu derinliği": "Box depth",
  "Aydınlık bölme oranı": "Light compartment fraction",
  "Başlangıç bölmesi": "Start compartment",
  "Çubuk uzunluğu": "Pole length",
  "Şerit uzunluğu": "Lane length",
  "Min bant hızı": "Min belt speed",
  "Maks bant hızı": "Max belt speed",
  Egim: "Incline",
  "Hız modu": "Speed mode",

  // Zone labels
  Platform: "Platform",
  "Hedef çeyrek": "Target quadrant",
  "Duvar halkası": "Wall annulus",
  Merkez: "Center",
  Cevre: "Periphery",
  "Açık kollar": "Open arms",
  "Kapalı kollar": "Closed arms",
  "A kolu": "Arm A",
  "B kolu": "Arm B",
  "C kolu": "Arm C",
  "Yeni kol": "Novel arm",
  "Yeni nesne": "Novel object",
  "Tanıdık nesne": "Familiar object",
  "Hedef delik": "Target hole",
  "Sosyal bölme": "Social chamber",
  "Nesne bölmesi": "Object chamber",
  "Orta bölme": "Center chamber",
  "Etkileşim bölgesi": "Interaction zone",
  "Aydınlık bölme": "Light compartment",
  "Karanlık bölme": "Dark compartment",
  Tepe: "Top",
  Taban: "Base",
  "Geri (uyarı) bölgesi": "Rear (warning) zone",

  // Metric labels
  "Analiz süresi": "Analyzed duration",
  "Toplam yol": "Total path",
  "Ortalama hız": "Mean speed",
  "Maksimum hız": "Maximum speed",
  "Hareketsizlik süresi": "Immobility time",
  "Bölge süresi": "Zone time",
  "Bölge girişleri": "Zone entries",
  "Bölgeye varış gecikmesi": "Latency to zone",
  "Yol verimliliği": "Path efficiency",
  "Kaçış gecikmesi": "Escape latency",
  "Yüzme yolu": "Swim path",
  "Ortalama yüzme hızı": "Mean swim speed",
  "Platform geçişleri": "Platform crossings",
  "Hedef çeyrek süresi oranı": "Target quadrant time ratio",
  "Çeyrek süresi": "Quadrant time",
  "Tigmotaksi süresi oranı": "Thigmotaxis time ratio",
  "Platforma ortalama mesafe": "Mean distance to platform",
  "Yönelim hatası": "Heading error",
  "Merkez süresi oranı": "Center time ratio",
  "Çevre süresi oranı": "Periphery time ratio",
  "Merkez girişleri": "Center entries",
  "Açık kol süresi oranı": "Open arm time ratio",
  "Kapalı kol süresi oranı": "Closed arm time ratio",
  "Açık kol girişleri": "Open arm entries",
  "Kapalı kol girişleri": "Closed arm entries",
  "Açık kola varış gecikmesi": "Latency to open arm",
  "Risk değerlendirme sayısı": "Risk assessment count",
  "Düşme gecikmesi": "Latency to fall",
  "Düşme anındaki devir": "Rpm at fall",
  "Deneme süresi": "Trial duration",
  "Düşme algılandı": "Fall detected",
  "Öğrenme eğimi": "Learning slope",
  "Spontan değişim oranı": "Spontaneous alternation ratio",
  "Toplam kol girişi": "Total arm entries",
  "Yeni kol süresi oranı": "Novel arm time ratio",
  "Yeni nesne keşif süresi": "Novel object exploration time",
  "Tanıdık nesne keşif süresi": "Familiar object exploration time",
  "Ayrım indeksi": "Discrimination index",
  "Toplam keşif süresi": "Total exploration time",
  "Birincil gecikme": "Primary latency",
  "Birincil hata": "Primary errors",
  "Toplam hata": "Total errors",
  "Sosyal bölme süresi": "Social chamber time",
  "Nesne bölmesi süresi": "Object chamber time",
  "Sosyallık indeksi": "Sociability index",
  "Yakın etkileşim süresi": "Close interaction time",
  "Aydınlık bölme süresi oranı": "Light compartment time ratio",
  "Aydınlık bölme girişleri": "Light compartment entries",
  "Karanlığa giriş gecikmesi": "Latency to enter dark",
  "Bölme geçişleri": "Compartment transitions",
  "Dönme süresi": "Turn time",
  "Toplam iniş süresi": "Total descent time",
  "İniş hızı": "Descent speed",
  "Koşu süresi": "Run time",
  "Koşu mesafesi": "Run distance",
  "Bitkinlik gecikmesi": "Latency to exhaustion",
  "Uyarı/şok sayısı": "Warning/shock count",

  // Event type labels
  "Bölgeye giriş": "Zone enter",
  "Bölgeden çıkış": "Zone exit",
  Hareketsizlik: "Immobility",
  "Risk değerlendirmesi": "Risk assessment",
  "Aydınlık/karanlık geçişi": "Light/dark transition",
  "Platforma ulaşma": "Platform reached",
  "Platform geçişi": "Platform crossing",
  "Nesne etkileşimi": "Object interaction",
  "Hedef deliğe ulaşma": "Target hole reached",
  "Hata (delik)": "Error (hole)",
  Etkilesim: "Interaction",
  Dusme: "Fall",
  Sok: "Shock",
  Tukenme: "Exhaustion",
});

/** Turkish -> English for metric definitions (long descriptions). */
const DEFINITIONS_EN = Object.freeze({
  "Geçersiz kareler kırpıldıktan sonra analiz edilen zaman penceresi.":
    "Analyzed time window after invalid frames are trimmed.",
  "Apparatus koordinatlarında toplam yol uzunluğu.":
    "Total path length in apparatus coordinates.",
  "Analiz penceresi boyunca ortalama hareket hızı.":
    "Mean movement speed over the analyzed window.",
  "Yumuşatılmış anlık hızın maksimumu.":
    "Maximum of the smoothed instantaneous speed.",
  "Paradigmaya özgü hareket eşiğinin altında geçirilen süre.":
    "Time spent below the paradigm-specific movement threshold.",
  "Tanımlı bir bölge içinde geçirilen süre (anahtar: zone_time_s.{zoneKey}).":
    "Time spent inside a declared zone (key: zone_time_s.{zoneKey}).",
  "Debounce sonrası bir bölgeye giriş sayısı (anahtar: zone_entries.{zoneKey}).":
    "Number of entries into a zone after debounce (key: zone_entries.{zoneKey}).",
  "Deneme başlangıcından bir bölgeye ilk geçerli girişe kadar geçen süre (anahtar: latency_to_zone_s.{zoneKey}).":
    "Time from trial start to first valid entry into a zone (key: latency_to_zone_s.{zoneKey}).",
  "Hedefe düz çizgi mesafesinin gerçek yol uzunluğuna oranı.":
    "Straight-line distance to target divided by actual path length.",
  "İlk sürekli platform-bölgesi girişine kadar geçen süre.":
    "Time to the first sustained platform-zone entry.",
  "Platforma veya deneme sonuna kadar toplam yüzme yolu.":
    "Total swim path until the platform or trial end.",
  "Öğrenme etkisini motor bozukluktan ayırmak için kullanılır.":
    "Used to separate learning effects from motor impairment.",
  "Sadece prob denemeleri: eski platform bölgesinden geçiş sayısı.":
    "Probe trials only: number of crossings through the former platform zone.",
  "Hedef çeyrekte geçen sürenin geçerli analiz süresine oranı.":
    "Time in the target quadrant divided by the valid analyzed duration.",
  "Bir çeyrekte geçen süre (anahtar: quadrant_time_s.{quadrant}, ör. NE/NW/SE/SW).":
    "Time spent in a quadrant (key: quadrant_time_s.{quadrant}, e.g. NE/NW/SE/SW).",
  "Tank duvarına yakın halka bölgede geçen süre oranı.":
    "Ratio of time spent in the annulus near the tank wall.",
  "Hedefe ortalama yakınlık; prob denemelerinde sağlam bir ölçüt.":
    "Mean proximity to the target; a robust measure for probe trials.",
  "Opsiyonel; heading veya yumuşatılmış yol vektörü gerektirir.":
    "Optional; requires heading or a smoothed path vector.",
  "Merkez bölgede geçen sürenin geçerli süreye oranı.":
    "Time in the center zone divided by the valid duration.",
  "Çevre bölgede geçen sürenin geçerli süreye oranı.":
    "Time in the periphery zone divided by the valid duration.",
  "Debounce sonrası merkez bölge girişleri.":
    "Center-zone entries after debounce.",
  "Açık kollarda geçen sürenin geçerli süreye oranı.":
    "Time in the open arms divided by the valid duration.",
  "Kapalı kollarda geçen sürenin geçerli süreye oranı.":
    "Time in the closed arms divided by the valid duration.",
  "Debounce sonrası açık kol girişleri.": "Open-arm entries after debounce.",
  "Debounce sonrası kapalı kol girişleri.": "Closed-arm entries after debounce.",
  "Bir açık kola ilk giriş süresi.": "Time of first entry into an open arm.",
  "Opsiyonel; davranış sınıflandırıcı varsa olay sayısı.":
    "Optional; event count when a behavior classifier is available.",
  "Deneme başlangıcından düşme olayına kadar geçen süre.":
    "Time from trial start to the fall event.",
  "Mod ve geçen süreden türetilir.": "Derived from the mode and the elapsed time.",
  "Denek düşmezse maksimum süreye eşit olabilir.":
    "May equal the maximum duration if the subject does not fall.",
  "Bir düşme olayının algılanıp algılanmadığı.":
    "Whether a fall event was detected.",
  "Tekrarlı denemeler arasında çalışma seviyesinde toplanır; tek deneme CV metriği değildir.":
    "Aggregated at study level across repeated trials; not a single-trial CV metric.",
  "Ardışık üçlü kol dizilerindeki doğru değişim oranı.":
    "Rate of correct alternations in consecutive arm triplets.",
  "Tüm kollara toplam giriş sayısı; lokomotor aktivite göstergesi.":
    "Total entries into all arms; an indicator of locomotor activity.",
  "İki denemeli protokolde yeni kolda geçen süre oranı.":
    "Ratio of time spent in the novel arm in the two-trial protocol.",
  "Yeni nesneyi aktif keşfetme süresi (burun nesneye yönelik).":
    "Active exploration time of the novel object (nose oriented toward the object).",
  "Tanıdık nesneyi aktif keşfetme süresi.":
    "Active exploration time of the familiar object.",
  "Tanıma belleği ölçütü; -1 (tanıdık) ile +1 (yeni) arasında.":
    "Recognition memory measure; between -1 (familiar) and +1 (novel).",
  "Her iki nesneyi keşfetme süresinin toplamı.":
    "Sum of exploration time for both objects.",
  "Hedef deliğe ilk ulaşma süresi.": "Time to first reach the target hole.",
  "Hedef deliğe ulaşmadan önce yapılan yanlış delik ziyaretleri.":
    "Wrong hole visits before reaching the target hole.",
  "Deneme boyunca toplam yanlış delik ziyareti.":
    "Total wrong hole visits during the trial.",
  "Uyaran fareyi içeren bölmede geçen süre.":
    "Time spent in the chamber containing the stimulus mouse.",
  "Boş kafes/nesne bulunan bölmede geçen süre.":
    "Time spent in the chamber with the empty cage/object.",
  "Sosyal tercih ölçütü; -1 (nesne) ile +1 (sosyal) arasında.":
    "Social preference measure; between -1 (object) and +1 (social).",
  "Uyaran kafesi etrafındaki etkileşim bölgesinde geçen süre.":
    "Time spent in the interaction zone around the stimulus cage.",
  "Aydınlık bölmede geçen sürenin geçerli süreye oranı.":
    "Time in the light compartment divided by the valid duration.",
  "Debounce sonrası aydınlık bölmeye giriş sayısı.":
    "Number of entries into the light compartment after debounce.",
  "Aydınlık başlangıçtan karanlık bölmeye ilk giriş süresi.":
    "Time of first entry into the dark compartment from a light start.",
  "Aydınlık ve karanlık bölmeler arası toplam geçiş sayısı.":
    "Total number of transitions between the light and dark compartments.",
  "Tepede aşağı dönmeyi tamamlama süresi.":
    "Time to complete the downward turn at the top.",
  "Tabana ulaşana kadar geçen toplam süre.":
    "Total time until reaching the base.",
  "Ortalama dikey iniş hızı.": "Mean vertical descent speed.",
  "Bitkinlik veya deneme sonuna kadar aktif koşu süresi.":
    "Active running time until exhaustion or trial end.",
  "Bant hızı ve koşu süresinden türetilen toplam mesafe.":
    "Total distance derived from belt speed and run time.",
  "Bitkinlik kriterine ulaşana kadar geçen süre.":
    "Time until the exhaustion criterion is reached.",
  "Bitkinlik kriteri olarak sayılan geri bölge temas/uyarı sayısı.":
    "Count of rear-zone contacts/warnings counted as the exhaustion criterion.",
});

/** Localizes a short label (falls back to the source text if missing). */
export function tLabel(label, lang) {
  if (lang === DEFAULT_LANG || label == null) return label;
  return LABELS_EN[label] ?? label;
}

/** Localizes a metric definition (falls back to the source text if missing). */
export function tDefinition(definition, lang) {
  if (lang === DEFAULT_LANG || definition == null) return definition;
  return DEFINITIONS_EN[definition] ?? definition;
}

/** Localizes a single metric definition (label + definition). */
export function localizeMetricDef(m, lang) {
  if (lang === DEFAULT_LANG) return m;
  return { ...m, label: tLabel(m.label, lang), definition: tDefinition(m.definition, lang) };
}

/** Localizes a metric list. */
export function localizeMetricList(list, lang) {
  return lang === DEFAULT_LANG ? list : list.map((m) => localizeMetricDef(m, lang));
}

/** Localizes a paradigm summary (name + trialTypes). */
export function localizeSummary(s, lang) {
  if (lang === DEFAULT_LANG) return s;
  return {
    ...s,
    name: tLabel(s.name, lang),
    trialTypes: s.trialTypes.map((t) => ({ ...t, label: tLabel(t.label, lang) })),
  };
}

/**
 * Localizes a paradigm detail. `spec` must already be a plain object with zones
 * resolved as an array (prepared in the route layer).
 */
export function localizeDetail(spec, lang) {
  if (lang === DEFAULT_LANG) return spec;
  const mapLabels = (arr) => (arr || []).map((x) => ({ ...x, label: tLabel(x.label, lang) }));
  return {
    ...spec,
    name: tLabel(spec.name, lang),
    trialTypes: mapLabels(spec.trialTypes),
    apparatusParameters: mapLabels(spec.apparatusParameters),
    sessionParameters: mapLabels(spec.sessionParameters),
    zones: mapLabels(spec.zones),
    metrics: localizeMetricList(spec.metrics, lang),
    eventTypes: mapLabels(spec.eventTypes),
  };
}
