---
title: Mimari
description: Katmanlar, teknoloji yığını ve analiz worker'larının sisteme nasıl bağlandığı.
---

Mişko; bir Go API'si, bir Vue paneli ve bir PostgreSQL veritabanından oluşur.
Video analizi API'nin dışında, iş talep edip sonuçlarını yükleyen worker'larda
çalışır.

## Teknoloji yığını

| Katman   | Teknoloji                                            |
|----------|------------------------------------------------------|
| Backend  | Go 1.27 · pgx · sqlc · PostgreSQL 18                  |
| Frontend | Vue 3 · Vite · Vue Router · Pinia · vue-i18n          |
| Güvenlik | JWT · bcrypt · rol tabanlı erişim denetimi            |
| Depolama | Google Cloud Storage (özel bucket, imzalı URL'ler)    |
| Worker   | Python · PyAV · NumPy · SciPy                         |
| Altyapı  | Docker · Docker Compose · Nginx · GitHub Actions      |

## Backend katmanları

API modüler bir monolittir. Her alan `internal/` altında kendi paketidir ve
hepsi aynı biçimde bölünür:

```
internal/<alan>/
├── domain/       saf iş kuralları, veritabanı ve HTTP yok
├── application/  kullanım senaryoları ve ihtiyaç duydukları portlar
└── adapters/
    ├── http/       rotalar, istek çözümleme, hata eşleme
    ├── postgres/   sqlc ile üretilen sorgular
    ├── catalog/    paradigma kataloğunun salt okunur görünümleri
    └── token/      token üretme ve doğrulama
```

`internal/bootstrap` kompozisyon köküdür: somut adaptörleri kullanım
senaryolarına bağlar ve tüm rotaları kurar. `internal/access/domain` ortak izin
çekirdeğini tutar; bir kullanım senaryosu bir store'a dokunmadan önce
`actor.Require(permission)` çağırır.

Alanlar çalışma modelini izler (bkz. [Alan modeli](../domain/)):

| Paket | Sahip olduğu |
|---|---|
| `identity`, `laboratory` | Kullanıcılar, giriş, roller, laboratuvar singleton'ı |
| `catalog` | Maddeler ve hastalık modelleri |
| `subjects` | Hayvanlar ve vücut ağırlıkları |
| `experiments` | Deneyler ve yaşam döngüleri, gruplar, rejim kalemleri, zaman noktaları, sonlanım noktaları, atama ve dışlama |
| `dosing` | Uygulamalar, günlük doz takvimi, indüksiyon sonuçları |
| `paradigms` | Sürümlü paradigma kataloğu ve metrik motoru (yalnızca kod) |
| `environments`, `protocols` | Ölçüm revizyonlu arenalar; bir deneyin test bataryaları |
| `tests` | Oturumlar, arenalarıyla birlikte içlerindeki hayvanlar, denemeler, yorumlar |
| `media`, `calibration` | Bir testin videosu; arena başına kalibrasyon ve hayvan başına düzeltmeler |
| `analysis` | Worker'lar, hayvan başına çalıştırmalar, sonuç yayını ve onayı |
| `reports`, `dashboard` | Salt okunur çalışma raporu, CSV dışa aktarma ve açılış ekranı sayıları |

Alanlar birbirini import etmez. Biri diğerine başvurduğunda, örneğin bir
atamanın deneğe, bunu veritabanı yabancı anahtarlarla zorlar; şemadaki raporlama
görünümleri rapor için tabloları birleştirir.

`tests/architecture` içindeki politika testi bu yapıyı zorunlu kılar: domain ve
application paketleri adaptörleri import edemez ve domain kodu standart
kütüphanenin küçük bir izin listesiyle sınırlıdır. Katman hatası, incelemeyi
beklemeden derlemeyi düşürür.

## Liste sorguları

Her liste endpoint'i çıplak bir dizi yerine sayfalanmış bir zarf döndürür:

```json
{ "data": [ ... ], "total": 42, "page": 1, "pageSize": 10 }
```

`page` ve `pageSize` sorgu parametreleridir ve sayfalamayı veritabanı uygular.
Panelde tek bir `DataTable` bileşeni ve `useDataTable` composable'ı bu zarfı
tüketir; böylece arama, sayfalama ve sıralama her ekranda aynı şekilde çalışır.

## Analiz nasıl çalışır

Kamera ve görü tarafı, kendi veritabanı olan ikinci bir servis değildir. Bu
API'ye kimlik doğrulayan bir worker sürecidir:

```
Panel ──► Go API ──► PostgreSQL (kayıtlar, metrikler, olaylar)
             │
             │  Authorization: Worker <token>
             ▼
          Worker  ──►  kaydı okur, çıktılarını imzalı URL'lerle
             │         bucket'a yazar
             ▼
   Cloud Storage (orijinal video, analiz videosu, yörünge)
```

Bir test, tek videosu ve her biri kendi arenasında olan bir veya daha fazla
hayvanı olan tek bir oturumdur. Jobs süreci hayvan başına bir çalıştırma kuyruğa
alır; worker bunu, kırpma alanı arenanın karedeki bölgesi olan o hayvanın arena
kalibrasyonuyla üstlenir. Worker sabitlenmiş kaynak videoyu okur ve bir analiz
videosu ile bir `misko.trajectory.v1` yörüngesi yükler. Metrik göndermez ve
hayvanın grubunu veya tedavisini hiç öğrenmez. API yüklenen her nesneyi bildirilen boyut, sağlama ve içerik türüne
karşı doğrular, paradigmanın kalite kurallarını uygular; ardından Go metrik
motoru metrikleri ve olayları yörüngeden hesaplar. Böylece takibi ne üretirse
üretsin, her ölçümün tek bir tanımı API içinde kalır. Bir sonuç, ancak bir kişi
onu kabul ettikten sonra çalışma raporunda sayılır.

Ağır veri bucket'ta kalır: veritabanı kayıtları, metrikleri, olayları ve nesne
referanslarını tutar, video baytlarını değil.

## Dağıtım

Yığın Docker Compose ile çalışır: PostgreSQL, API, analiz çalıştırmalarını
kuyruklayan jobs süreci ve paneli sunan Nginx. Şema ve ilk yönetici, konteyner
açılışında değil, açık komutlarla kurulur (`bootstrap migrate` ve tek seferlik
`bootstrap setup`). `migrate`, numaralı SQL migration'larını tek transaction
içinde uygulayıp kaydeder; her deploy yeni container'ları başlatmadan önce onu
çalıştırır. Şema çalışma modeli için sıfırdan yeniden yazıldı; önceki
migration'larla kurulmuş bir veritabanı reddedilir ve sıfırlanmalıdır.
Backend, frontend ve worker'ın her birinin kendi path-filtreli GitHub Actions
pipeline'ı vardır.
