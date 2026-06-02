---
title: Yol haritası
description: Fazlı plan — önce omurgayı uçtan uca çalıştır, sonra her fazda et ekle.
---

Felsefe: **önce omurgayı uçtan uca çalıştır** (sahte veriyle bile), sonra her
fazda et ekle. İki sistem (Mişko + CV servisi) baştan bağımsız kalır; tek temas
noktaları [entegrasyon kontratıdır](../integration/).

## Faz 0 — Kimlik & içeriden SaaS modeli ✅

Mişko on-prem dağıtılan **içeriden (yalnızca kurum) bir SaaS**'tır. Public kayıt
yoktur.

- Docker açılışında superadmin bootstrap (sistem üretimi güçlü şifre, bir kez
  yazılır; idempotent).
- Kayıt kaldırıldı (frontend + backend).
- İçeriden kullanıcı yönetimi (yalnızca admin).

## Faz 1 — Temel: kiracılık + RBAC + paradigma kayıt defteri

- **Kiracılık (`Laboratory`, tek kiracı):** kurulum başına tam olarak bir lab;
  singleton guard'lı bir kurulum sihirbazı ile oluşturulur.
- **RBAC (5 rol, kod matrisi):** SUPERADMIN, LAB_MANAGER, RESEARCHER, TECHNICIAN,
  VIEWER; kodda tanımlı izin matrisi.
- **Paradigma kayıt defteri (hardcoded, SOLID):** `ParadigmSpec` kayıt defteri
  (MWM, OPEN_FIELD, EPM, ROTAROD); UI formu paradigmanın parametrelerinden otomatik
  oluşturulur.
- **`LabParadigm`:** yöneticiler lab başına hangi paradigmaların aktif olduğunu
  açıp kapatır.

## Faz 2 — Alan modeli (bilim)

- Zengin `Subject` + `WeightLog` zaman serisi.
- `DiseaseModel` & `Treatment` katalogları + N–N join'ler.
- `Apparatus` — fiziksel düzenek (cm geometri, yüzey rengi/malzemesi, bölgeler).
- `Study → Group` + boylamsal test.

## Faz 3 — Omurga: sınır kontratı (sahte CV ile)

- `POST /api/tests/:id/result` (servis auth, idempotent).
- Kalibrasyon Test'e bağlanır.
- Kendi PostgreSQL'i olan CV servisi iskeleti + stub sonuç push.
- Object storage için MinIO.

## Faz 4 — Senaryo zekâsı

- Senaryo/cihaz geometri editörü (bölge çizimi, cm ölçeği).
- Paradigma başına kabul kriterleri + metrik motoru.

## Faz 5 — Gerçek CV

- `local_usb` adapter (OpenCV) + YOLOv8 + ByteTrack.

## Faz 6 — Canlı izleme & çoklu kaynak

- CV → Vue paneli WebSocket/SSE ile (Mişko'yu baypas eder).
- `rtsp/http` ve `ws_push` (telefon) adapter'ları.

## Faz 7 — Dağıtım

- İmaj registry (GHCR) + staging/prod dağıtımı.
- (Opsiyonel) CV tarafında TimescaleDB.

---

**Sınır hatırlatması:** ham telemetri, event'ler ve video **CV servisinde** durur;
Mişko yalnızca **özet metrikleri + artefakt URL'lerini** tutar.
