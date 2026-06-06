---
title: Alan modeli
description: Mişko'nun system of record olarak sahip olduğu yalın, senaryo-merkezli veri modeli.
---

Mişko, kasıtlı olarak küçük tutulmuş bir alana sahip **video-only davranış testi
platformudur**. Mişko bu modeli **system of record** olarak sahiplenir; CV servisi
bağımsız kalır. Metrik tanımları, MWM normalizasyonu ve kalite kontrol semantiği
[Ölçüm mimarisi](../measurements/) sayfasındadır.

Beş kavram, kurulumdan sonuca tek bir düz çizgi:

```txt
Denek (fare)
Paradigma (salt-okunur kod katalog) -> Ortam (isimli fiziksel instance)
                                          |
                                          v
                                       Senaryo  (ortam + metrikler +
                                                  beklenen sonuçlar)
                                          |
                                          v
Test = Denek + Senaryo -> sonuç (metrikler) + passed
```

## Kiracılık (tek kiracı, on-prem)

Mişko **kurulum başına bir laboratuvar** çalıştırır - singleton, çok kiracılı
değil.

- **`Laboratory`** (singleton): ad, kod, zaman dilimi, ayarlar.
- **Kurulum sihirbazı (CLI, açılışta):** lab adı + superadmin e-postasını alır ve
  `Laboratory` ile `SUPERADMIN`'i birlikte oluşturur. Singleton guard ikinci bir
  lab oluşturmayı reddeder; adım idempotenttir.

## Denek (fare)

Sade tutulur: `code` (benzersiz), `sex`, `groupName` (serbest-metin deney grubu),
`birthDate`, `notes`.

## Paradigma ve Ortam

- **Paradigma:** kullanıcının düzenlediği bir DB satırı değil, **kodda tanımlı**
  bilimsel bir test türüdür. Spec (parametreler, bölgeler, metrik sözlüğü,
  önerilen kabul) mühendislik sahipliğindedir ve kararlıdır; yalnızca değerler
  kullanıcı verisidir. Salt-okunur katalog (`GET /api/paradigms`). Kayıt defteri:
  `MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`, `Y_MAZE`, `NOVEL_OBJECT`, `BARNES_MAZE`,
  `THREE_CHAMBER`, `LIGHT_DARK`, `POLE`, `TREADMILL`.
- **Ortam (Environment):** bir lab'ın paradigmadan oluşturduğu isimli, kalıcı
  instance. Kendine yeten bir snapshot tutar (`{ paradigmKey, schemaVersion,
  apparatus, zones }`); apparatus değerleri kod tarafından sabit parametre
  aralıklarına göre doğrulanır ve test anında kilitlenir. Paradigma başına birden
  çok ortam (ör. iki ayrı Morris su tankı).

Fiziksel düzenek **Ortam'ın kendisidir** - ayrı bir `Apparatus` modeli yoktur.
Kamera-cm kalibrasyonu sonraki bir konudur (yol haritası Adım 5).

## Senaryo (merkezî deney tanımı)

`Scenario`, bir araştırmacının bir kez kurduğu, bir deneyin eksiksiz ve yeniden
kullanılabilir tanımıdır. Her detayı taşır:

- `name`, `description`.
- **ortamlar** (N-N) - üzerinde koşulacak bir veya birden çok ortam; böylece
  senaryo bir veya birden çok paradigmayı kapsayabilir.
- **ortam başına beklenen sonuçlar** - `{ [environmentId]: [kriterler] }` map'i.
  Her ortam kendi geç/kal kriterlerini tanımlar; o ortamın paradigma metriklerine
  göre doğrulanır.
- **oturum parametreleri** - trial sayısı, süre vb.

Her ortam kendi paradigmasını ve kendi beklenen sonuçlarını taşıdığından, bir
senaryo kendini tümüyle tanımlar.

## Test (tek koşu)

`Test`, bir `Scenario`'nun bir `Subject` üzerinde tek bir koşusudur:

- `scenarioId` (ortamı, paradigmayı, metrikleri ve beklenen sonuçları getirir),
  `subjectId`, `operatorId`.
- `status` (`PENDING | RUNNING | DONE | FAILED`), `startedAt`, `endedAt`.
- `result` (JSON) - CV'nin hesapladığı metrikler + QC + artefakt URL'leri,
  senaryonun metriklerine göre doğrulanır.
- `passed` - `result`'ın senaryonun beklenen sonuçlarına göre değerlendirmesi
  (yoksa null).

Test başına paradigma, ortam, metrik veya kabul seçimi **yoktur** - hepsi
senaryoda durur. Test başlatmak yalnızca bir denek ister.

## Roller & izinler (RBAC)

**Kodda tanımlı izin matrisi** olan beş lab odaklı rol (DB'den düzenlenebilir izin
yok):

| İzin \ Rol | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|:--:|:--:|:--:|:--:|:--:|
| `user:manage`     | ✅ | ✅ | — | — | — |
| `lab:configure`   | ✅ | ✅ | — | — | — |
| `subject:write`   | ✅ | ✅ | ✅ | — | — |
| `apparatus:write` (ortamlar & senaryolar) | ✅ | ✅ | ✅ | — | — |
| `test:write`      | ✅ | ✅ | ✅ | — | — |
| `test:run`        | ✅ | ✅ | ✅ | ✅ | — |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

`requirePermission("test:run")` middleware'i birincil kapıdır.

## Mişko'da durmayanlar

Kare kare telemetri ve event satırları CV servisinin PostgreSQL'inde durur. Video
ve trajektori dosyaları object storage'da durur - Mişko yalnızca URL'leri tutar.
Mişko'da timeseries tablosu yoktur. Bkz. [Entegrasyon](../integration/).
