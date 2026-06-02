/**
 * Bilimsel sozluk lokalizasyonu (Step 2).
 *
 * Paradigma/metrik spec'leri Turkce etiket ve tanimlari satir-ici tasir
 * (varsayilan dil). Bu modul, salt-okunur inceleme API'sinin `?lang=` ile
 * lokalize etiket dondurmesini saglar: `tr` icin kaynak metin oldugu gibi,
 * `en` icin asagidaki katalogdan cevirisi (eksikse Turkce'ye geri duser).
 *
 * Tek kaynak ilkesi: yapinin sahibi backend kalir; frontend yalnizca kucuk
 * sabit enum'lari (zone type/role, species) ceviren UI sozlugunu tutar.
 */
export const SUPPORTED_LANGS = Object.freeze(["tr", "en"]);
export const DEFAULT_LANG = "tr";

/** Gecersiz/eksik dili varsayilana indirger. */
export function normalizeLang(lang) {
  return SUPPORTED_LANGS.includes(lang) ? lang : DEFAULT_LANG;
}

/**
 * Kisa etiketler (paradigma adlari, trial/parametre/bolge/metrik etiketleri)
 * icin Turkce -> Ingilizce. Ayni Turkce etiket her zaman ayni kavrami ifade
 * ettigi icin tek bir duz harita guvenli ve cakismasizdir.
 */
