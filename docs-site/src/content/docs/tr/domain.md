---
title: Alan modeli
description: Mişko'nun system of record olarak sahip olduğu çalışma modeli; katalog ve çalışma tasarımından test oturumlarına, onaylanmış sonuçlara ve rapora kadar.
---

Mişko, kemirgenler üzerindeki preklinik davranış çalışmalarının system of
record'udur. Bir çalışma tek bir soruyu yanıtlar: X maddesi, D dozunda, M
hastalık modelinin yarattığı bozukluğu geri çeviriyor mu? Model bu sorudan
çıkar. Metrik tanımları ve kalite kontrol kuralları
[Ölçüm mimarisi](../measurements/) sayfasındadır. Gerekçeleri ve kararlarıyla
tam tasarım `docs/plans/DOMAIN_REDESIGN.md` dosyasındadır.

```txt
Katalog:    Madde, Hastalık modeli, Paradigma (kod), Ortam (arena)

Tasarım:    Deney -> Gruplar (rol + hastalık modeli) -> Rejim kalemleri
                  -> Zaman noktaları, Protokoller, Sonlanım noktaları

Yürütme:    Denek -> Atama (ömür boyu tek grup) -> Uygulamalar,
                                                   İndüksiyon sonucu
            Test (oturum: zaman noktası x protokol adımı, tek video)
              -> Test denekleri (denek + arena) -> Denemeler
                                                 -> Kalibrasyon (arena veya düzeltme, bölgelerle)
                                                 -> Analiz çalıştırmaları (worker takip eder, Mişko türetir)
                                                    -> Onaylanmış sonuç

Rapor:      denek başına kabul edilmiş sonuçlar -> gruplar -> rol tabanlı karşılaştırmalar
```

## Kiracılık (tek kiracı, on-prem)

Mişko **kurulum başına bir laboratuvar** çalıştırır; singleton, çok kiracılı
değil.

- **`Laboratory`** (singleton): ad, kod, saat dilimi. Gün 0, doz günleri ve
  varsayılan test saatleri laboratuvarın saat dilimini kullanır.
- **Kurulum (CLI):** `bootstrap setup` laboratuvar adını ve yönetici e-postasını
  alır, laboratuvarı ve `SUPERADMIN` kullanıcısını birlikte oluşturur. Tekrar
  çalıştırılırsa hiçbir şey oluşturmaz.

## Katalog

Çalışmalar arasında yeniden kullanılan, laboratuvar genelindeki referans veri.

- **Madde:** bir deneğe verilen her şey: etkin bir bileşik, referans ilaç,
  taşıyıcı (serum fizyolojik, %0,5 CMC) ya da bir hastalık modelini indükleyen
  ajan (MPTP, LPS, skopolamin). `kind` (`COMPOUND`, `REFERENCE_DRUG`, `VEHICLE`,
  `INDUCER`) yalnızca seçim listelerine yardım eder; bir maddenin çalışmadaki
  görevi kullanıldığı yerden gelir.
- **Hastalık modeli:** bozukluğun nasıl oluşturulduğu (`CHEMICAL`, `SURGICAL`,
  `DIET`, `STRESS`, `OTHER`) ve serbest metin olarak nasıl doğrulandığı. Her
  model indüklenir.
- **Paradigma:** düzenlenebilir bir veritabanı satırı değil, **kodda tanımlı**
  bilimsel bir test türüdür: parametreler, bölgeler, türetilen olaylar ve metrik
  tanımları, hiç değişmeyen yayınlanmış sürümler halinde. Kayıt defteri: `MWM`,
  `NOVEL_OBJECT`, `POLE`, `TREADMILL`.
- **Ortam:** tek bir paradigmanın tek bir fiziksel düzeneği; örneğin belirli bir
  su tankı ya da tek kamera altındaki üç yeni nesne kutusundan biri. Ölçüleri
  değişmeyen, numaralı revizyonlardır. Bir testte denek bir **arenaya** konur;
  arena, bir ortam revizyonudur.

## Denek

Denek: kod, tür (`MOUSE`, `RAT`) ve cinsiyet. Bir denek bir çalışmaya
katılmadan önce kaydedilebilir. Vücut ağırlıkları deneğe aittir, ölçüm zamanıyla
kaydedilir ve hiç değişmez.

## Çalışma tasarımı

Buradaki her şey hiçbir deneğe dokunulmadan önce kararlaştırılır ve çalışma
başladığında dondurulur.

- **Deney:** kod, başlık, açıklama, hipotez, randomizasyon yöntemi ve kör olup
  olmadığı. Yaşam döngüsü `DRAFT -> RUNNING -> COMPLETED`. Başlatmak **gün 0**'ı
  belirler; tamamlamak deneyi salt okunur yapar.
