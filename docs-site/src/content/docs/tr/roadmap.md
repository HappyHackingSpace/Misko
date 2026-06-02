---
title: Yol haritası
description: Video-only davranış testi platformu için adım adım mimari.
---

Mişko **video-only davranış testi platformu** olarak başlar. Core roadmap içinde
sensör yoktur. Tüm ölçümler kamera karelerinden, kalibrasyondan, apparatus
geometrisinden ve kod sahipli paradigma spec'lerinden türetilir.

```txt
Kamera karesi
  -> detection veya segmentation
  -> tracking
  -> pikselden cm'ye kalibrasyon
  -> apparatus koordinatlarında trajectory
  -> zone ve event metrikleri
  -> kalite kontrol metrikleri
  -> Mişko'da valide edilmiş özet sonuç
```

## Adım 0 - Kimlik ✅

- Docker açılışında superadmin bootstrap.
- Public kayıt kaldırıldı.
- İçeriden kullanıcı yönetimi.

## Adım 1 - Lab temeli

- `Laboratory` singleton: kurulum başına bir lab.
- Kurulum sihirbazı lab ve `SUPERADMIN`'i birlikte oluşturur.
- Beş rol: `SUPERADMIN`, `LAB_MANAGER`, `RESEARCHER`, `TECHNICIAN`, `VIEWER`. ✅
- `requirePermission(...)` ile kodda tanımlı izin matrisi. ✅
- `LabParadigm` yöneticilerin paradigmaları açıp kapatmasını sağlar.

Tamamlandı: beş rollü izin matrisi (`backend/src/config/permissions.js`) ve subject, scenario, device, test, user route'larında `requirePermission(...)` denetimi, ayrıca ADMIN/OPERATOR'dan SUPERADMIN/RESEARCHER'a geçiş migration'ı. Bekleyen: `Laboratory` singleton ve kurulum sihirbazı.

## Adım 2 - Bilimsel kontrat

- Kod sahipli `ParadigmSpec` kayıt defteri (11 paradigma): `MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`, `Y_MAZE`, `NOVEL_OBJECT`, `BARNES_MAZE`, `THREE_CHAMBER`, `LIGHT_DARK`, `POLE`, `TREADMILL`. ✅
- [Ölçüm mimarisi](../measurements/) içindeki kod sahipli metrik sözlüğü. ✅
- Kanonik birimler: `cm`, `cm_s`, `s`, `ratio`, `count`. ✅
- Paradigma başına parametreler, bölgeler, metrikler, önerilen kabul şablonları, QC gereksinimleri ve artefakt beklentileri. ✅
- Opsiyonel, test bazlı, kullanıcı tanımlı kabul kriterleri (motor `backend/src/config/acceptance.js`, düzenleyici `frontend/src/components/AcceptanceEditor.vue`). ✅
- `schemaVersion` ile sonuç şema versiyonlama. ✅
- Salt-okunur inceleme API'si: `GET /api/paradigms`, `GET /api/paradigms/:key`, `GET /api/paradigms/metrics`, `GET /api/paradigms/units`. ✅

Tamamlandı: kayıt defterleri `backend/src/config/{units,metrics,paradigms}.js` içinde; `isKnownMetricKey(...)` Step 4'te tanımsız sonuç anahtarlarını reddetmeye hazır. Bekleyen: paradigma detay sayfalarının frontend'de gösterilmesi ve reddetme mantığının sonuç gönderimine bağlanması.

Çıkış kriteri: her metriğin birimi, tanımı, input listesi ve aggregation davranışı vardır.

## Adım 3 - Araştırma domain modeli

- Zengin `Subject` ve `WeightLog`.
- Denek join'leriyle `DiseaseModel` ve `Treatment` katalogları.
- Boylamsal çalışma için `Study -> Group`.
- Fiziksel düzenekler için `Apparatus`.
- Sabit kurulum veya oturum override için `Calibration`.
- `Test` paradigma, apparatus, denek, operatör, cihaz, çalışma, timepoint ve kalibrasyonu referans alır.
- `Test.result` yapılandırılmış JSON olur.