const LABELS_EN = Object.freeze({
  // Paradigma adlari
  "Morris Su Tanki": "Morris Water Maze",
  "Acik Alan": "Open Field",
  "Yukseltilmis Arti Labirent": "Elevated Plus Maze",
  Rotarod: "Rotarod",
  "Y Labirenti": "Y Maze",
  "Yeni Nesne Tanima": "Novel Object Recognition",
  "Barnes Labirenti": "Barnes Maze",
  "Uc Bolmeli Sosyallik": "Three-Chamber Sociability",
  "Aydinlik/Karanlik Kutu": "Light/Dark Box",
  "Cubuk (Pole) Testi": "Pole Test",
  "Kosu Bandi (Treadmill)": "Treadmill",

  // Trial etiketleri
  "Ogrenme (platform var)": "Acquisition (platform present)",
  "Prob (platform yok)": "Probe (no platform)",
  Standart: "Standard",
  "Sabit hiz": "Fixed speed",
  Hizlanan: "Accelerating",
  "Spontan degisim": "Spontaneous alternation",
  "Yeni kol (2 denemeli)": "Novel arm (two trials)",
  Alistirma: "Familiarization",
  "Test (yeni nesne)": "Test (novel object)",
  Ogrenme: "Acquisition",
  "Prob (kacis kutusu yok)": "Probe (no escape box)",
  Sosyallik: "Sociability",
  "Sosyal yenilik": "Social novelty",
  "Dayaniklilik (hizlanan)": "Endurance (accelerating)",

  // Apparatus / oturum parametre etiketleri
  "Tank capi": "Tank diameter",
  "Platform capi": "Platform diameter",
  "Platform merkezi X": "Platform center X",
  "Platform merkezi Y": "Platform center Y",
  "Platform ceyregi": "Platform quadrant",
  "Duvar halkasi genisligi": "Wall annulus width",
  "Su opakligi": "Water opacity",
  "Su sicakligi": "Water temperature",
  "Maks deneme suresi": "Max trial duration",
  "Baslangic konumu": "Start position",
  "Deneme indeksi": "Trial index",
  "Arena genisligi": "Arena width",
  "Arena derinligi": "Arena depth",
  "Merkez orani": "Center fraction",
  "Hareketsizlik esigi": "Immobility threshold",
  "Kol uzunlugu": "Arm length",
  "Kol genisligi": "Arm width",
  "Merkez kare kenari": "Center square side",
  "Kapali kol duvar yuksekligi": "Closed arm wall height",
  "Cubuk capi": "Rod diameter",
  "Min devir": "Min rpm",
  "Maks devir": "Max rpm",
  "Hizlanma suresi": "Acceleration duration",
  "Donus modu": "Rotation mode",
  "Kollar arasi aci": "Angle between arms",
  "Nesne kesif yaricapi": "Object exploration radius",
  "Yeni nesne konumu": "Novel object position",
  "Delik sayisi": "Hole count",
  "Delik capi": "Hole diameter",
  "Hedef delik indeksi": "Target hole index",
  "Bolme genisligi": "Chamber width",
  "Bolme derinligi": "Chamber depth",
  "Etkilesim bolgesi yaricapi": "Interaction zone radius",
  "Sosyal bolme tarafi": "Social chamber side",
  "Kutu genisligi": "Box width",
  "Kutu derinligi": "Box depth",
  "Aydinlik bolme orani": "Light compartment fraction",
  "Baslangic bolmesi": "Start compartment",
  "Cubuk uzunlugu": "Pole length",
  "Serit uzunlugu": "Lane length",
  "Min bant hizi": "Min belt speed",
  "Maks bant hizi": "Max belt speed",
  Egim: "Incline",
  "Hiz modu": "Speed mode",

  // Bolge (zone) etiketleri
  Platform: "Platform",
  "Hedef ceyrek": "Target quadrant",
  "Duvar halkasi": "Wall annulus",
  Merkez: "Center",
  Cevre: "Periphery",
  "Acik kollar": "Open arms",
  "Kapali kollar": "Closed arms",
  "A kolu": "Arm A",
  "B kolu": "Arm B",
  "C kolu": "Arm C",
  "Yeni kol": "Novel arm",
  "Yeni nesne": "Novel object",
  "Tanidik nesne": "Familiar object",
  "Hedef delik": "Target hole",
  "Sosyal bolme": "Social chamber",
  "Nesne bolmesi": "Object chamber",
  "Orta bolme": "Center chamber",
  "Etkilesim bolgesi": "Interaction zone",
  "Aydinlik bolme": "Light compartment",
  "Karanlik bolme": "Dark compartment",
  Tepe: "Top",
  Taban: "Base",
  "Geri (uyari) bolgesi": "Rear (warning) zone",

  // Metrik etiketleri
  "Analiz suresi": "Analyzed duration",
  "Toplam yol": "Total path",
  "Ortalama hiz": "Mean speed",
  "Maksimum hiz": "Maximum speed",
  "Hareketsizlik suresi": "Immobility time",
  "Bolge suresi": "Zone time",
  "Bolge girisleri": "Zone entries",
  "Bolgeye varis gecikmesi": "Latency to zone",
  "Yol verimliligi": "Path efficiency",
  "Kacis gecikmesi": "Escape latency",
  "Yuzme yolu": "Swim path",
  "Ortalama yuzme hizi": "Mean swim speed",
  "Platform gecisleri": "Platform crossings",
  "Hedef ceyrek suresi orani": "Target quadrant time ratio",
  "Ceyrek suresi": "Quadrant time",
  "Tigmotaksi suresi orani": "Thigmotaxis time ratio",
  "Platforma ortalama mesafe": "Mean distance to platform",
  "Yonelim hatasi": "Heading error",
  "Merkez suresi orani": "Center time ratio",
  "Cevre suresi orani": "Periphery time ratio",
  "Merkez girisleri": "Center entries",
  "Acik kol suresi orani": "Open arm time ratio",
  "Kapali kol suresi orani": "Closed arm time ratio",
  "Acik kol girisleri": "Open arm entries",
  "Kapali kol girisleri": "Closed arm entries",
  "Acik kola varis gecikmesi": "Latency to open arm",
  "Risk degerlendirme sayisi": "Risk assessment count",
  "Dusme gecikmesi": "Latency to fall",
  "Dusme anindaki devir": "Rpm at fall",
  "Deneme suresi": "Trial duration",
  "Dusme algilandi": "Fall detected",
  "Ogrenme egimi": "Learning slope",
  "Spontan degisim orani": "Spontaneous alternation ratio",
  "Toplam kol girisi": "Total arm entries",
  "Yeni kol suresi orani": "Novel arm time ratio",
  "Yeni nesne kesif suresi": "Novel object exploration time",
  "Tanidik nesne kesif suresi": "Familiar object exploration time",
  "Ayrim indeksi": "Discrimination index",
  "Toplam kesif suresi": "Total exploration time",
  "Birincil gecikme": "Primary latency",
  "Birincil hata": "Primary errors",
  "Toplam hata": "Total errors",
  "Sosyal bolme suresi": "Social chamber time",
  "Nesne bolmesi suresi": "Object chamber time",
  "Sosyallik indeksi": "Sociability index",
  "Yakin etkilesim suresi": "Close interaction time",
  "Aydinlik bolme suresi orani": "Light compartment time ratio",
  "Aydinlik bolme girisleri": "Light compartment entries",
  "Karanliga giris gecikmesi": "Latency to enter dark",
  "Bolme gecisleri": "Compartment transitions",
  "Donme suresi": "Turn time",
  "Toplam inis suresi": "Total descent time",
  "Inis hizi": "Descent speed",
  "Kosu suresi": "Run time",
  "Kosu mesafesi": "Run distance",
  "Bitkinlik gecikmesi": "Latency to exhaustion",
  "Uyari/sok sayisi": "Warning/shock count",
});