- **Grup:** çalışmanın bir kolu; rolü ve hastalık modeliyle tanımlanır:

  | Rol | Hastalık modeli | Aldığı | Amacı |
  |---|---|---|---|
  | `HEALTHY_CONTROL` | yok | varsa taşıyıcı | normal referans |
  | `SHAM` | var | indüksiyon yerine taşıyıcı | indüksiyon işleminin kendisini kontrol eder |
  | `DISEASE_CONTROL` | var | indüksiyon ve taşıyıcı | hastalığın yarattığı bozukluk |
  | `POSITIVE_CONTROL` | var | indüksiyon ve referans ilaç | testin iyileşmeyi yakalayabildiğini gösterir |
  | `TREATMENT` | var | indüksiyon ve test edilen madde | etkinlik ve doz yanıtı |

  Bir grubun hedef sayısı (n) vardır.
- **Rejim kalemi:** bir grubun aldığı: amaç (`INDUCTION`, `VEHICLE`,
  `TREATMENT`), madde, doz, birim, uygulama yolu, sıklık, ilk ve son çalışma
  günü. Bir grubun en fazla bir tedavi kalemi olur; böylece bir tedavi grubu tek
  bir maddeyi tek bir dozda alır. Aynı maddenin tedavi grupları doz yanıtını
  oluşturur.
- **Zaman noktası:** deneklerin test edildiği an; gün 0'a göre çalışma günü
  olarak (gün -1'de Baseline, gün 14'te Day 14).
- **Protokol:** test bataryası; sıralı adımlardan oluşan, değişmeyen sürümleri
  vardır. Bir adım; paradigma sürümü, deneme türü, deneme sayısı ve oturum
  ayarlarıdır. Düzenek belirtmez.
- **Sonlanım noktası:** çalışmanın neye göre değerlendirildiği: bir zaman
  noktasında bir paradigmanın bir metriği, birincil veya ikincil, hangi yönün
  daha iyi sayıldığı ve isteğe bağlı bir başlangıç zaman noktası. Bir çalışmanın
  bir birincil sonlanım noktası olur.

Bir çalışma yalnızca tasarımı tamamsa başlar: zaman noktaları, hedef sayılı
gruplar, bir birincil sonlanım noktası, her role uyan bir rejim ve sham, tedavi
veya pozitif kontrol gruplarının kullandığı her hastalık modeli için bir
hastalık kontrolü.

## Yürütme

- **Atama:** bir denek **ömür boyu tam olarak bir gruba, dolayısıyla tek bir
  çalışmaya** aittir. Denekler elle atanabilir ya da hedef sayılarına kadar
  gruplara rastgele dağıtılabilir. Taslakta atama taşınabilir veya kaldırılabilir;
  çalışma başladıktan sonra grup sabittir.
- **Dışlama:** çalışmadan ayrılan bir denek (`DIED`, `HUMANE_ENDPOINT`,
  `INDUCTION_FAILED`, `TECHNICAL`, `OTHER`) bir gerekçeyle dışlanır, asla
  silinmez. Raporda görünmeye devam eder ve yeniden test veya analiz edilmez.
- **Uygulama:** gerçekten verilen her doz; hastalık indüksiyonu ve taşıyıcı
  dahil. İsteğe bağlı olarak karşıladığı rejim kalemine bağlanır. Kilogram başına
  bir doz, hesaplandığı ağırlığı kaydeder. Günlük doz takvimi rejim
  kalemlerinden ve gün 0'dan türetilir.
- **İndüksiyon sonucu:** deneğin hastalık modelinin doğrulanıp doğrulanmadığı;
  bir kez kaydedilir.

## Test (tek oturum)

`Test`, çalışan bir deneyin tek bir oturumudur: bir zaman noktası, bir protokol
adımı, **tek bir video**. Her biri kendi arenasında olan bir veya daha fazla
deneği, yani **test deneklerini** kaydeder; genellikle bir, bazen tek kamera
altında en fazla üç arena.

- Durum: `PLANNED -> IN_PROGRESS -> COMPLETED`, ya da gerekçeyle `CANCELLED`.
- Denekler yalnızca test planlıyken katılır veya ayrılır. Bir denek her zaman
  noktası ve protokol adımında en fazla bir kez test edilir ve her arena adımın
  paradigmasını çalıştırmalıdır.
- Denemeler denek başına kaydedilir ve yalnızca eklenir; tekrarlanan bir deneme
  bir sonraki deneme numarasını alır.
- Farklı gruplardaki denekler aynı oturumu paylaşabilir. Bir ölçümün grubu her
  zaman deneğin atamasından gelir; grupla ilgili hiçbir şey teste kopyalanmaz.

## Video, kalibrasyon ve analiz

- **Video:** bir testin doğrulanmış tek bir güncel videosu vardır. Yeni bir
  yükleme onun yerini alır; önceki videolar köken kaydı için saklanır.
