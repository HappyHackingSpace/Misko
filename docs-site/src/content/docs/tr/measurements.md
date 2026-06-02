---
title: Ölçüm mimarisi
description: Paradigma spec'leri, metrik tanımları, MWM normalizasyonu ve kalite kontrol.
---

Mişko'nun gevşek sonuç JSON'u değil, kararlı ölçüm kontratları tutması gerekir.
Her paradigma, CV servisinin çalışmalar ve laboratuvarlar arasında
karşılaştırılabilir sonuç üretebilmesi için parametrelerini, bölgelerini,
metriklerini, kabul kurallarını ve kalite gereksinimlerini önceden tanımlamalıdır.

## Mimari ilke

Her sonuç dört katmandan geçer:

| Katman | Amaç |
|---|---|
| `ParadigmSpec` | Parametreler, bölgeler, metrikler, kabul kuralları ve sonuç şeması için kod sahipli kontrat. |
| `Apparatus` | Lab sahipli fiziksel düzenek tanımı: geometri, malzeme, yüzey ve cm cinsinden bölgeler. |
| `Calibration` | Sabit düzenek veya tek test oturumu için pikselden cm'ye eşleme. |
| `MetricDefinition` | Her metrik anahtarının birim, formül, normalizasyon ve QC bağımlılığını tanımlayan kod sahipli sözlük. |

CV hayvanı takip eder. Bilimsel kontratı Mişko tanımlar.

## Paradigma spec sayfaları

Her paradigma için şu bölümleri içeren bir detay sayfası gerekir:

- Kimlik: key, ad, kategori, trial türleri.
- Apparatus parametreleri: birimli fiziksel değerler, validasyon, varsayılanlar ve min/max değerler.
- Oturum parametreleri: trial süresi, başlangıç pozisyonu, trial indeksi, protokol varyantı.
- Bölgeler: zone key'leri, geometri tipi, koordinat sistemi ve türetme kuralı.
- Metrikler: zorunlu ve opsiyonel metrik key'leri, birimler, tanımlar ve normalizasyon.
- Kabul kriterleri: çalışma seviyesinde override edilebilen varsayılan pass/fail kuralları.
- QC gereksinimleri: takip güveni, düşen kareler, kalibrasyon hatası, occlusion, ışık ve kontrast.
- Artefaktlar: video, trajectory, heatmap, kalibrasyon görseli ve debug overlay URL'leri.

```ts
interface ParadigmSpec {
  key: "MWM" | "OPEN_FIELD" | "EPM" | "ROTAROD";
  name: string;
  category: "learning_memory" | "anxiety" | "motor" | "social";
  trialTypes: TrialTypeDef[];
  apparatusParameters: FieldDef[];
  sessionParameters: FieldDef[];
  zones(config: ApparatusConfig): ZoneDef[];
  metrics: MetricDefinition[];
  acceptance: AcceptanceRule[];
  qc: QualityRequirement[];
}
```

## Ölçüm sözlüğü

Metrik sözlüğü, iki servisin aynı ismi farklı hesaplar için kullanmasını engeller.

```ts
interface MetricDefinition {
  key: string;
  paradigmKeys: string[];
  unit: "cm" | "cm_s" | "s" | "count" | "ratio" | "percent" | "deg" | "rpm" | "boolean";
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

Kanonik birimler: pozisyon ve mesafe `cm`, hız `cm_s`, süre `s`, ağırlık `g`,
oranlar `0..1`, sayımlar `count`. Piksel değerleri Mişko sonuç metriklerine
girmez.

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
| Open Field | Total distance, center time, periphery time, center entries, immobility, mean speed. |
| EPM | Open arm time, closed arm time, open ve closed arm entries, latency to open arm, opsiyonel risk assessment event'leri. |
| Rotarod | Latency to fall, rpm at fall, trial duration, fall detected, tekrarlı trial'larda learning slope. |

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

QC durumu davranışsal pass/fail'den ayrıdır. `Test.passed` davranışsal kabul
sonucudur. `result.qc.status` güvenilirlik sonucudur.

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
