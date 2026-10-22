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
| `subjects` | Denekler ve vücut ağırlıkları |
| `experiments` | Deneyler ve yaşam döngüleri, gruplar, rejim kalemleri, zaman noktaları, sonlanım noktaları, atama ve dışlama |
| `dosing` | Uygulamalar, günlük doz takvimi, indüksiyon sonuçları |
| `paradigms` | Sürümlü paradigma kataloğu ve metrik motoru (yalnızca kod) |
| `environments`, `protocols` | Ölçüm revizyonlu arenalar; bir deneyin test bataryaları |
| `tests` | Oturumlar, arenalarıyla birlikte içlerindeki denekler, denemeler, yorumlar |
| `media`, `calibration` | Videolar (testlerin videoları ve paradigma testleri için varlıklar); bir arenaya, bir test deneğine veya bir paradigma testine ait kalibrasyonlar |
| `analysis` | Analiz servisi: analiz işleri, worker'lar ve yetenekleri, kiralar, çıktılar, metrikler, olaylar ve kalite kontrolü |
| `subjectanalysis` | Analizin deney tarafı: hangi işin hangi deneği analiz ettiği, işleri gönderen düzenleyici, onay |
| `trials` | Paradigma testleri: bir paradigmanın herhangi bir deney dışında, tek arenada tek videoda denenmesi |
| `reports`, `dashboard` | Salt okunur çalışma raporu, CSV dışa aktarma ve açılış ekranı sayıları |

Modüller birbirini import etmez. Biri çalışma anında diğerine ihtiyaç
duyduğunda, kullanan modül application paketinde küçük bir arayüz (port)
tanımlar ve `internal/bootstrap` diğer modülü çağıran bir adaptör bağlar:
analiz servisinin portu (`Submit`, `Get`, `Jobs`, `ByRequester`, `Results`),
media varlık portu, kalibrasyon portu ve ortam revizyonu portu. Biri diğerinin
satırlarına başvurduğunda, örneğin bir atamanın deneğe, bunu veritabanı yabancı
anahtarlarla zorlar; şemadaki raporlama görünümleri rapor için tabloları
birleştirir. Paradigma kataloğu, yalnızca her modülün `adapters/catalog`
paketinin okuduğu ortak bir çekirdektir.

```
trials ───────────┐                         ┌──► media (kaynak videolar)
                  ├──► analysis (port) ─────┤
subjectanalysis ──┘        │                └──► calibration (kopya)
   │                       └──► dinleyici: "iş başarılı" ──► subjectanalysis
   └──► reports, dashboard (deneklerin kabul edilen işleri üzerinde görünümler)
```

`tests/architecture` içindeki politika testi bu yapıyı zorunlu kılar: domain ve
application paketleri adaptörleri import edemez, domain kodu standart
kütüphanenin küçük bir izin listesiyle sınırlıdır, adaptörler yalnızca kendi
modüllerini import eder, analiz servisi deney tarafını veya paradigma
testlerini, paradigma testleri de deney tarafını asla import etmez. Katman
hatası, incelemeyi beklemeden derlemeyi düşürür.

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

Analiz servisi "bu videoyu, bu paradigma sürümüyle, bu parametrelerle ve bu
kalibrasyonla" analiz eder. Bir analiz işi, değişmeyen bir girdi kopyası
(kaynak videonun sürümü ve klibi, çözümlenmiş parametreler, kırpma alanı,
dönüşümü ve bölgeleriyle kalibrasyonun bir kopyası ve opak bir talep eden: bir
test deneği veya bir paradigma testi) ile yürütme durumundan (denemeler ve
kira, çıktılar, metrikler, olaylar, kalite kontrolü) oluşur. İki modül iş
ister: deney tarafı bir test deneği hazır olduğunda onun için bir iş gönderir,
paradigma testleri her video için bir iş ve istendiğinde değiştirilmiş
eşiklerle yeni işler gönderir. Worker bir işi, kırpma alanı arenanın karedeki
bölgesi olan kalibrasyon kopyasıyla üstlenir, sabitlenmiş kaynak videoyu okur
ve bir analiz videosu ile bir `misko.trajectory.v1` yörüngesi yükler. Metrik
göndermez ve deneğin grubunu veya tedavisini hiç öğrenmez. API yüklenen her nesneyi bildirilen boyut, sağlama ve içerik türüne
karşı doğrular, paradigmanın kalite kurallarını uygular; ardından Go metrik
motoru metrikleri ve olayları yörüngeden hesaplar. Böylece takibi ne üretirse
üretsin, her ölçümün tek bir tanımı API içinde kalır. Bir sonuç, ancak bir kişi
onu kabul ettikten sonra çalışma raporunda sayılır; yalnızca test deneklerine
bağlı işler onaya gelir, bir paradigma testinin işi onaya, rapora veya
panoya asla ulaşmaz.

Ağır veri bucket'ta kalır: veritabanı kayıtları, metrikleri, olayları ve nesne
referanslarını tutar, video baytlarını değil.

## Dağıtım

Yığın Docker Compose ile çalışır: PostgreSQL, API, zamanlayıcıyı işleten jobs
süreci (kira süresi dolan işler, deneklerin ve paradigma testlerinin otomatik
işleri) ve paneli sunan Nginx. Şema ve ilk yönetici, konteyner
açılışında değil, açık komutlarla kurulur (`bootstrap migrate` ve tek seferlik
`bootstrap setup`). `migrate`, numaralı SQL migration'larını tek transaction
içinde uygulayıp kaydeder; her deploy yeni container'ları başlatmadan önce onu
çalıştırır. Şema çalışma modeli için sıfırdan yeniden yazıldı; önceki
migration'larla kurulmuş bir veritabanı reddedilir ve sıfırlanmalıdır.
Backend, frontend ve worker'ın her birinin kendi path-filtreli GitHub Actions
pipeline'ı vardır.
