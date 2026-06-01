# 🐭 Mişko

Laboratuvar fareleri için davranış testi yönetim sistemi.

Mişko, biyoloji laboratuvarlarında fareler üzerinde yürütülen davranış
testlerini tek bir yerden yönetmeyi amaçlar. Test senaryoları (havuz,
labirent, sopa, yol), denekler, test cihazları ve testlerin tamamı
uygulama üzerinden tanımlanır ve takip edilir.

## Özellikler

- **Kimlik doğrulama** — JWT + bcrypt. **Internal SaaS**: public kayıt yoktur. Superadmin Docker açılışında otomatik oluşturulur (güçlü şifre sistem tarafından üretilir); diğer kullanıcılar **içeriden** ADMIN tarafından açılır.
- **Senaryolar** — 4 sabit apparat türü: `POOL` (havuz), `MAZE` (labirent), `STICK` (sopa), `PATH` (yol).
- **Denekler** — fareler (kod, cinsiyet, grup, not).
- **Test cihazları** — testin yürütüldüğü telefonlar (Android / iOS).
- **Testler** — senaryo + denek + operatör + cihaz; `PENDING → RUNNING → DONE / FAILED` durum akışı.
- **Panel** — özet sayımlar ve son testler.

## Teknoloji

| Katman   | Teknoloji                                          |
|----------|----------------------------------------------------|
| Backend  | Node.js 20 · Express · Prisma · PostgreSQL         |
| Frontend | Vue 3 · Vite · Vue Router · Pinia                  |
| Güvenlik | JWT · bcryptjs · helmet · zod (doğrulama)          |
| Altyapı  | Docker · Docker Compose · Nginx · GitHub Actions   |

## Depo yapısı

```
Misko/
├── docker-compose.yml          # db + backend + frontend
├── .env.example                # compose ortam değişkenleri
├── .github/workflows/
│   ├── backend-ci.yml          # backend pipeline (path-filtered)
│   └── frontend-ci.yml         # frontend pipeline (path-filtered)
│
├── backend/                    # Express API (katmanlı mimari)
│   ├── Dockerfile
│   ├── docker-entrypoint.sh    # migrate deploy + start
│   ├── prisma/
│   │   ├── schema.prisma
│   │   ├── migrations/
│   │   └── seed.js
│   └── src/
│       ├── server.js           # giriş noktası (listen)
│       ├── app.js              # express app fabrikası
│       ├── config/             # ortam değişkeni doğrulama
│       ├── lib/                # prisma client (singleton)
│       ├── middleware/         # authenticate, validate, errorHandler, notFound
│       ├── common/             # genel CRUD servis + controller fabrikası
│       ├── modules/            # auth · scenarios · subjects · devices · tests
│       │   └── <modül>/        #   routes + controller + service (+ validation)
│       ├── routes/             # /api router birleştirici
│       └── utils/              # ApiError, asyncHandler
│
└── frontend/                   # Vue 3 + Vite SPA
    ├── Dockerfile              # vite build → nginx
    ├── nginx.conf              # SPA fallback + /api proxy
    └── src/
        ├── views/              # Dashboard, Login, Scenarios, Subjects, Devices, Tests
        ├── stores/             # Pinia auth store
        ├── router.js
        └── api.js
```

### Backend mimari katmanları

```
İstek → routes → middleware (auth/validate) → controller → service → Prisma → DB
                                                   ↑
                                          asyncHandler + ApiError
                                                   ↓
                                        merkezi errorHandler → JSON
```

## Sistem sınırı — CV servisi ayrıdır