/** Metrik tanimlari (uzun aciklamalar) icin Turkce -> Ingilizce. */
const DEFINITIONS_EN = Object.freeze({
  "Gecersiz kareler kirpildiktan sonra analiz edilen zaman penceresi.":
    "Analyzed time window after invalid frames are trimmed.",
  "Apparatus koordinatlarinda toplam yol uzunlugu.":
    "Total path length in apparatus coordinates.",
  "Analiz penceresi boyunca ortalama hareket hizi.":
    "Mean movement speed over the analyzed window.",
  "Yumusatilmis anlik hizin maksimumu.":
    "Maximum of the smoothed instantaneous speed.",
  "Paradigmaya ozgu hareket esiginin altinda gecirilen sure.":
    "Time spent below the paradigm-specific movement threshold.",
  "Tanimli bir bolge icinde gecirilen sure (anahtar: zone_time_s.{zoneKey}).":
    "Time spent inside a declared zone (key: zone_time_s.{zoneKey}).",
  "Debounce sonrasi bir bolgeye giris sayisi (anahtar: zone_entries.{zoneKey}).":
    "Number of entries into a zone after debounce (key: zone_entries.{zoneKey}).",
  "Deneme baslangicindan bir bolgeye ilk gecerli girise kadar gecen sure (anahtar: latency_to_zone_s.{zoneKey}).":
    "Time from trial start to first valid entry into a zone (key: latency_to_zone_s.{zoneKey}).",
  "Hedefe duz cizgi mesafesinin gercek yol uzunluguna orani.":
    "Straight-line distance to target divided by actual path length.",
  "Ilk surekli platform-bolgesi girisine kadar gecen sure.":
    "Time to the first sustained platform-zone entry.",
  "Platforma veya deneme sonuna kadar toplam yuzme yolu.":
    "Total swim path until the platform or trial end.",
  "Ogrenme etkisini motor bozukluktan ayirmak icin kullanilir.":
    "Used to separate learning effects from motor impairment.",
  "Sadece prob denemeleri: eski platform bolgesinden gecis sayisi.":
    "Probe trials only: number of crossings through the former platform zone.",
  "Hedef ceyrekte gecen surenin gecerli analiz suresine orani.":
    "Time in the target quadrant divided by the valid analyzed duration.",
  "Bir ceyrekte gecen sure (anahtar: quadrant_time_s.{quadrant}, or. NE/NW/SE/SW).":
    "Time spent in a quadrant (key: quadrant_time_s.{quadrant}, e.g. NE/NW/SE/SW).",
  "Tank duvarina yakin halka bolgede gecen sure orani.":
    "Ratio of time spent in the annulus near the tank wall.",
  "Hedefe ortalama yakinlik; prob denemelerinde saglam bir olcut.":
    "Mean proximity to the target; a robust measure for probe trials.",
  "Opsiyonel; heading veya yumusatilmis yol vektoru gerektirir.":
    "Optional; requires heading or a smoothed path vector.",
  "Merkez bolgede gecen surenin gecerli sureye orani.":
    "Time in the center zone divided by the valid duration.",
  "Cevre bolgede gecen surenin gecerli sureye orani.":
    "Time in the periphery zone divided by the valid duration.",
  "Debounce sonrasi merkez bolge girisleri.":
    "Center-zone entries after debounce.",
  "Acik kollarda gecen surenin gecerli sureye orani.":
    "Time in the open arms divided by the valid duration.",
  "Kapali kollarda gecen surenin gecerli sureye orani.":
    "Time in the closed arms divided by the valid duration.",
  "Debounce sonrasi acik kol girisleri.": "Open-arm entries after debounce.",
  "Debounce sonrasi kapali kol girisleri.": "Closed-arm entries after debounce.",
  "Bir acik kola ilk giris suresi.": "Time of first entry into an open arm.",
  "Opsiyonel; davranis siniflandirici varsa olay sayisi.":
    "Optional; event count when a behavior classifier is available.",
  "Deneme baslangicindan dusme olayina kadar gecen sure.":
    "Time from trial start to the fall event.",
  "Mod ve gecen sureden turetilir.": "Derived from the mode and the elapsed time.",
  "Denek dusmezse maksimum sureye esit olabilir.":
    "May equal the maximum duration if the subject does not fall.",
  "Bir dusme olayinin algilanip algilanmadigi.":
    "Whether a fall event was detected.",
  "Tekrarli denemeler arasinda calisma seviyesinde toplanir; tek deneme CV metrigi degildir.":
    "Aggregated at study level across repeated trials; not a single-trial CV metric.",
  "Ardisik uclu kol dizilerindeki dogru degisim orani.":
    "Rate of correct alternations in consecutive arm triplets.",
  "Tum kollara toplam giris sayisi; lokomotor aktivite gostergesi.":
    "Total entries into all arms; an indicator of locomotor activity.",
  "Iki denemeli protokolde yeni kolda gecen sure orani.":
    "Ratio of time spent in the novel arm in the two-trial protocol.",
  "Yeni nesneyi aktif kesfetme suresi (burun nesneye yonelik).":
    "Active exploration time of the novel object (nose oriented toward the object).",
  "Tanidik nesneyi aktif kesfetme suresi.":
    "Active exploration time of the familiar object.",
  "Tanima bellegi olcutu; -1 (tanidik) ile +1 (yeni) arasinda.":
    "Recognition memory measure; between -1 (familiar) and +1 (novel).",
  "Her iki nesneyi kesfetme suresinin toplami.":
    "Sum of exploration time for both objects.",
  "Hedef delige ilk ulasma suresi.": "Time to first reach the target hole.",
  "Hedef delige ulasmadan once yapilan yanlis delik ziyaretleri.":
    "Wrong hole visits before reaching the target hole.",
  "Deneme boyunca toplam yanlis delik ziyareti.":
    "Total wrong hole visits during the trial.",
  "Uyaran fareyi iceren bolmede gecen sure.":
    "Time spent in the chamber containing the stimulus mouse.",
  "Bos kafes/nesne bulunan bolmede gecen sure.":
    "Time spent in the chamber with the empty cage/object.",
  "Sosyal tercih olcutu; -1 (nesne) ile +1 (sosyal) arasinda.":
    "Social preference measure; between -1 (object) and +1 (social).",
  "Uyaran kafesi etrafindaki etkilesim bolgesinde gecen sure.":
    "Time spent in the interaction zone around the stimulus cage.",
  "Aydinlik bolmede gecen surenin gecerli sureye orani.":
    "Time in the light compartment divided by the valid duration.",
  "Debounce sonrasi aydinlik bolmeye giris sayisi.":
    "Number of entries into the light compartment after debounce.",
  "Aydinlik baslangictan karanlik bolmeye ilk giris suresi.":
    "Time of first entry into the dark compartment from a light start.",
  "Aydinlik ve karanlik bolmeler arasi toplam gecis sayisi.":
    "Total number of transitions between the light and dark compartments.",
  "Tepede asagi donmeyi tamamlama suresi.":
    "Time to complete the downward turn at the top.",
  "Tabana ulasana kadar gecen toplam sure.":
    "Total time until reaching the base.",
  "Ortalama dikey inis hizi.": "Mean vertical descent speed.",
  "Bitkinlik veya deneme sonuna kadar aktif kosu suresi.":
    "Active running time until exhaustion or trial end.",
  "Bant hizi ve kosu suresinden turetilen toplam mesafe.":
    "Total distance derived from belt speed and run time.",
  "Bitkinlik kriterine ulasana kadar gecen sure.":
    "Time until the exhaustion criterion is reached.",
  "Bitkinlik kriteri olarak sayilan geri bolge temas/uyari sayisi.":
    "Count of rear-zone contacts/warnings counted as the exhaustion criterion.",
});

