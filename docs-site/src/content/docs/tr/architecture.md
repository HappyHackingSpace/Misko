---
title: Mimari
description: Katmanlar, teknoloji yığını ve iki sistemli sınır.
---

Mişko, ince bir kontratla birbirine bağlanmış iki bağımsız sistemdir. Mişko'nun
kendisi katmanlı bir Express API + Vue 3 SPA'dır; CV servisi ise kendi
veritabanına sahip ayrı bir Python servisidir.

## Teknoloji yığını

| Katman   | Teknoloji                                        |
|----------|--------------------------------------------------|
| Backend  | Node.js 20 · Express · Prisma · PostgreSQL       |
| Frontend | Vue 3 · Vite · Vue Router · Pinia                |
| Güvenlik | JWT · bcryptjs · helmet · zod                    |
| Altyapı  | Docker · Docker Compose · Nginx · GitHub Actions |
| CV svc   | Python · FastAPI · YOLOv8 · ByteTrack · OpenCV   |

## Backend katmanları

```
İstek → routes → middleware (auth/validate) → controller → service → Prisma → DB
                                                  ↑
                                         asyncHandler + ApiError
                                                  ↓
                                       merkezî errorHandler → JSON
```

Her modül (`auth`, `users`, `scenarios`, `subjects`, `devices`, `tests`) aynı
yapıyı izler: `routes + controller + service (+ validation)`. Ortak bir CRUD
fabrikası basit kaynakları tutarlı tutar.

## İki sistemli sınır

Mişko, lab iş akışının **system of record**'udur. Kamera + görü tarafı, kendi
PostgreSQL'i olan **tamamen bağımsız** bir servistir. Ağır veri — kare
telemetrisi, event'ler, video — CV servisinde durur; Mişko yalnızca testin
**özet metriklerini** ve artefakt URL'lerini tutar.

```
Laboratuvar (singleton) ── Mişko API ── PostgreSQL (ilişkisel, küçük)
                               │
                               │  POST /api/tests/:id/result  (X-Service-Key)
                               ▼
                          CV Servisi ── PostgreSQL (telemetri, büyük)
                               │
                               ▼
                       Object storage (MinIO/S3) — video + artefaktlar
```

Tam kontrat — test ↔ capture session eşlemesi, sonuç push gövdesi ve servis-servis
auth — [Entegrasyon](../integration/) sayfasındadır.

## Dağıtım

Tüm yığın Docker Compose ile çalışır (PostgreSQL + API + Nginx). Migration'lar
konteyner açılışında otomatik uygulanır, superadmin ilk açılışta oluşturulur.
Backend ve frontend'in her biri ayrı, path-filtreli bir GitHub Actions pipeline'ına
sahiptir.
