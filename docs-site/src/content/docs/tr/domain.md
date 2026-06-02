---
title: Alan modeli
description: Mişko'nun system of record olarak sahip olduğu araştırma seviyesi veri modeli.
---

Bu, ince başlangıç şemasını gerçek davranışsal-sinirbilim iş akışlarını
destekleyen bir modele dönüştüren hedef modeldir. Mişko bu modeli **system of
record** olarak sahiplenir; CV servisi bağımsız kalır.

Metrik tanımları, MWM normalizasyonu ve kalite kontrol semantiği
[Ölçüm mimarisi](../measurements/) sayfasında detaylandırılır.

## Kiracılık (tek kiracı, on-prem)

Mişko **kurulum başına bir laboratuvar** çalıştırır — singleton, çok kiracılı
değil.

- **`Laboratory`** (singleton): ad, kod, zaman dilimi, ayarlar.
- **Kurulum sihirbazı (CLI, açılışta):** lab adı + superadmin e-postasını alır ve
  `Laboratory` ile `SUPERADMIN`'i birlikte oluşturur. Singleton guard ikinci bir
  lab oluşturmayı reddeder; adım idempotenttir.
- **`LabParadigm`:** paradigma kataloğu globaldir ama her lab bir **alt kümeyi**
  etkinleştirir (ör. "bu labda sadece havuz var"). Yöneticiler açıp kapatır.

## Paradigma ≠ Cihaz ≠ Kalibrasyon

En büyük tasarım kararı — eski `Scenario`'nun birleştirdiği üç kavramı ayırmak:

| Kavram | Yanıtladığı | Değişen | Sahip |
|---|---|---|---|
| **Paradigma** | *ne* test edilir & hangi metrikler önemli | sabit katalog | Mişko (kod) |
| **Cihaz** | *hangi fiziksel düzenek* (geometri, renk, boyut) | lab/kuruluma göre | Mişko |
| **Kalibrasyon** | *piksel → cm + bölgeler* nasıl eşlenir | sabit rig veya oturum başına | Mişko / CV |

Kamera paradigmayı veya bölgeleri **çıkarsamaz**. CV hayvanı takip eder;
geometri (cm cinsinden bölgeler) ve piksel↔cm eşlemesi ona *verilir*.

### Paradigma — hardcoded, SOLID bir kayıt defteri

Paradigma, kullanıcının düzenlediği bir DB satırı değil, **kodda tanımlı**
bilimsel bir test türüdür. Spec (hangi parametreler, bölgeler, metrikler vardır)
mühendislik sahipliğindedir ve kararlıdır; yalnızca **değerler** kullanıcı
verisidir. Her paradigma ortak bir arayüzü uygular; bir tane eklemek bir sınıf
eklemek ve başka hiçbir yere dokunmamak demektir.

```ts
interface ParadigmSpec {
  key: string;            // 'MWM' | 'OPEN_FIELD' | 'EPM' | 'ROTAROD'
  name: string;
  category: string;       // learning_memory | anxiety | motor | social
  parameters: Field[];    // → otomatik UI formu + doğrulama
  zones(config): Zone[];  // yapılandırılmış geometriden somut bölgeler
  metrics: MetricDef[];   // CV servisinin hesaplaması gerekenler
  acceptance(config): Rule | null;
  qc: QualityRequirement[];
  validate(config): void;
}
```

Başlangıç kayıt defteri: `MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`.

Sonuç JSON'u aktif paradigmanın metrik sözlüğüne, sonuç şemasına, QC
gereksinimlerine ve protokol versiyonuna göre doğrulanmalıdır.

### Cihaz (fiziksel "ortam")

Aynı paradigma farklı düzeneklerde çalışır — beyaz tank vs siyah tank, farklı çap,
farklı platform konumu. CV doğruluğu **hayvan-arkaplan kontrastına** bağlı
olduğundan görünüm birinci sınıftır: `surfaceColor`, `material`, `shape`,
`dimensions` (cm), paradigmaya özel config ve cm cinsinden somut `zones`.

### Kalibrasyon (kamera ↔ dünya)

Hem kalıcı monte kamerayı (bir kez kalibre et, cihazda sakla) hem hareketli
kamerayı (oturum başına kalibre et, Test'te override) destekler. Çözümleme
sırası: **Test override → Cihaz varsayılanı**.

## Denek (fare) — zengin alan

- **Kimlik:** kod, mikroçip ID, kulak etiketi.
- **Biyolojik:** tür, suş, hat/genotip, zigotluk, cinsiyet, doğum tarihi,
  **tüy rengi** (görü kontrastı).
- **Fizyoloji:** `WeightLog` zaman serisi (çoğu protokol günlük tartar), sağlık
  durumu.
- **Barınma & yaşam döngüsü:** kafes, batın/kohort, durum (`ALIVE | SACRIFICED |
  DEAD`).
- **Deneysel:** N–N hastalık modelleri ve N–N tedaviler (doz, yol, çizelge ile).

## Hastalık modelleri & tedaviler

Bir fare aynı anda **birden çok** hastalık modeli ve tedavi taşıyabilir. İkisi de
deneğe N–N bağlanan kataloglardır; indükleme yöntemi, doz, yol ve çizelgeyi tutar.

## Çalışma → Grup (boylamsal)

Gerçek lablar bir Çalışma ve karşılaştırma grupları (kontrol, model, tedavi)
etrafında örgütlenir. Bir `Test`, çalışmayı referans alır ve bir `timepoint`
etiketi taşır; böylece aynı deneğin tekrarlı testleri karşılaştırılabilir —
sonraki istatistiklerin temeli.

## Roller & izinler (RBAC)

**Kodda tanımlı izin matrisi** olan beş lab odaklı rol (DB'den düzenlenebilir izin
yok):

| İzin \ Rol | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|:--:|:--:|:--:|:--:|:--:|
| `user:manage`     | ✅ | ✅ | — | — | — |
| `lab:configure`   | ✅ | ✅ | — | — | — |
| `paradigm:toggle` | ✅ | ✅ | — | — | — |
| `study:write`     | ✅ | ✅ | ✅ | — | — |
| `subject:write`   | ✅ | ✅ | ✅ | — | — |
| `apparatus:write` | ✅ | ✅ | ✅ | — | — |
| `weight:write`    | ✅ | ✅ | ✅ | ✅ | — |
| `test:write`      | ✅ | ✅ | ✅ | — | — |
| `test:run`        | ✅ | ✅ | ✅ | ✅ | — |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

`requirePermission("test:run")` middleware'i birincil kapıdır.

## Mişko'da durmayanlar

Kare kare telemetri ve event satırları CV servisinin PostgreSQL'inde durur. Video
ve trajektori dosyaları object storage'da durur — Mişko yalnızca URL'leri tutar.
Mişko'da timeseries tablosu yoktur. Bkz. [Entegrasyon](../integration/).
