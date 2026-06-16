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
                                       Senaryo  (bir veya birden çok ortam)
                                          |
                                          v
Test = Denek + Senaryo -> ortam-bazlı metrik sonuçları (veri, verdikt yok)
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
  olay tipleri CV detect spec'i ile) mühendislik sahipliğindedir ve kararlıdır; yalnızca değerler
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

`Scenario`, bir kez kurulan, bir deneyin yeniden kullanılabilir tanımıdır. Taşır:

- `name`, `description`.
- **ortamlar** (N-N) - üzerinde koşulacak bir veya birden çok ortam; böylece
  senaryo bir veya birden çok paradigmayı kapsayabilir. Her ortamın paradigması
  metriklerini sabitler.
- **oturum parametreleri** (opsiyonel) - trial sayısı, süre vb.

Senaryoda **geç/kal kriteri yoktur**. Davranışsal sonuç **veridir, verdikt
değil** - yorum analiz katmanında yapılır.

### Sinyal vs metrik vs metrik sonucu

Üç ayrı katman: **sinyaller** (kameranın ham gerçek-zamanlı zaman serisi -
trajectory, timestamp; CV servisinde kalır, Mişko'ya girmez), **metrikler**
(paradigmanın tanımları - ne ölçülür, birim/aralık/inputs) ve **metrik sonuçları**
(bir koşunun değerleri; CV sinyalden hesaplar ya da elle girilir). Mişko metrik
sonuçlarını saklar, sinyalleri değil.

## Test (tek koşu)

`Test`, bir `Scenario`'nun bir `Subject` üzerinde koşusudur. Operatör **ortam
ortam** koşturur (Başlat → metrik sonuçlarını gir → Bitir):

- `scenarioId`, `subjectId`, `operatorId`.
- `status` (`PENDING | RUNNING | DONE | FAILED`), `startedAt`, `endedAt`.
- `result` (JSON) - ortam başına:
  `{ schemaVersion, environments: { [envId]: { status, startedAt, endedAt, metrics } } }`.
  Metrik değerleri ortamın paradigma sözlüğüne göre doğrulanır. CV servisinin
  göndereceği şekille birebir - manuel girişte gözlemlenebilir metrikler elle dolar.

`passed`/verdikt **yoktur**. Test başlatmak yalnızca denek ve senaryo ister.

## Roller & izinler (RBAC)

**Kodda tanımlı izin matrisi** olan beş lab odaklı rol (DB'den düzenlenebilir izin
yok):

| İzin \ Rol | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|:--:|:--:|:--:|:--:|:--:|
| `user:manage`     | ✅ | ✅ | - | - | - |
| `lab:configure`   | ✅ | ✅ | - | - | - |
| `subject:write`   | ✅ | ✅ | ✅ | - | - |
| `apparatus:write` (ortamlar & senaryolar) | ✅ | ✅ | ✅ | - | - |
| `test:write`      | ✅ | ✅ | ✅ | - | - |
| `test:run`        | ✅ | ✅ | ✅ | ✅ | - |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

`requirePermission("test:run")` middleware'i birincil kapıdır.

## Mişko'da durmayanlar

Kare kare telemetri ve event satırları CV servisinin PostgreSQL'inde durur. Video
ve trajektori dosyaları object storage'da durur - Mişko yalnızca URL'leri tutar.
Mişko'da timeseries tablosu yoktur. Bkz. [Entegrasyon](../integration/).
