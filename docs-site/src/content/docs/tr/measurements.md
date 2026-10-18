---
title: Ölçüm mimarisi
description: Paradigma spec'leri, metrik tanımları, MWM normalizasyonu ve kalite kontrol.
---

Mişko'nun gevşek sonuç JSON'u değil, kararlı ölçüm kontratları tutması gerekir.
Her paradigma, CV servisinin çalışmalar ve laboratuvarlar arasında
karşılaştırılabilir sonuç üretebilmesi için parametrelerini, bölgelerini,
metriklerini, olay tiplerini (CV detect spec'leri ile) ve kalite
gereksinimlerini önceden tanımlamalıdır.

## Mimari ilke

Her sonuç dört katmandan geçer:

| Katman | Amaç |
|---|---|
| `ParadigmSpec` | Parametreler, bölgeler, metrikler, olay tipleri (CV detect spec'leri ile) ve sonuç şeması için kod sahipli kontrat. |
| `Apparatus` | Lab sahipli fiziksel düzenek tanımı: geometri, malzeme, yüzey ve cm cinsinden bölgeler. |
| `Calibration` | Sabit düzenek veya tek test oturumu için pikselden cm'ye eşleme. |
| `MetricDefinition` | Her metrik anahtarının birim, formül, normalizasyon ve QC bağımlılığını tanımlayan kod sahipli sözlük. |

CV deneği takip eder. Bilimsel kontratı Mişko tanımlar.

## Paradigmalar şablondur

Paradigma, kodda tanımlı ve sürümlenen bir şablondur. Ortamlar bu şablondan
oluşturulur, protokol adımları da bu ortamlara bağlanır. Her parametrenin
nerede girildiğini söyleyen bir kapsamı vardır:

| Kapsam | Girildiği yer | Zorunlu mu | Örnekler |
|---|---|---|---|
| `APPARATUS` | Ortam | Evet, fiziksel düzenekte ölçülür | arena genişliği, tank çapı, direk uzunluğu |
| `ANALYSIS` | Ortam | Hayır, varsayılanlar çoğu düzenek için uygundur | duvar kenarı bölge genişliği, kaçış için platformda kalma süresi, en az toplam keşif süresi |
| `SESSION` | Protokol adımı | Hayır, varsayılan kullanılır | deneme süresi sınırı, koşu bandı başlangıç ve bitiş hızı |

Her parametrenin bir etiketi ve neyin ölçüleceğini ya da değerin neyi
değiştirdiğini anlatan bir yardım metni vardır. Adlar, etiketler, yardım
metinleri, bölge ve olay adları, metriklerin sade açıklamaları ve teknik
tanımlar (metrik tanımları, eksik veri kuralları, bölge geometrisi, olay
tanımları) desteklenen her dilde (İngilizce ve Türkçe) tanımın hemen yanında
yazılır; API bunları panelin istediği dilde döner (`Accept-Language` ya da
`?lang=tr`). Formüller düzyazı değil ifade oldukları için olduğu gibi kalır.
Yayınlanmış manifestler İngilizce olarak sabitlendiğinden bir çeviri
kontratı fark edilmeden değiştiremez. Panel oranları yüzde, açık/kapalı
değerleri anahtar olarak gösterir.

```ts
interface ParadigmManifest {
  key: string;
  name: string;           // yanıt dilinde
  description: string;    // testin ne olduğu ve neyi ölçtüğü
  category: "LEARNING_MEMORY" | "MOTOR";
  trialTypes: { key: string; label: string }[];
  parameters: {
    key: string;
    scope: "APPARATUS" | "ANALYSIS" | "SESSION";
    label: string;
    help: string;
    unit: "cm" | "cm_s" | "s" | "count" | "ratio" | "boolean";
    valueType: "number" | "integer";
    min: number;
    max: number;
    default: number;
  }[];
  zones: { key: string; label: string; role: string; geometry: string }[];
  metrics: { key: string; label: string; description: string; unit: string;
             definition: string; formula: string; missingWhen: string }[];
  qc: { key: string; operator: string; value: number; unit: string }[];
}
```

Bir hesaplamayı, aralığı ya da varsayılanı değiştirmek yeni bir paradigma
sürümü yayımlar. İfadeler ve yardım metinleri yeni sürüm gerektirmeden
iyileştirilebilir.

## Her paradigmanın otomatik analizi

Her paradigma videodan otomatik olarak analiz edilir; bir kişi yalnızca sonucu
inceler. İşçi deneğin ağırlık merkezini kalibrasyonun santimetre çerçevesinde
izler ve `backend/internal/paradigms/domain` içindeki metrik motoru her olayı bu
örneklerden ve bölgelerden çıkarır. Olaylara asla işçi karar vermez; böylece
bir ölçümün, deneği neyin izlediğinden bağımsız olarak tek bir tanımı vardır.

| Paradigma | Kamera ve dünya çerçevesi | Bölgeler | Çıkarılan olaylar |
|---|---|---|---|
| `MWM` | tankın üstünde; tank merkezli cm, x doğu, y kuzey | parametrelerden (platform, hedef çeyrek, duvar kenarı bölgesi) | yörüngenin bölge girişleri |
| `NOVEL_OBJECT` | arenanın üstünde; zeminin başlangıç köşesi, x genişlik, y derinlik boyunca | kalibre edilen `novel_object`, `familiar_object` (her nesnenin çevresinde, ondan yaklaşık 2 cm taşan bir daire ya da çokgen) | ağırlık merkezi o bölgede en az `min_bout_s` kaldığı sürece `novel` ya da `familiar` etiketli `exploration` aralıkları |
| `POLE` | direğin yanında; x yatay, y tabandan yükseklik (taban 0, tepe `pole_length_cm`) | kalibre edilen `pole`, `base` (çokgen) | `turn_complete`, `base_reached`, `fall` (anlar) |
| `TREADMILL` | kulvarın yanında; x kulvar boyunca şok ızgarası ucundan (0) öne (`lane_length_cm`), y dikey | kalibre edilen `shock_grid` (çokgen) | `shock` (anlar), `exhaustion` (an) |

Testin videosu doğrulandığında, deneğin kalibrasyonu her zorunlu bölgesiyle
geçerli olduğunda ve etkin bir işçinin yetenekleri arasında paradigma sürümü
bulunduğunda jobs süreci denek başına bir `AUTOMATED` çalıştırma kuyruğa ekler.
İşçi, kalibrasyonun santimetre çerçevesinde bir yörünge yükler; API ondan asla
metrik ya da olay kabul etmez. Her kural eşiği (`min_bout_s`,
`turn_descent_cm`, `fall_speed_cm_s`, `exhaustion_dwell_s`,
`min_shock_dwell_s`, `max_sample_gap_s`) standart bir varsayılanı olan ve
laboratuvarın ortam başına ayarlayabildiği bir analiz parametresidir.

Yeni nesne testinin bilinen sınırı: keşif, burnun yönüyle değil, gövde
merkezinin bir nesne bölgesinin içinde olmasıyla yaklaşık olarak ölçülür; bu
yüzden nesnenin üzerinde oturmak da keşif sayılır.

Her çalıştırma denek başına sonucu besler; bu sonuç insan
onayı için `PENDING` olarak açılır ve yalnızca kabul edilen sonuçlar rapora
girer.

### Bölge gereksinimleri

Bölgeler iki yerden birinden gelir:

- **Parametrelerden.** Su labirenti bölgelerini ortamın ölçülerinden türetir
  (platform konumu ve çapı, duvar kenarı bölge genişliği). Kalibrasyonunun
  dünya noktaları bu bölgelerin tanımlandığı çerçeveyi kullanmalıdır:
  **tank merkezli** çerçeve ((0, 0) tankın merkezi, x doğu, y kuzey; duvar
  noktaları r = tank çapı / 2 olmak üzere (r, 0), (0, r), (-r, 0) ve (0, -r)).
- **Kalibre edilen.** Yeni nesne, direk ve koşu bandı paradigmaları kalibre
  edilen bölgeler tanımlar (nesneler, direk ve tabanı, şok ızgarası). Bu
  bölgeler kalibrasyonla birlikte kamera karesi üzerine daire ya da çokgen
  olarak çizilir ve bir anahtarın birden çok şekli olabilir. Bir deneğin test
  videosundaki geçersiz kılma kalibrasyonu bunları taşıyabilir, örneğin yeni
  nesne öbür taraftaysa; kalibratörün yer değiştirme düğmesi iki nesne
  bölgesini değiştirir. Şekiller çizildiği gibi piksel olarak
  saklanır ve kalibrasyonun uydurulan dönüşümüyle santimetreye çevrilir; karenin
  içinde kalmaları gerekir ve kalibrasyon kaydedildikten sonra asla değişmezler
  (`014_calibration_zones.sql` migrasyonu). Her zorunlu bölgesi olan geçerli bir
  kalibrasyon uygulanana kadar denek `WAITING_FOR_CALIBRATION` durumunda kalır
  ve analiz edilmez. İşçi takibi bölgelerin dışbükey zarfıyla (bir payla)
  sınırlar; böylece bölgeler arasında kalan ve hiçbir bölgeye ait olmayan
  alanlar da takip edilir.

### Çıkarımlı örnekler

`misko.trajectory.v1` her örnek için isteğe bağlı bir `inferred` alanı taşır.
Bir worker, deneğin gizli olduğu bir kare dizisini `tracked` ve
`inferred: true` olarak işaretlenmiş örneklerle doldurabilir; bu örnekler
takip edilmiş sayılır ama takip güveni ortalamasına girmez. Güncel katalogdaki
hiçbir paradigma buna ihtiyaç duymaz ve paketle gelen worker çıkarımlı örnek
göndermez; tespitin olmadığı kareler kayıp olarak kalır.

## Geçti/kaldı verdikti yok

Bir lab test sonucu **veridir, verdikt değil**. Mişko davranışsal bir `passed`
bayrağı hesaplamaz ve bir kabul kriteri katmanı yoktur. Bir değerin "iyi" olup
olmadığı yorumdur, kayıt sistemine değil analiz katmanına aittir. Yalnızca QC
(takip güvenilirliği) bir geçti/kaldı anlamı taşır, o da davranışla değil veri
kalitesiyle ilgilidir.

## Olay tipleri ve CV tespiti

Bir koşu **olaylar** toplar; metrikler bu olaylardan türetilir (bkz.
`docs/METRIC_ENGINE.md`). Aynı olay akışı izlenen yörüngeden ve bölgelerden
çıkarılır, yani olay katmanı Mişko ile bilgisayarlı görü arasındaki
kontrattır.

Her olay tipi, CV servisinin onu trajectory ve bölgelerden nasıl türettiğini
tanımlayan bir `detect` spec'i bildirir:

```ts
interface EventTypeDef {
  type: string;                       // örn. "zone_enter", "immobile", "fall"
  label: string;
  payload?: Record<string, "number" | "text" | "zone">;
  detect: DetectSpec;
}

type DetectSpec =
  | { kind: "zone_transition"; edge: "enter" | "exit"; zone?: string }
  | { kind: "zone_first_enter"; zone: string }
  | { kind: "speed_below"; threshold_cm_s: number; min_duration_s?: number }
  | { kind: "custom" };               // özel bir dedektör/model gerektirir
```

Jenerik türler (`zone_transition`, `zone_first_enter`, `speed_below`)
**veri-güdümlüdür**: yalnızca bunları kullanan bir paradigma yeni CV kodu
gerektirmez, olay tiplerini bildirmek yeterlidir. `custom`, özel bir dedektör
gerektiren bir olayı işaretler (örn. direk testinde dönüş, nesne keşfi).
Olay tipleri paradigma detay API'sinde sunulur
(`GET /api/paradigms/:key` -> `eventTypes`).

## Lokalizasyon

Katalog metinleri `backend/internal/paradigms/domain` altındaki paradigma
tanımlarında, İngilizce ve Türkçe yan yana yazılır
(`T("Arena width", "Arena genişliği")`). `GET /api/paradigms`,
`GET /api/paradigms/{key}` ve `GET /api/paradigms/{key}/versions/{version}`,
`?lang=` (`en` ya da `tr`) değerinin, yoksa `Accept-Language` başlığının, o da
yoksa İngilizcenin dilinde yanıt verir ve `Content-Language` başlığını
ayarlar. Panel seçili dili her istekte gönderir. Bir ad, etiket, yardım metni
ya da metrik açıklaması herhangi bir dilde eksikse, anahtar gibi görünüyorsa ya
da uzun tire içeriyorsa test başarısız olur. Küçük sabit enum'lar (birimler,
kategoriler, kapsamlar, kalite kuralı adları) panelin kendi sözlüğünde çevrilir.

## Ölçüm sözlüğü

Metrik sözlüğü, iki servisin aynı ismi farklı hesaplar için kullanmasını engeller.

```ts
interface MetricDefinition {
  key: string;
  paradigmKeys: string[];
  unit: "cm" | "cm_s" | "s" | "count" | "ratio" | "percent" | "deg" | "boolean";
  valueType: "number" | "integer" | "boolean" | "object";
  required: boolean;
  definition: string;
  formula: string;
  inputs: string[];
  normalization: NormalizationDef | null;
  aggregation: "per_trial" | "per_session" | "per_subject_timepoint" | "study_summary";
  qcDependencies: string[];
}
```

Kanonik sonuç birimleri: pozisyon ve mesafe `cm`, hız `cm_s`, süre `s`, ağırlık
`g`, açı `deg`, oranlar `ratio` (`0..1`) veya `percent`, olay
bayrakları `boolean`, sayımlar `count`. Apparatus parametreleri bu enum'ı
yeniden kullanır ve sonuç metriklerinde hiç görünmeyen `mm` (küçük düzenek
çapları) ile `c` (Celsius cinsinden su sıcaklığı) birimlerini ekler. Piksel
değerleri Mişko sonuç metriklerine girmez.

## Temel metrikler

| Key | Birim | Tanım |
|---|---|---|
| `duration_s` | `s` | Geçersiz kareler kırpıldıktan sonra analiz edilen zaman penceresi. |
| `distance_cm` | `cm` | Apparatus koordinatlarında toplam yol uzunluğu. |
| `mean_speed_cm_s` | `cm_s` | Mesafenin süreye bölümü. |
| `zone_time_s.{zoneKey}` | `s` | Tanımlı bir bölgede geçirilen süre. |
| `zone_entries.{zoneKey}` | `count` | Tanımlı bölgeye debounce edilmiş giriş sayısı. |
| `latency_to_zone_s.{zoneKey}` | `s` | Trial başlangıcından ilk geçerli bölge girişine kadar geçen süre. |
| `path_efficiency_ratio` | `ratio` | Hedefe düz çizgi mesafesinin gerçek yola oranı. |

## Paradigma metrikleri

| Paradigma | Ana metrikler |
|---|---|
| MWM | Escape latency, path length, swim speed, target quadrant time, platform crossings, thigmotaxis, mean distance to platform. |
| Yeni nesne | Yeni nesne keşif süresi, tanıdık nesne keşif süresi, toplam keşif süresi, ayırt etme indeksi. |
| Direk | Dönme süresi, toplam süre, iniş süresi, iniş hızı, düşme. |
| Koşu bandı | Tükenme latansı, koşu süresi, koşu mesafesi, şok sayısı. |

## MWM normalizasyonu

Morris Su Tankı laboratuvarlar arasında ciddi farklılık gösterir. Karşılaştırmalı
sonuç için hem ham cm değerleri hem normalize değerler gerekir.

Zorunlu apparatus alanları:

```json
{
  "tank_diameter_cm": 120,
  "tank_center_cm": { "x": 0, "y": 0 },
  "platform_diameter_cm": 10,
  "platform_center_cm": { "x": 30, "y": -30 },
  "platform_quadrant": "SE",
  "water_opacity": "opaque",
  "water_temp_c": 22,
  "surface_color": "white",
  "start_positions": ["N", "E", "S", "W"],
  "wall_annulus_width_cm": 12
}
```

Koordinat normalizasyonu:

```txt
radius_cm = tank_diameter_cm / 2
x_norm = x_cm / radius_cm
y_norm = y_cm / radius_cm
distance_norm = distance_cm / tank_diameter_cm
platform_distance_norm = distance_to_platform_cm / tank_diameter_cm
```

Lablar arası raporlar yalnızca uyumlu trial türlerini ve protokol versiyonlarını
karşılaştırmalı, normalize uzaysal metrikleri tercih etmeli, lab içi
tekrarlanabilirlik için ham cm değerlerini korumalı ve latency yanında swim
speed'i her zaman göstermelidir.

## Kalite kontrol

QC durumu (`result.qc.status`) yalnızca veri güvenilirliği hakkındadır.
Davranışsal bir geçti/kaldı yoktur; davranışsal sonuç metrik verisinin
kendisidir.

| QC key | Birim | Varsayılan aksiyon |
|---|---|---|
| `tracking_confidence_mean` | `ratio` | `0.80` altında uyar. |
| `tracking_confidence_p05` | `ratio` | `0.50` altında inceleme ister. |
| `dropped_frame_ratio` | `ratio` | `0.05` üstünde uyar, `0.15` üstünde fail. |
| `calibration_error_cm_mean` | `cm` | `1.0 cm` üstünde uyar. |
| `calibration_error_cm_max` | `cm` | `2.0 cm` üstünde inceleme ister. |
| `occlusion_time_ratio` | `ratio` | `0.10` üstünde uyar. |
| `out_of_bounds_time_ratio` | `ratio` | `0.02` üstünde fail. |
| `lighting_warning` | `boolean` | Manuel inceleme. |
| `contrast_warning` | `boolean` | Manuel inceleme. |

QC durumları: `PASS`, `WARN`, `REVIEW_REQUIRED`, `FAIL`.

### Bugün uygulanan kurallar

Yukarıdaki tablo hedef modeldir. API'nin bugün takip edilen bir çalıştırmaya
uyguladığı kurallar paradigma sürümünün kurallarıdır (`trajectoryQC`):

| Kural | Sınır | Neyle ölçülür |
|---|---|---|
| `min_tracking_confidence` | `>= 0.7` | gözlenen örneklerin (takip edilen ve çıkarımlı olmayan) ortalama güveni |
| `max_lost_frame_ratio` | `<= 0.1` | takip edilmeyen örneklerin payı; çıkarımlı örnekler takip edilmiş sayılır |
| `max_calibration_error_cm` | `<= 2` cm | kalibrasyon kaydedilirken kontrol noktaları |

Çıkarımlı örnekler güven ortalamasını asla yükseltmez ya da düşürmez. İşçi
bunların payını yörüngenin tanılama bilgisine (`worker.inferredRatio`) yazar ve
bir paradigma sürümü bunu `max_inferred_ratio` ile sınırlayabilir; yayımlanmış
hiçbir sürüm henüz bu kuralı koymuyor. Bir kuralı çiğneyen çalıştırma
`QC_FAILED` nedeniyle başarısız olur ve hiçbir şey yayımlamaz.

## Sonuç şekli

Domain migration ile `Test.result` yapılandırılmış JSON olmalıdır.

```json
{
  "schemaVersion": "misko.result.v1",
  "paradigmKey": "MWM",
  "protocolVersion": "mwm.v1",
  "trialType": "acquisition_hidden_platform",
  "metrics": {
    "escape_latency_s": 18.4,
    "path_length_cm": 735.2,
    "path_length_norm": 6.13,
    "mean_swim_speed_cm_s": 39.9,
    "thigmotaxis_time_ratio": 0.22
  },
  "qc": {
    "status": "PASS",
    "tracking_confidence_mean": 0.94,
    "dropped_frame_ratio": 0.01,
    "calibration_error_cm_mean": 0.42
  },
  "artifacts": {
    "videoUrl": "s3://misko/tests/t-1/video.mp4",
    "trajectoryUrl": "s3://misko/tests/t-1/trajectory.parquet",
    "heatmapUrl": "s3://misko/tests/t-1/heatmap.png"
  }
}
```