- **Kalibrasyon:** pikselleri santimetreye çevirir. Varsayılan olarak her arena
  (ortam revizyonu) düzenek kurulurken bir kez kalibre edilir; kırpma alanı
  arenanın kamera karesindeki bölgesidir. Kamera veya arena kaydıysa güncel
  videoda tek bir test deneğine elle bir **düzeltme** uygulanır. Parametrelerden
  türetilmeyen bölgeler (yeni ve tanıdık nesne, direk ve tabanı, koşu bandının
  şok ızgarası) **kalibre edilen bölgelerdir**: kalibrasyonla birlikte taşınır,
  kare üzerinde piksel olarak çizilir ve santimetre olarak da saklanır. Yandan
  görünüşler (direk, koşu bandı) `APPARATUS_SIDE` düzlemini kullanır. Bir
  kalibrasyon ve bölgeleri asla değişmez. Her paradigma kalibrasyon gerektirir.
- **Analiz çalıştırması:** test deneği başına bir tane. Bir worker deneğin
  ağırlık merkezini arenasının kırpma alanında izler ve yörüngeyi yükler; Mişko
  her olayı ve metriği örneklerden ve bölgelerden kendisi türetir. Gözlemci
  puanlaması yoktur: bir kişi yalnızca sonucu inceler. Girdiler sabitlenir;
  yeniden analiz yeni bir çalıştırmadır ve eski çalıştırmalar kalır.
- **Onaylanmış sonuç:** her sonucu bir kişi onaylar. Bir deneğin ilk başarılı
  çalıştırması `PENDING` bir sonuç açar; inceleyen kişi kabul eder ya da bir
  gerekçeyle reddeder. Kör bir çalışmada inceleyen kişi grubu, hastalığı, maddeyi
  veya dozu görmez. İstatistiklere yalnızca kabul edilen sonuçlar girer.

Davranışsal sonuç **veridir, verdikt değil**: bir testin geçti veya kaldı sonucu
yoktur.

## Rapor

Deney birimi denektir. Her sonlanım noktası için rapor, dışlanmamış deneklerin
kabul edilmiş sonuçlarını alır ve grup başına n, ortalama, SD, SEM ve başlangıca
göre değişimi, ayrıca grup rollerinden çıkan karşılaştırmaları verir:

- model geçerliliği: hastalık kontrolü ile sağlıklı kontrol ve sham;
- test duyarlılığı: pozitif kontrol ile hastalık kontrolü;
- etkinlik: her tedavi grubu ile kendi hastalık kontrolü, kurtarma yüzdesiyle;
- doz yanıtı: tek bir maddenin doza göre sıralanmış tedavi grupları.

Farklı paradigma veya metrik motoru sürümlerinin sonuçları hiç birleştirilmez.

## Yorum (test tartışması)

`Comment`, bir `Test`'e iliştirilen serbest metin notudur ve bir tartışma alanı
oluşturur.

- `testId`, `authorId`, `body` (düz metin), `createdAt`, `updatedAt`.
- `body` olduğu gibi saklanır ve **asla** HTML olarak yorumlanmaz; API
  katmanında uzunluğu sınırlanır (en fazla 2000 karakter), kırpılır ve kontrol
  karakterlerinden arındırılır.

Yetkilendirme bilinçli olarak basittir ve izin matrisinin parçası değildir:

- **okuma / oluşturma**: giriş yapan herhangi bir kullanıcı (yani her rol yorum
  yazabilir).
- **düzenleme**: yalnızca yazarın kendisi.
- **silme**: yazar veya moderasyon için yetkili bir rol (`SUPERADMIN` /
  `LAB_MANAGER`).

Yorum yazma işlemleri, hafif bir kötüye kullanım önlemi olarak kullanıcı başına
hız sınırlıdır (dakikada 20).

## Roller & izinler (RBAC)

**Kodda tanımlı izin matrisi** olan beş lab odaklı rol (DB'den düzenlenebilir izin
yok):

| İzin \ Rol | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|:--:|:--:|:--:|:--:|:--:|
| `user:manage`     | ✅ | ✅ | - | - | - |
| `lab:configure`   | ✅ | ✅ | - | - | - |
| `study:write` (katalog, tasarım, atama, protokoller) | ✅ | ✅ | ✅ | - | - |
| `subject:write`   | ✅ | ✅ | ✅ | - | - |
| `weight:write`    | ✅ | ✅ | ✅ | ✅ | - |
| `apparatus:write` (ortamlar, arena kalibrasyonu) | ✅ | ✅ | ✅ | - | - |
| `test:write` (test planlama) | ✅ | ✅ | ✅ | - | - |
| `test:run` (test çalıştırma, videolar, dozlar, onay) | ✅ | ✅ | ✅ | ✅ | - |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

Her kullanım senaryosu bir store'a dokunmadan önce izni kendisi kontrol eder
(`actor.Require(...)`).

## Veritabanının dışında kalanlar

Video dosyaları, analiz videoları ve yörüngeler özel nesne depolamasında durur;
veritabanı bucket, nesne adı ve generation bilgisini tutar, video baytlarını veya
imzalı URL'leri asla tutmaz. Paradigma tanımları satır değil, koddur.