/** Kisa etiketi lokalize eder (eksikse kaynak metne geri duser). */
export function tLabel(label, lang) {
  if (lang === DEFAULT_LANG || label == null) return label;
  return LABELS_EN[label] ?? label;
}

/** Metrik tanimini lokalize eder (eksikse kaynak metne geri duser). */
export function tDefinition(definition, lang) {
  if (lang === DEFAULT_LANG || definition == null) return definition;
  return DEFINITIONS_EN[definition] ?? definition;
}

/** Tek bir metrik tanimini lokalize eder (label + definition). */
export function localizeMetricDef(m, lang) {
  if (lang === DEFAULT_LANG) return m;
  return { ...m, label: tLabel(m.label, lang), definition: tDefinition(m.definition, lang) };
}

/** Metrik listesini lokalize eder. */
export function localizeMetricList(list, lang) {
  return lang === DEFAULT_LANG ? list : list.map((m) => localizeMetricDef(m, lang));
}

/** Paradigma ozetini lokalize eder (name + trialTypes). */
export function localizeSummary(s, lang) {
  if (lang === DEFAULT_LANG) return s;
  return {
    ...s,
    name: tLabel(s.name, lang),
    trialTypes: s.trialTypes.map((t) => ({ ...t, label: tLabel(t.label, lang) })),
  };
}

/**
 * Paradigma detayini lokalize eder. `spec` zaten zones'u dizi olarak cozulmus
 * duz nesne olmali (route katmaninda hazirlanir).
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
  };
}