Mişko, lab iş akışının **system of record**'udur (test/denek/senaryo/sonuç).
Kamera + görü (CV) tarafı **tamamen bağımsız** bir servistir
(Python/FastAPI/YOLOv8, kendi PostgreSQL'i). Ağır veri — kare kare telemetri,
event'ler, video — **CV servisinde** durur; Mişko yalnızca testin **özet
metriklerini** ve artefakt URL'lerini saklar.

İki sistemin nasıl konuştuğu (test ↔ capture session eşlemesi, sonuç push
kontratı, servis-servis auth) **[docs/ENTEGRASYON.md](docs/ENTEGRASYON.md)**
dosyasındadır.

## Hızlı başlangıç (Docker — önerilen)

Tek komutla tüm yığın (PostgreSQL + API + Nginx):

```bash
cp .env.example .env          # gerekirse JWT_SECRET vb. düzenle
docker compose up -d --build
```

- Frontend: http://localhost:8080
- API: http://localhost:4000/api
- Migration'lar konteyner açılışında otomatik uygulanır (`prisma migrate deploy`).

**Superadmin** açılışta otomatik oluşturulur (`ADMIN_EMAIL`, varsayılan
`admin@fare.lab`). Sistem **güçlü bir şifre üretir** ve backend log'una **bir kez**
yazar:

```bash
docker compose logs backend | grep -A6 SUPERADMIN
```

Bu şifreyle giriş yapın, ardından şifrenizi değiştirin ve diğer kullanıcıları
panel içindeki **Kullanıcılar** ekranından açın.

Örnek domain verisini (4 senaryo + denek `F-001`) yüklemek için:

```bash
docker compose exec backend node prisma/seed.js
```

## Yerel geliştirme

PostgreSQL'i compose ile çalıştırıp uygulamaları yerelde geliştirin:

```bash
docker compose up -d db        # sadece veritabanı

# Backend
cd backend
cp .env.example .env
npm install
npm run db:migrate             # prisma migrate dev
npm run db:bootstrap           # superadmin oluştur (şifreyi konsola yazar)
npm run db:seed                # örnek domain verisi (opsiyonel)
npm run dev                    # http://localhost:4000

# Frontend (ayrı terminal)
cd frontend
npm install
npm run dev                    # http://localhost:5173  (/api → :4000 proxy)
```

## Giriş

Superadmin bootstrap (`npm run db:bootstrap` veya Docker açılışı) çalıştığında
e-posta `ADMIN_EMAIL` ile, **şifre sistem tarafından üretilip konsola/log'a bir
kez** yazılır. Bu sistemde sabit/varsayılan şifre yoktur — public kayıt da yoktur.

Yeni kullanıcılar panel içindeki **Kullanıcılar** (yalnızca ADMIN) ekranından
açılır; şifre boş bırakılırsa sistem üretir ve bir kez gösterir.

## NPM script'leri

### Backend
| Script | Açıklama |
|--------|----------|
| `npm run dev` | İzlemeli geliştirme sunucusu |
| `npm start` | Üretim sunucusu |
| `npm run lint` / `lint:fix` | ESLint |
| `npm run format` | Prettier |
| `npm run db:migrate` | `prisma migrate dev` |
| `npm run db:deploy` | `prisma migrate deploy` |
| `npm run db:seed` | Örnek veri |
| `npm run db:studio` | Prisma Studio |

### Frontend
| Script | Açıklama |
|--------|----------|
| `npm run dev` | Vite dev sunucusu |
| `npm run build` | Üretim derlemesi |
| `npm run preview` | Derlemeyi önizle |
| `npm run lint` / `lint:fix` | ESLint |

## API (özet)

| Yöntem | Yol                  | Açıklama                       |
|--------|----------------------|--------------------------------|
| GET    | `/api/health`        | Sağlık kontrolü (public)       |
| POST   | `/api/auth/login`    | Giriş, JWT döner               |
| GET    | `/api/auth/me`       | Oturum bilgisi                 |
| CRUD   | `/api/users`         | Kullanıcı yönetimi (ADMIN)     |
| CRUD   | `/api/scenarios`     | Senaryolar                     |
| CRUD   | `/api/subjects`      | Denekler                       |
| CRUD   | `/api/devices`       | Cihazlar                       |
| CRUD   | `/api/tests`         | Testler (durum geçişleri)      |

`/api/auth/*` ve `/api/health` dışındaki tüm uç noktalar
`Authorization: Bearer <token>` ister. Hatalar tek tip JSON döner:
`{ "error": string, "details"?: any }`.

## CI/CD

`backend/**` ve `frontend/**` için **ayrı**, path-filtreli GitHub Actions
pipeline'ları:

- **Backend CI** — PostgreSQL servisi ile: `npm ci` → `prisma generate` →
  lint → `prisma validate` → `migrate deploy` → boot/health smoke testi →
  Docker imaj derlemesi.
- **Frontend CI** — `npm ci` → lint → `vite build` → artifact yükleme →
  Docker imaj derlemesi.

## Yol haritası

Fazlı plan: **[docs/YOL-HARITASI.md](docs/YOL-HARITASI.md)**.

- [x] Faz 0 — Internal SaaS kimlik modeli (superadmin bootstrap, signup kaldırıldı, içeriden kullanıcı yönetimi)
- [ ] Faz 1 — Sınır omurgası: `POST /api/tests/:id/result` (servis auth + idempotency) + `cv-service/` iskeleti + MinIO
- [ ] Faz 2 — Senaryo geometri editörü + kabul kriterleri / metrik motoru
- [ ] Faz 3 — Gerçek CV (local_usb + YOLOv8 + ByteTrack)
- [ ] Faz 4 — Canlı izleme (WS/SSE) + telefon/RTSP adapter'ları
- [ ] Faz 5 — İmaj registry (GHCR) + staging/prod dağıtımı

## Lisans

[MIT](LICENSE) © Happy Hacking Space
