# Mişko ↔ CV Servisi — Entegrasyon Kontratı

> Durum: tasarım. Tarih: 2026-06-01.
> İlgili: CV servisinin iç tasarımı `Workspace/fare-davranis/BACKEND-MIMARI.md` dosyasındadır.

Bu belge, **birbirinden tamamen bağımsız** iki sistemin nasıl konuştuğunu tanımlar.
Amaç gevşek bağ (loose coupling): iki taraf da kendi veritabanına, sürüm
döngüsüne ve dağıtımına sahiptir; yalnızca aşağıdaki sınır üzerinden haberleşir.

## 1. İki sistem ve sorumluluklar

| | **Mişko** (bu repo) | **CV Servisi** (`fare-davranis`) |
|---|---|---|
| Rol | Lab iş akışının **system of record**'u | Görüntü yakalama + inference + telemetri |
| Stack | Node/Express/Prisma/**PostgreSQL** + Vue | Python/FastAPI/YOLOv8/ByteTrack/OpenCV |
| Tuttuğu veri | user, subject, scenario, device, **test + özet sonuç** | ham kare telemetrisi, event'ler, video |
| Veri hacmi | Küçük/ilişkisel | **Büyük** (kendi PostgreSQL'i) |
| Sahiplik | Test'in "ne/kim/ne zaman"ı | Test'in "ölçüm/kanıt"ı |

## 2. Veri sahipliği (kim neyi tutar)

- **Mişko** → testin kimliği ve bağlamı: hangi senaryo, hangi denek, hangi
  operatör, hangi cihaz, durum, başlangıç/bitiş, ve testin **özet metrikleri**
  (`result` JSON) + `passed`.
- **CV Servisi** → testin ham çıktısı: kare kare konum/poz telemetrisi,
  event kayıtları (`timestamp · fare_id · olay · tur · sure_sn · camera_id`),
  video dosyaları. **Bunların hiçbiri Mişko'nun Postgres'ine girmez.**
- **Object storage** (MinIO/S3) → video ve büyük artefaktlar. Mişko ve CV
  yalnızca **URL referansı** tutar, dosyanın kendisini değil.

## 3. Eşleme: Test ↔ Capture Session

Bir Mişko **Test**'i, CV tarafında bir **capture session**'a karşılık gelir.
Bağ, iki alanla kurulur:

- `cameraId` / `cageId` — hangi kamera/kafes (CV her event'i bununla etiketler).
- Zaman penceresi — Test'in `startedAt` → `endedAt` aralığı.

> CV servisi sürekli (7/24) çalışabilir; bir Test sadece o akıştan bir
> **zaman dilimini** sahiplenir. Böylece CV'nin Mişko'yu beklemesi gerekmez.

## 4. Test yaşam döngüsü (CV ile)

```mermaid
sequenceDiagram
    participant Op as Operatör (Vue)
    participant M as Mişko API
    participant CV as CV Servisi
    participant S as Object Storage

    Op->>M: POST /api/tests (scenario, subject, device, cameraId)
    M-->>Op: Test (PENDING)
    Op->>M: PATCH /api/tests/:id (status=RUNNING, startedAt)
    Note over CV: CV zaten o kamerayı işliyor;<br/>events camera_id ile DB'sine yazılıyor
    Op->>M: PATCH /api/tests/:id (status=DONE, endedAt)
    M-->>CV: (opsiyonel) test bitti bildirimi / CV polling
    CV->>CV: [startedAt, endedAt] + cameraId için metrik hesapla
    CV->>S: video + trajectory yükle
    CV->>M: POST /api/tests/:id/result (servis auth) + metrics + artifact URL
    M-->>CV: 200 OK (idempotent)
```

## 5. Sınır API'si (kontrat)

İletişim yönü: **CV → Mişko push** (CV ham verinin tek doğruluk kaynağıdır,
metriği o üretir ve Mişko'ya bildirir). Mişko, CV'yi sorgulamaz.

### 5.1 Sonuç gönderimi (uygulanacak)

```
POST /api/tests/:id/result
Authorization: yok  →  X-Service-Key: <SERVICE_API_KEY>
Content-Type: application/json
```

İstek gövdesi:

```json
{
  "captureSessionId": "cv-9f3a...",      // idempotency anahtarı
  "cameraId": "cam-1",
  "startedAt": "2026-06-01T15:00:00Z",
  "endedAt":   "2026-06-01T15:05:00Z",
  "metrics": {                            // senaryoya göre serbest şema
    "distance_cm": 1234.5,
    "time_in_zone_sn": 45.2,
    "latency_to_platform_sn": 12.0,
    "events": { "ate": 3, "rest": 7 }
  },
  "passed": true,
  "artifacts": {
    "videoUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.mp4",
    "trajectoryUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.parquet"
  }
}
```

Mişko'nun yaptığı: `metrics` → `Test.result` (JSON), `passed` → `Test.passed`,
durum `DONE`. `captureSessionId` daha önce işlendiyse no-op döner (idempotent).

### 5.2 Kimlik doğrulama (servis-servis)

- Operatör uç noktaları JWT kullanır (mevcut).
- **Servis-servis** çağrılar ayrı bir **servis anahtarı** kullanır:
  `X-Service-Key` header'ı, env `SERVICE_API_KEY` ile doğrulanır.
- CV servisi operatör JWT'si taşımaz; kendi servis kimliğiyle gelir.

## 6. Sınırı geçmeyen şeyler

- Ham kare telemetrisi ve event satırları → **CV'nin PostgreSQL'inde** kalır.
- Video/parquet → **object storage**; Mişko sadece URL görür.
- Mişko'nun şemasına telemetri/timeseries tablosu **eklenmez** (Timescale dahil).

## 7. Dayanıklılık

- **Idempotency:** `captureSessionId` ile tekrarlı gönderim güvenli.
- **Retry:** CV, Mişko erişilemezse exponential backoff ile yeniden dener;
  başarana kadar sonucu kendi tarafında "gönderilmedi" işaretiyle tutar.
- **Bağımsız ayakta kalma:** Mişko düşse CV yakalamaya/yazmaya devam eder;
  sonuçları sonradan gönderir. CV düşse Mişko domain yönetimi etkilenmez.

## 8. Gelecek (bu sürümde yok)

- Push yerine **mesaj kuyruğu** (Redis/RabbitMQ) ile sonuç teslimi.
- Canlı izleme için CV'den Vue paneline **WebSocket/SSE** (Mişko'yu baypas eden ayrı kanal).
- Per-frame telemetri için CV tarafında **TimescaleDB** yükseltmesi.

---

**Özet:** Mişko hafif/ilişkisel system-of-record; CV servisi ağır veriyi kendi
Postgres'inde tutan bağımsız bir görü servisi. Tek temas noktası, test bitişinde
CV'nin Mişko'ya gönderdiği **özet + artefakt URL**'leridir.
