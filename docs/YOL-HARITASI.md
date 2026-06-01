# Mişko — Yol Haritası (fazlı)

> Felsefe: **önce uçtan uca omurga çalışsın** (sahte veriyle bile), sonra her
> fazda et bağla. İki sistem (Mişko + CV servisi) baştan **bağımsız** kalır;
> tek temas noktası `docs/ENTEGRASYON.md`'deki kontrattır.

## Faz 0 — Kimlik & internal SaaS modeli (önce bu)

Mişko **internal (kuruma özel) bir SaaS** uygulamasıdır. Dışarıya açık kayıt
(public signup) **yoktur**. Kullanıcı yönetimi tamamen içeriden yapılır.

- **Superadmin bootstrap (Docker açılışında):** Proje ilk ayağa kalkarken
  superadmin **e-postası** ortam değişkeninden (`ADMIN_EMAIL`) verilir; sistem
  **güçlü bir şifre üretir** ve bunu açılış log'una **bir kez** yazar. Admin
  zaten varsa işlem **no-op**'tur (idempotent).
- **Signup kaldırıldı:** Frontend'deki "Kayıt ol" akışı ve backend'deki public
  `POST /api/auth/register` uç noktası kaldırılır. Geriye sadece **giriş**
  (`/login`) ve **oturum** (`/me`) kalır.
- **İçeriden kullanıcı yönetimi:** Yeni kullanıcıları yalnızca **ADMIN** açar
  (`/api/users`, admin-only). Kullanıcı adı/e-posta + şifre admin tarafından
  atanır; şifre admin tarafından sıfırlanabilir.
- Roller: `ADMIN` (kullanıcı yönetimi + her şey) ve `OPERATOR` (test/denek
  yönetimi).

## Faz 1 — Omurga: sınır kontratı çalışsın (sahte CV ile)

- Mişko: `POST /api/tests/:id/result` (servis-servis `X-Service-Key` auth,
  `captureSessionId` ile **idempotent**). `metrics → Test.result`,
  `passed → Test.passed`, durum `DONE`.
- `result` / `Scenario.config` PostgreSQL'de **Jsonb** olarak tutulur.
- CV servisi iskeleti: `cv-service/` (monorepo), **kendi PostgreSQL'i**,
  FastAPI `/health`, sonuç push'u için **STUB** (gerçek görü yok).
- Object storage: compose'a **MinIO** (video/artefakt URL'leri için).

## Faz 2 — Senaryo zekâsı

- Senaryo **geometri editörü** (alan/bölge çizimi, cm ölçek) → `Scenario.config`.
- Senaryo başına **kabul kriterleri** ve **metrik motoru** (passed hesaplama).

## Faz 3 — Gerçek CV

- `local_usb` adapter (OpenCV) + **YOLOv8** (native) + **ByteTrack**.
- CV kendi DB'sine telemetri/event yazar; test bitişinde Mişko'ya özet push'lar.

## Faz 4 — Canlı izleme & çoklu kaynak

- CV → Vue paneline **WebSocket/SSE** (Mişko'yu baypas eden ayrı kanal).
- `rtsp/http` ve `ws_push` (telefon getUserMedia) adapter'ları.

## Faz 5 — Dağıtım

- İmaj registry (GHCR) push + staging/prod dağıtımı.
- (Opsiyonel) per-frame telemetri için CV tarafında **TimescaleDB**.

---

**Sınır hatırlatması:** Ham kare telemetrisi, event'ler ve video **CV
servisinde** kalır; Mişko yalnızca **özet metrik + artefakt URL** tutar. Detay:
`docs/ENTEGRASYON.md`.
