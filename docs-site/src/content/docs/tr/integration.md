---
title: Entegrasyon (Mişko ↔ CV servisi)
description: Birbirinden tamamen bağımsız iki sistem arasındaki kontrat.
---

Bu belge **birbirinden tamamen bağımsız** iki sistemin nasıl konuştuğunu tanımlar.
Amaç gevşek bağ: her taraf kendi veritabanına, sürüm döngüsüne ve dağıtımına
sahiptir; yalnızca aşağıdaki sınır üzerinden haberleşirler.

## İki sistem ve sorumluluklar

| | **Mişko** | **CV servisi** |
|---|---|---|
| Rol | Lab iş akışının system of record'u | Yakalama + inference + telemetri |
| Stack | Node/Express/Prisma/**PostgreSQL** + Vue | Python/FastAPI/YOLOv8/ByteTrack/OpenCV |
| Tuttuğu | user, subject, environment, scenario, **test + özet** | ham kare telemetrisi, event'ler, video |
| Hacim | Küçük / ilişkisel | **Büyük** (kendi PostgreSQL'i) |
| Sahiplik | Testin "ne / kim / ne zaman"ı | Testin "ölçüm / kanıt"ı |

## Veri sahipliği

- **Mişko** → testin kimliği ve bağlamı, ayrıca **ortam-bazlı metrik sonuçları**
  (`result` JSON).
- **CV servisi** → ham çıktı: kare kare konum/poz telemetrisi, event satırları,
  video dosyaları. Bunların hiçbiri Mişko'nun veritabanına girmez.
- **Object storage** (MinIO/S3) → video ve büyük artefaktlar. İki taraf da
  yalnızca **URL referansı** tutar, dosyanın kendisini değil.

## Eşleme: Test ↔ capture session

Bir Mişko **Test**'i, CV tarafında bir **capture session**'a karşılık gelir;
bağ iki şeyle kurulur:

- `cameraId` / `cageId` — hangi kamera/kafes (CV her event'i bununla etiketler).
- Zaman penceresi — Test'in `startedAt → endedAt` aralığı.

CV servisi sürekli çalışabilir; bir Test sadece o akıştan bir **dilimi**
sahiplenir, böylece CV'nin Mişko'yu beklemesi gerekmez.

## Test yaşam döngüsü

Bir Test yalnızca **Senaryo + Denek**'ten oluşturulur. Senaryo ortamları
(geometri/zones) ve paradigmalarını (metrikleri sabitler) zaten taşır -
dolayısıyla bunları ne operatör ne de CV koşu anında seçer.

```mermaid
sequenceDiagram
    participant Op as Operatör (Vue)
    participant M as Mişko API
    participant CV as CV servisi
    participant S as Object storage

    Op->>M: POST /api/tests (scenario, subject, cameraId)
    M-->>Op: Test (PENDING)
    Op->>M: PATCH /api/tests/:id (status=RUNNING, startedAt)
    Note over CV: CV zaten o kamerayı işliyor;<br/>event'ler camera_id ile DB'sine yazılıyor
    Op->>M: PATCH /api/tests/:id (status=DONE, endedAt)
    CV->>CV: [startedAt, endedAt] + cameraId için ortamın metriklerini hesapla
    CV->>S: video + trajektori yükle
    CV->>M: POST /api/tests/:id/result (servis auth) + metrics + artefakt URL'leri
    M->>M: ortam başına doğrula + sakla
    M-->>CV: 200 OK (idempotent)
```

## CV sonuç akışı (Mişko içinde)

CV bir sonuç push ettiğinde Mişko sabit bir hat işletir:

1. **Servis auth** - `X-Service-Key`, `SERVICE_API_KEY` ile doğrulanır (operatör
   JWT yok); aksi halde 401.
2. **Idempotency** - bu `captureSessionId` daha önce kaydedildiyse, saklanan sonuç
   no-op olarak döner. Yeniden denemek güvenli.
3. **Kontratı çöz** - push, testin senaryosunun bir **ortamını** hedefler; o
   ortamın **paradigması** + metrik sözlüğü yüklenir.
4. **Doğrula** - `metrics` içindeki her anahtar o paradigmanın bilinen bir metriği
   olmalı (bilinmeyen anahtarlar reddedilir); değerler `valueType`/`validRange`'e
   uymalı (templated metrikler zone map'idir).
5. **Sakla** - metrikler `Test.result.environments[envId].metrics`'e yazılır (JSON;
   QC + artefakt URL'leri yanında). Ham video/telemetri Mişko'ya hiç girmez.
6. **Sonlandır** - ortam DONE işaretlenir; hepsi bitince test DONE. **Geç/kal
   verdikti yoktur** - sonuç veridir, analiz katmanında yorumlanır. QC metriklerin
   yanında saklanır.

## Sınır API'si (kontrat)

Yön: **CV → Mişko push**. CV ham verinin tek doğruluk kaynağıdır; metriği o üretir
ve bildirir. Mişko, CV'yi sorgulamaz.

```
POST /api/tests/:id/result
X-Service-Key: <SERVICE_API_KEY>
Content-Type: application/json
```

```json
{
  "captureSessionId": "cv-9f3a...",
  "cameraId": "cam-1",
  "startedAt": "2026-06-01T15:00:00Z",
  "endedAt":   "2026-06-01T15:05:00Z",
  "metrics": {
    "distance_cm": 1234.5,
    "time_in_zone_s": 45.2,
    "latency_to_platform_s": 12.0,
    "events": { "ate": 3, "rest": 7 }
  },
  "artifacts": {
    "videoUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.mp4",
    "trajectoryUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.parquet"
  }
}
```

Mişko `metrics`'i ortamın paradigma sözlüğüne göre doğrular,
`Test.result.environments[envId].metrics`'e saklar ve ortamı (hepsi bitince
testi) `DONE` yapar. Geç/kal verdikti yoktur. `captureSessionId` daha önce
işlendiyse çağrı no-op döner (idempotent).

## Servis-servis kimlik doğrulama

Operatör uç noktaları JWT kullanır (mevcut). Servis-servis çağrılar ayrı bir
**servis anahtarı** kullanır: `X-Service-Key` header'ı, `SERVICE_API_KEY` ile
doğrulanır. CV servisi operatör JWT'si taşımaz — kendi servis kimliğiyle gelir.

## Dayanıklılık

- **Idempotency:** `captureSessionId` ile tekrarlı gönderim güvenlidir.
- **Retry:** Mişko erişilemezse CV exponential backoff ile yeniden dener ve
  sonucu başarana kadar "gönderilmedi" işaretiyle tutar.
- **Bağımsız ayakta kalma:** Mişko düşse CV yakalamaya devam eder ve sonradan
  gönderir; CV düşse Mişko'nun domain yönetimi etkilenmez.