## Adım 4 - Fake CV ile video-only sınır

- `X-Service-Key` ile `POST /api/tests/:id/result`.
- `captureSessionId` ile idempotency.
- Aktif paradigma spec'i ve metrik sözlüğüne göre sonuç validasyonu.
- Kendi PostgreSQL'i ve `/health` endpoint'i olan `cv-service/` iskeleti.
- Sahte ama geçerli metriklerle stub sonuç push.
- Video ve artefaktlar için MinIO.

## Adım 5 - Geometri ve kalibrasyon

- Circle, rectangle, plus ve custom polygon için apparatus geometri editörü.
- Platform, center, periphery, quadrant, wall annulus ve arm zone editörü.
- Referans kareyle pikselden cm'ye kalibrasyon ve reprojection error.
- Sabit apparatus kalibrasyonu ve test başına override.
- Tank merkezli koordinatlar, ham cm değerleri ve normalize mesafelerle MWM normalizasyonu.

## Adım 6 - Gerçek video CV MVP

- Kamera adapter'ları: `local_usb`, yüklenen video dosyası ve sonra telefon stream'i.
- Fare lokalizasyonu için detection veya segmentation modeli.
- ByteTrack veya eşdeğer tracker.
- OpenCV preprocessing ve homography.
- Kare bazlı telemetri CV servisinde kalır.
- Özet metrikler Mişko'ya push edilir.

| Paradigma | MVP metrikleri |
|---|---|
| MWM | Escape latency, path length, swim speed, quadrant time, thigmotaxis, probe trial için platform crossings. |
| Open Field | Distance, mean speed, center time, periphery time, immobility. |
| EPM | Open arm time, closed arm time, open arm entries, closed arm entries. |
| Rotarod | İlk aşamada manuel incelemeyle trial duration ve fall candidate event'leri. |

## Adım 7 - Kalite kontrol ve inceleme

- Tracking confidence, dropped frame ratio, calibration error, occlusion ratio, out-of-bounds ratio, lighting warning ve contrast warning.
- QC durumları: `PASS`, `WARN`, `REVIEW_REQUIRED`, `FAIL`.
- QC davranışsal `passed` değerinden ayrıdır.
- Video, overlay, trajectory ve metrik özetiyle manuel inceleme ekranı.
- Study export'ları düşük kaliteli koşuları varsayılan olarak filtreler.

## Adım 8 - Analiz ve raporlama

- Grup ve timepoint bazlı study dashboard'ları.
- Boylamsal denek görünümü.
- MWM acquisition curve ve probe summary.
- Open Field ve EPM özetleri.
- Rotarod tekrarlı trial curve'leri.
- Metrik tanımları ve QC durumuyla CSV ve JSON export.

## Adım 9 - Gelişmiş davranış modülleri

- Opsiyonel pose-estimation adapter'ı: DeepLabCut, SLEAP veya başka açık kaynak model.
- Rearing, grooming, freezing, risk assessment ve head direction classifier'ları.
- İyileştirilmiş Rotarod fall detection.
- Single-animal iş akışları stabil olduktan sonra multi-animal desteği.

## Adım 10 - Canlı izleme ve kaynaklar

- WebSocket veya SSE ile CV'den Vue live paneline akış.
- RTSP ve HTTP adapter'ları.
- Telefon `getUserMedia` push adapter'ı.
- Canlı QC uyarıları.

## Adım 11 - Açık kaynak deployment

- Mişko ve CV servisi için final lisans stratejisi.
- Tüm local stack için Docker Compose profili.
- GHCR image publishing.
- GitHub Pages docs deploy.
- Örnek dataset'ler, demo videolar ve örnek apparatus tanımları.

## Non-goals

- Core mimaride sensor fusion yok.
- RFID, accelerometer, load cell veya IR beam bağımlılığı yok.
- Mişko PostgreSQL içinde ham kare telemetrisi yok.
- Bilimsel paradigma tanımları için editable DB row yok.
- İlk mimaride cross-lab multi-tenant deployment yok.
