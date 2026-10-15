---
title: Go backend (önizleme)
description: Go backend'in kurulumu ve kullanımı; giriş, kullanıcılar, roller, katalog, çalışma tasarımı, dozlama, test oturumları, videolar, kalibrasyon, analiz onayı ve çalışma raporu.
---

:::caution
Bu sayfa henüz yayınlanmamış Go backend'i anlatır. Vue paneli bu API'ye hâlâ
taşınmaktadır; Kurulum ve Kullanım sayfaları mevcut sürümü anlatmaya devam eder.
Canlıya almayın.
:::

## Şu an neler var

| Alan | Durum |
|------|-------|
| Sağlık ve hazır olma kontrolleri | Hazır |
| Giriş, mevcut kullanıcı, kendi şifresini değiştirme | Hazır |
| Beş rolle kullanıcı yönetimi | Hazır |
| Laboratuvar ayarları (kurulum başına tek laboratuvar) | Hazır |
| Madde ve hastalık modeli kataloğu | Hazır |
| Denekler ve vücut ağırlıkları | Hazır |
| Deneyler: rollü gruplar, rejimler, zaman noktaları, sonlanım noktaları, atama ve dışlama | Hazır |
| Dozlama: uygulamalar, günlük doz takvimi, indüksiyon sonuçları | Hazır |
| On bir paradigmanın metrik tanımlarıyla paradigma kataloğu | Hazır |
| Ölçüm revizyonlarıyla ortamlar ve deney test protokolleri | Hazır |
| Arenalarda bir veya birden çok hayvanla oturum olarak testler, denemeler ve yorumlar | Hazır |
| Google Cloud Storage'a video yükleme, test başına tek güncel video | Hazır, gerçek depolama testi bekliyor |
| Arena başına kalibrasyon ve hayvan başına düzeltme | Hazır |
| Hayvan başına analiz çalıştırmaları ve her sonucun insan onayı | Hazır; worker takip edilen yedi paradigmanın hepsini izler, yalnızca sentetik videoyla test edildi |
| Rotarod, direk, koşu bandı ve yeni nesne için gözlemci puanlaması | Hazır |
| Rol tabanlı karşılaştırmalarla çalışma raporu, CSV dışa aktarma, gösterge paneli | Hazır |
| Yeni API üzerinde Vue paneli | İskeleti var, tamamlanmadı |

## Kurulum

Docker ve Docker Compose gerekir. Komutları reponun kök dizininde çalıştırın:

```bash
docker compose up -d db
docker compose --profile setup run --rm migrate
docker compose --profile setup run --rm setup
docker compose up --build -d backend jobs
curl --fail http://127.0.0.1:4000/api/ready
```

1. `db`, PostgreSQL'i yeni bir volume ile başlatır. Eski kurulumun verisine dokunulmaz.
2. `migrate`, boş bir veritabanında tabloları oluşturur ya da mevcut bir veritabanında eksik migration'ları uygular. Güncel bir veritabanında hiçbir şey yapmaz.
3. `setup`, laboratuvarı ve ilk yöneticiyi oluşturur; yöneticinin e-postasını ve
   üretilen şifreyi terminalinize **yalnızca bir kez** yazar. Tekrar
   çalıştırılırsa hiçbir şey oluşturmaz.
4. `backend`, API'yi `127.0.0.1:4000` adresinde başlatır; `jobs` analiz
   çalıştırmalarını kuyruğa alır.

`setup` terminal olmadan çalışırsa (örneğin CI içinde) `-credentials-file YOL`
verilmedikçe başlamayı reddeder. Bu durumda şifre, yalnızca sahibinin
okuyabildiği yeni bir dosyaya yazılır. Şifre hiçbir zaman loglara yazılmaz.

Compose veritabanı yerine kendi PostgreSQL sunucunuzu kullanıyorsanız sürüm
PostgreSQL 18 veya üzeri olmalıdır. `migrate` komutunu çalıştıran kullanıcının
veritabanında şema oluşturma izni olmalıdır.

### Şema yenilenmeden önceki veritabanları

Veritabanı şeması, aşağıda anlatılan çalışma modeli için sıfırdan yeniden
yazıldı. `migrate`, önceki migration'larla kurulmuş bir veritabanını reddeder ve
içinde hiçbir şeyi değiştirmez. Böyle bir veritabanı sıfırlanmalıdır: silip
yeniden oluşturun ya da Compose ile `docker compose down -v` komutuyla volume'u
kaldırın, ardından `migrate` ve `setup` komutlarını yeniden çalıştırın. Önceki
veri taşınmaz.

### Ayarlar

Compose dosyasındaki değerler yalnızca geliştirme içindir. Gerçek bir kurulumda
kendi değerlerinizi verin:

| Değişken | Varsayılan | Anlamı |
|----------|------------|--------|
| `JWT_SECRET` | geliştirme değeri | Giriş token'larını imzalayan anahtar. En az 32 rastgele bayt kullanın. |
| `TOKEN_TTL` | `12h` | Bir girişin geçerlilik süresi; 5 dakika ile 7 gün arası. |
| `BCRYPT_COST` | `12` | Şifre hash gücü; 10 ile 14 arası. |
| `ADMIN_EMAIL` | `admin@misko.local` | `setup` ile oluşturulan ilk yöneticinin e-postası. |
| `LAB_NAME` | `Misko Laboratory` | Laboratuvar adı. |
| `LAB_TIMEZONE` | `UTC` | Laboratuvarın saat dilimi; `Europe/Istanbul` gibi bir IANA adı. Gün 0, doz günleri ve varsayılan test saatleri buna göre hesaplanır. |

Veritabanı, depolama ve zaman aşımı ayarları dahil tüm değişkenler backend README dosyasındadır.

## Kullanım

### Giriş

```bash
curl -X POST http://127.0.0.1:4000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@misko.local","password":"SETUP_CIKTISINDAKI_SIFRE"}'
```

Yanıtta bir `token` bulunur. Diğer tüm isteklerde bunu
`Authorization: Bearer TOKEN` başlığıyla gönderin. `GET /api/auth/me`
kullanıcınızı ve rolünüzün izinlerini döndürür. Üretilen şifreyi hemen
`POST /api/auth/password` (`currentPassword`, `newPassword`) ile değiştirin.
Yanıt yeni bir token içerir ve eski token'ların hepsi geçersiz olur.

Şifreler en az 8 karakter ve en fazla 72 bayt olmalıdır. ş veya ğ gibi İngilizce
dışı harfler birden fazla bayt kaplar.

### Roller

| Rol | Yapabilecekleri |
|-----|-----------------|
| SUPERADMIN, LAB_MANAGER | Kullanıcı, laboratuvar ayarları ve analiz worker'ları yönetimi dahil her şey |
| RESEARCHER | Çalışma tasarlamak; kataloğu, denekleri, düzenekleri ve protokolleri yönetmek; test planlamak ve çalıştırmak; her şeyi okumak |
| TECHNICIAN | Ağırlık kaydetmek, test çalıştırmak, doz ve indüksiyon sonucu kaydetmek, bir testteki tek hayvanı kalibre etmek, analiz sonuçlarını onaylamak; her şeyi okumak |
| VIEWER | Yalnızca okuma |

| İzin | Kapsamı |
|---|---|
| `study:write` | Katalog, deney tasarımı ve yaşam döngüsü, atama ve dışlama, protokoller |
| `apparatus:write` | Ortamlar, revizyonları ve arena kalibrasyonları |
| `subject:write` | Denekler |
| `weight:write` | Vücut ağırlıkları |
| `test:write` | Test planlamak: oluşturmak, hayvan eklemek veya çıkarmak, iptal etmek |
| `test:run` | Test ve deneme çalıştırmak, videolar, hayvan başına kalibrasyon düzeltmesi, dozlar, indüksiyon sonuçları, elle analiz çalıştırması, sonuç onayı |
| `*:read` | Her şeyi okumak |

Rol değişikliği kullanıcının bir sonraki isteğinde geçerli olur. Kullanıcı
silindiğinde oturumu anında kapanır.

### Kullanıcı yönetimi

Bu uç noktaları yalnızca SUPERADMIN ve LAB_MANAGER kullanabilir:

- `GET /api/users`, kullanıcıları `search`, `role`, `sort`, `order`, `page` ve `pageSize` ile listeler.
- `POST /api/users`, `email`, `name`, `role` ve isteğe bağlı `password` ile
  kullanıcı oluşturur. Şifre verilmezse güçlü bir şifre üretilir ve bir kez döndürülür.
- `PATCH /api/users/{id}`, `name` veya `role` değiştirir.
- `POST /api/users/{id}/reset-password`, yeni şifre belirler (ya da üretir) ve o
  kullanıcının tüm oturumlarını kapatır.
- `DELETE /api/users/{id}`, kullanıcıyı siler.

İki güvenlik kuralı her zaman geçerlidir. Kendi hesabınızı silemezsiniz; son
SUPERADMIN veya LAB_MANAGER silinemez ya da daha düşük bir role alınamaz. Herkese
açık kayıt yoktur.

### Laboratuvar ayarları

Giriş yapmış her kullanıcı `GET /api/lab` ile okuyabilir. SUPERADMIN ve
LAB_MANAGER, `PATCH /api/lab` ile `name`, `code` ve `timezone` alanlarını
değiştirebilir. Boş `code` değeri kodu temizler. `GET /api/meta` herkese açıktır
ve giriş ekranı için laboratuvar adını döndürür.

### Katalog

Maddeler ve hastalık modelleri laboratuvarın tüm çalışmalarında ortaktır.
RESEARCHER ve üstü roller bunları yönetir; herkes okuyabilir.

- `POST /api/catalog/substances` (`name`, `kind`, isteğe bağlı `description`).
  `kind` değeri `COMPOUND`, `REFERENCE_DRUG`, `VEHICLE` veya `INDUCER` olur;
  yalnızca seçim listelerine yardım eder. Bir maddenin çalışmadaki görevi onu
  kullanan rejimden gelir.
- `POST /api/catalog/disease-models` (`name`, `CHEMICAL`, `SURGICAL`, `DIET`,
  `STRESS` veya `OTHER` değerli `inductionType`, isteğe bağlı
  `confirmationCriterion` ve `description`). Her hastalık modeli indüklenir;
  indükleyen madde ve dozu her çalışmanın rejiminde yer alır.
- İki liste de `search` alır; maddeler `kind`, hastalık modelleri
  `inductionType` ile süzülür. Bir kaydı `PATCH` ile değiştirin. Kullanımdaki
  bir kayıt silinemez (`substance.inUse`, `diseaseModel.inUse`).

### Denekler ve ağırlıklar

Giriş yapmış her kullanıcı denekleri okuyabilir. SUPERADMIN, LAB_MANAGER ve
RESEARCHER denekleri değiştirebilir.

- `POST /api/subjects`, `code`, `species` (`MOUSE` veya `RAT`), `sex` (`FEMALE`,
  `MALE` veya `UNKNOWN`) ile denek oluşturur. Kodlar büyük/küçük harf farkı gözetmeden benzersizdir.
- `GET /api/subjects`, denekleri `search`, `species`, `sex`,
  `unallocated=true`, `sort`, `order`, `page` ve `pageSize` ile listeler. Her
  denek kendi `allocation` bilgisini (deney, grup ve dışlanıp dışlanmadığı) ya da
  null gösterir.
- `PATCH /api/subjects/{id}`, gönderdiğiniz alanları değiştirir.
- `DELETE /api/subjects/{id}`, bir gruba atanmamış ve ağırlık kaydı olmayan bir deneği siler.
- TECHNICIAN ve üstü roller vücut ağırlığını `POST /api/subjects/{id}/weights`
  (`grams`, `measuredAt`) ile kaydeder. Ağırlıklar hayvana aittir ve değişmez.

Bir hayvan bir çalışmaya katılmadan önce kaydedilebilir ve yalnızca bir
çalışmaya katılır.

### Deneyler

Bir deney taslak olarak tasarlanır, başlatılır ve tamamlanır. Giriş yapmış her
kullanıcı deneyleri okuyabilir. SUPERADMIN, LAB_MANAGER ve RESEARCHER deney
tasarlar, başlatır ve hayvanları gruplara atar.

1. Deneyi `POST /api/experiments` (`code`, `title`, isteğe bağlı
   `description`, `hypothesis`, `randomisation` ve `blinded`) ile oluşturun.
   Deney `DRAFT` olarak başlar.
2. Grupları `POST /api/experiments/{id}/groups` (`G1` gibi `code`, `name`,
   `role`, `diseaseModelId`, `targetSize`) ile ekleyin. Rol şunlardan biridir:

   | Rol | Hastalık modeli | Aldığı | Amacı |
   |---|---|---|---|
   | `HEALTHY_CONTROL` | yok | varsa taşıyıcı | normal referans |
   | `SHAM` | gerekli | indüksiyon yerine taşıyıcı | indüksiyon işleminin kendisini kontrol eder |
   | `DISEASE_CONTROL` | gerekli | indüksiyon ve taşıyıcı | hastalığın yarattığı bozukluk |
   | `POSITIVE_CONTROL` | gerekli | indüksiyon ve referans ilaç | testin iyileşmeyi yakalayabildiğini gösterir |
   | `TREATMENT` | gerekli | indüksiyon ve test edilen madde | etkinlik ve doz yanıtı |

3. Her grubun ne aldığını `POST /api/experiments/{id}/regimen-items`
   (`groupId`, `INDUCTION`, `VEHICLE` veya `TREATMENT` değerli `purpose`,
   `substanceId`, `amount`, `unit`, `route`, `ONCE`, `DAILY`, `TWICE_DAILY`,
   `WEEKLY` veya `intervalDays` ile `EVERY_N_DAYS` değerli `frequency`,
   `startDay`, `endDay`) ile tanımlayın. Günler gün 0'dan sayılan çalışma
   günleridir. Bir grubun en fazla bir `TREATMENT` kalemi olur; böylece bir
   tedavi grubu tek bir maddeyi tek bir dozda alır. Aynı maddenin farklı
   dozlardaki tedavi grupları doz yanıtını oluşturur.
4. Zaman noktalarını `POST /api/experiments/{id}/timepoints` (`name`,
   `studyDay`, `position`) ile ekleyin; örneğin gün -1'de Baseline, gün 14'te
   Day 14.
5. Bir protokol ekleyin (aşağıya bakın), ardından çalışmanın neye göre
   değerlendirileceğini `POST /api/experiments/{id}/endpoints` (`timepointId`,
   `paradigmKey`, `metricKey`, `PRIMARY` veya `SECONDARY` değerli `kind`,
   `HIGHER_IS_BETTER` veya `LOWER_IS_BETTER` değerli `direction`, isteğe bağlı
   `baselineTimepointId`) ile seçin. `GET /api/experiments/{id}/metric-options`
   protokoldeki paradigmaların ölçebildiği metrikleri listeler. En fazla bir
   birincil sonlanım noktası olabilir.
6. Tasarımı, karşılanmayan her kuralı listeleyen
   `GET /api/experiments/{id}/readiness` ile kontrol edin, ardından
   `POST /api/experiments/{id}/start` (`dayZero`, bir tarih) ile başlatın.
   Başlatmak için en az bir zaman noktası ve bir grup, her grupta hedef sayı, bir
   birincil sonlanım noktası ve her role uyan bir rejim gerekir: tedavi veya
   pozitif kontrol grubunda bir tedavi ve bir indüksiyon, hastalık kontrolünde
   bir indüksiyon ve bir taşıyıcı, sham grubunda bir taşıyıcı olup indüksiyon
   olmaması, sağlıklı kontrolde indüksiyon ve tedavi olmaması; ayrıca her sham,
   tedavi ve pozitif kontrol grubunun aynı hastalık modeline sahip bir hastalık
   kontrolü olmalıdır.
7. Tüm testler tamamlandığında veya iptal edildiğinde çalışmayı
   `POST /api/experiments/{id}/complete` ile kapatın. Tamamlanan deney salt
   okunurdur.

Başlatmak grupları, rejim kalemlerini, zaman noktalarını ve sonlanım
noktalarını dondurur. Kod, başlık ve açıklama tamamlanana kadar
düzenlenebilir.

### Atama ve dışlama

- Bir hayvanı `POST /api/experiments/{id}/allocations` (`subjectId`, `groupId`,
  isteğe bağlı `cage`) ile bir gruba atayın ya da bunu
  `POST /api/experiments/{id}/allocations/randomise` (`subjectIds`, isteğe bağlı
  `groupIds` ve `cage`) ile sisteme bırakın. Rastgele atama grupları hedef
  sayılarına kadar dengeli doldurur ve ya tüm hayvanları atar ya da hiçbirini.
- Bir hayvan kalıcı olarak tek bir deneyin tek bir grubuna aittir. Deney
  taslakken hayvanı başka gruba taşıyabilir ya da çıkarabilirsiniz. Çalışma
  başladıktan sonra yalnızca kafesi değişebilir.
- Bir hayvan çalışmadan ayrılırsa
  `POST /api/experiments/{id}/allocations/{allocationId}/exclusion` (`DIED`,
  `HUMANE_ENDPOINT`, `INDUCTION_FAILED`, `TECHNICAL` veya `OTHER` değerli
  `reason`, isteğe bağlı not) ile dışlayın. Dışlanan hayvan silinmez: raporda
  görünmeye devam eder, yeni bir teste katılmaz ve yeniden analiz edilmez.
- `GET /api/experiments/{id}/allocations` hayvanları (`groupId` veya `excluded`
  ile süzülerek) ve her grubun hedefe göre doluluğunu listeler.

### Dozlama

TECHNICIAN ve üstü roller, deney çalışırken gerçekten verileni kaydeder.

- `GET /api/experiments/{id}/dosing?date=YYYY-MM-DD`, o gün verilmesi gereken
  her dozu rejim kalemlerinden ve gün 0'dan hayvan bazında listeler; kaçının
  verildiğini ve hayvanın son ağırlığına göre dozu gösterir. `date` verilmezse
  laboratuvarın saat dilimindeki bugün kullanılır.
- `POST /api/experiments/{id}/allocations/{allocationId}/administrations` bir
  dozu kaydeder. `regimenItemId` verilirse madde, doz ve uygulama yolu plandan
  gelir; gerçekte verilen farklı bir miktar veya yol girilebilir. Plansız bir doz
  hepsini belirtir. Kilogram başına dozlar (`mg/kg`, `ug/kg`, `mL/kg`,
  `IU/kg`) aynı hayvanın son 7 gün içindeki bir ağırlığını gerektirir;
  `weightMeasurementId` verilmezse en son ağırlık kullanılır ve yanıt mutlak dozu
  gösterir. Hastalık indüksiyonu da aynı şekilde, `INDUCTION` amacıyla kaydedilir.
- `POST /api/experiments/{id}/allocations/{allocationId}/induction-outcome`
  hayvanın hastalık modelinin `CONFIRMED` ya da `NOT_CONFIRMED` olduğunu bir kez
  kaydeder.
- `GET /api/experiments/{id}/administrations`,
  `GET /api/subjects/{id}/administrations` ve
  `GET /api/experiments/{id}/induction-outcomes` kayıtları listeler.

Miktarları ondalık metin olarak yazın, örneğin `"0.25"`. Kayıtlar, kaydedildikleri
andaki adları, dozları ve ağırlıkları saklar ve hiç değişmez.

### Paradigma kataloğu

Giriş yapmış her kullanıcı paradigma kataloğunu okuyabilir. Paradigmalar kodda
tanımlıdır; API üzerinden oluşturulamaz, düzenlenemez veya silinemez.

- `GET /api/paradigms`, paradigmaları yayınlanmış sürümleriyle ve otomatik video
  analizinin olup olmadığıyla (`automatedAnalysis`) listeler. On bir paradigma
  yayınlanmıştır: `BARNES_MAZE`, `EPM`, `LIGHT_DARK`, `MWM`, `NOVEL_OBJECT`,
  `OPEN_FIELD`, `POLE`, `ROTAROD`, `THREE_CHAMBER`, `TREADMILL` ve `Y_MAZE`.
  Yedisi bir analiz worker'ı tarafından takip edilir (`OPEN_FIELD`, `EPM`,
  `Y_MAZE`, `BARNES_MAZE`, `THREE_CHAMBER`, `MWM`, `LIGHT_DARK`) ve paradigma
  sürümü için bir worker kaydedildiğinde bunlarda otomatik analiz vardır. Diğer
  dördü (`ROTAROD`, `POLE`, `TREADMILL`, `NOVEL_OBJECT`) videoda bir gözlemci
  tarafından puanlanır (aşağıdaki Gözlemci puanlaması bölümüne bakın).
- `GET /api/paradigms/{key}` en son sürümü, `GET /api/paradigms/{key}/versions/{version}`
  ise belirli bir sürümü döndürür. Her sürüm; parametrelerini kapsamı (ortamda
  `APPARATUS` ve `ANALYSIS`, protokol adımında `SESSION`), etiketi, yardım metni,
  birimi, izin verilen aralığı ve varsayılanıyla; bölgelerini ve olaylarını; her
  metrik için de sade bir açıklamayı, tanımı, formülü, girdileri ve veri eksik
  olduğunda ne olacağını listeler. Adlar, etiketler ve açıklamalar `?lang=` ya da
  `Accept-Language` ile (`en` veya `tr`) seçilir.

Bazı paradigmalar izlenen bir konumdan fazlasına ihtiyaç duyar:

- Yükseltilmiş artı labirent kolları veya Barnes labirenti delikleri gibi
  `calibrated` işaretli bölgeler daire ya da çokgen olarak çizilir. Su labirenti
  platformu gibi diğer bölgeler düzenek parametrelerinden gelir.
- `inputEvents`, izlenmek yerine puanlanan gözlemleri listeler; örneğin rotarod
  düşüşü, pole testinde dönüşler veya yeni nesne keşfi. Puanlanmamış bir olay
  türü sıfır değil, eksik metrik üretir.

Sonuçlar her zaman hesaplandıkları paradigma sürümünü, metrik motoru sürümünü ve
sonuç şeması sürümünü belirtir. Hesaplanamayan bir metrik hiçbir zaman sıfır
olarak değil, bir nedenle eksik olarak raporlanır. Örneğin kaçışın olmadığı bir
probe denemesi `EVENT_NOT_OBSERVED`, üçten az kol girişi olan bir Y labirenti
denemesi `INSUFFICIENT_ENTRIES` döndürür. Yayınlanmış bir sürüm hiç değişmez;
değişen bir tanım yeni bir sürüm olarak yayınlanır.

### Ortamlar ve protokoller

Ortam, tek bir paradigma için kurulmuş tek bir fiziksel düzenektir; örneğin belirli
bir su tankı ya da tek kameranın gördüğü birkaç açık alan kutusundan biri. Bir
testte her hayvan bir arenaya konur; arena, bir ortamın bir revizyonudur.
RESEARCHER, LAB_MANAGER ve SUPERADMIN ortam ve protokol oluşturup
güncelleyebilir; giriş yapmış her kullanıcı bunları okuyabilir.

- `POST /api/environments`, ilk revizyonuyla birlikte bir ortam oluşturur.
  Paradigma sürümünün tüm düzenek ölçülerini `parameters` içinde verin; örneğin
  `MWM` için tank ve platform boyutları. İsteğe bağlı olarak analiz ayarlarını da
  verebilirsiniz; verilmeyen analiz ayarları varsayılan değerleri alır. İzin
  verilen aralık dışındaki değerler veya platformun tank dışında kalması gibi
  tutarsız değerler reddedilir.
- Düzenek değiştiğinde `POST /api/environments/{id}/revisions` ile yeni bir
  revizyon ekleyin. Önceki revizyonlar hiç değişmez ve silinemez. Ortamın adını ve
  notlarını yine de düzenleyebilirsiniz. `GET /api/environment-revisions` arena
  seçimi için tüm revizyonları listeler.

Protokol, bir deneyin test bataryasıdır. Paradigmaları birleştirebilir; örneğin
açık alan testinin ardından yükseltilmiş artı labirent.

- `POST /api/experiments/{id}/protocols`, ilk sürümüyle birlikte bir protokol
  oluşturur. Her adım sırasını, paradigmayı ve sürümünü, deneme türünü, deneme
  sayısını, denemeler arasındaki bekleme süresini saniye olarak ve oturum
  parametrelerini verir. Adım bir düzenek belirtmez: arena her testte hayvan
  başına seçilir.
- Adımları 1'den başlayarak boşluksuz numaralandırın. Paradigmanın tanımlamadığı
  bir deneme türü ve aralık dışındaki oturum değerleri reddedilir. Verilmeyen
  oturum parametreleri paradigma varsayılanlarını alır ve kaydedilen sürüm
  bunları gösterir.
- Bir protokolü değiştirmek için
  `POST /api/experiments/{id}/protocols/{protocolId}/versions` ile yeni bir sürüm
  ekleyin. Kaydedilmiş sürümler hiç değişmez.

### Testler, denemeler ve yorumlar

Test, çalışan bir deneyin tek bir oturumudur: bir zaman noktası, bir protokol
adımı ve bir video. Her biri kendi arenasında olan bir veya daha fazla hayvanı
kaydeder; genellikle bir, bazen tek kamera altında en fazla üç kutu. Farklı
gruplardaki hayvanlar aynı oturumu paylaşabilir. Her ölçümün grubu hayvanın
atamasından gelir.

- RESEARCHER, LAB_MANAGER ve SUPERADMIN testleri
  `POST /api/experiments/{id}/tests` ile planlar: zaman noktası, protokol sürümü
  ve adımı, isteğe bağlı planlanan zaman ve `subjects`, yani `allocationId` ve
  `environmentRevisionId` çiftlerinden oluşan bir liste (en fazla 12). Zaman
  verilmezse test, gün 0 artı zaman noktasının çalışma gününe planlanır. Her
  arena adımın paradigmasını çalıştırmalıdır ve bir hayvan aynı zaman noktası ve
  adımın iki testinde yer alamaz.
- Test planlıyken `POST /api/tests/{id}/subjects` ile hayvan ekleyin ya da
  `DELETE /api/tests/{id}/subjects/{subjectId}` ile çıkarın. Son hayvan
  çıkarılamaz; bunun yerine testi iptal edin.
- TECHNICIAN ve üstü roller testleri çalıştırır: `POST /api/tests/{id}/start`,
  ardından her hayvanın denemelerini
  `POST /api/tests/{id}/subjects/{subjectId}/trials` ile kaydedin, sonra
  `POST /api/tests/{id}/complete`. Her hayvanın en az bir denemesi olmalıdır. Bir
  deneme tekrarlanacaksa aynı tekrarı yeniden kaydedin; bir sonraki deneme
  numarasını alır ve önceki deneme korunur.
- Tamamlanmamış bir test bir gerekçeyle iptal edilebilir. Dışlanmış bir hayvanı
  olan test, o hayvan çıkarılana kadar başlatılamaz.
- `GET /api/tests`, deneyler arası iş listesidir; deney, durum, zaman noktası ve
  tarih aralığıyla süzülür.

Giriş yapmış herkes, izleyiciler dahil, bir teste yorum yazabilir. Yalnızca kendi
yorumlarınızı düzenleyebilirsiniz. LAB_MANAGER ve SUPERADMIN her yorumu
silebilir. Her kullanıcı dakikada en fazla 20 yorum yazabilir veya düzenleyebilir.

### Videolar

Videolar özel bir Google Cloud Storage bucket'ında saklanır. Bir yönetici
`GCS_BUCKET` değerini (workload identity kullanılıyorsa `GCS_SIGNER_EMAIL`
değerini de) ayarlayana kadar video rotaları `media.storageNotConfigured` koduyla
503 döndürür. Bucket, CORS ve izin kurulumu için `backend/README.md` dosyasına
bakın.

Bir testin, tüm hayvanlarını gösteren tek bir videosu vardır. Videolar test
başladıktan sonra eklenebilir.

- TECHNICIAN ve üstü roller yüklemeyi `POST /api/tests/{id}/recordings` ile
  başlatır; dosya adı, türü (MP4, MOV veya WebM), boyutu ve CRC32C sağlamasını
  verir. Yanıt imzalı bir istek içerir; tarayıcı bununla dosyayı doğrudan
  depolamaya yükler.
- Yükleme bittikten sonra aynı kişi (ya da LAB_MANAGER veya SUPERADMIN)
  `POST /api/tests/{id}/recordings/{recordingId}/finalize` çağırır. Sunucu
  depodaki dosyayı kontrol eder. Uyuşmayan dosya, `CHECKSUM_MISMATCH` gibi bir
  gerekçeyle `REJECTED` olarak işaretlenir.
- Yeni bir yükleme testin videosunun yerini alır. Önceki video köken kaydı için
  saklanır; `GET /api/tests/{id}/recordings/current` doğrulanmış en yeni videoyu
  döndürür.
- Giriş yapmış herkes doğrulanmış bir video için
  `GET /api/tests/{id}/recordings/{recordingId}/read-url` ile kısa süreli bir
  bağlantı alabilir. Oynatıcılar bu bağlantıda ileri geri sarabilir.

### Kalibrasyon

Bir videonun mesafelere ve bölgelerdeki sürelere dönüşebilmesi için sistemin her
arenada kamera piksellerinin santimetreye nasıl karşılık geldiğini bilmesi
gerekir.

- Kalibrasyon arenaya aittir. Düzenek kurulurken düzenekleri değiştirme yetkisi
  olan biri sabit kameradan bir kare açar ve her ortam revizyonunu bir kez
  `POST /api/environments/{id}/revisions/{number}/calibrations` ile kalibre
  eder. Kırpma alanı o arenanın karedeki bölgesidir; böylece tek kamera altındaki
  üç arenanın her biri kendi kalibrasyonunu alır. O arenada test edilen her
  hayvan bunu kullanır.
- Kamera ve arena kayabilir. Bunu henüz hiçbir şey kontrol etmez: arena
  kalibrasyonunu kullanan bir hayvanın durumu `drift: UNCHECKED` gösterir. Bir
  video uyuşmuyorsa TECHNICIAN (veya üstü bir rol) o tek hayvanı testin güncel
  videosunda `POST /api/tests/{id}/subjects/{testSubjectId}/calibrations` ile
  elle kalibre eder. Düzeltme, arena kalibrasyonunun yerini yalnızca o hayvan
  için alır; reddedilirse hayvan arenaya geri dönmek yerine bekler. Yeni bir video
  yeniden arena kalibrasyonlarıyla başlar.
- İki tür de aynı noktaları alır: en az 4 FIT noktası ve en az 3 ayrı CHECK
  noktası; her biri piksel konumu ve santimetre cinsinden gerçek konumuyla. FIT
  noktaları dönüşümü belirler, CHECK noktaları onu sınar. Bir CHECK noktası
  paradigmanın izin verdiğinden fazla saparsa (yörünge paradigmaları için 2 cm)
  kalibrasyon `REJECTED` olarak kaydedilir.
- `calibrated` bölgeleri olan paradigmalar (yükseltilmiş artı labirent, Y
  labirenti, Barnes labirenti, üç bölmeli test) bu bölgelerin aynı kare üzerinde
  çizilmesini de ister. Bunları piksel cinsinden `zones` olarak gönderin; örneğin
  `{"key": "open_arms", "polygon": [{"x": 300, "y": 480}, ...]}` ya da
  `{"key": "center", "circle": {"x": 960, "y": 540, "radius": 60}}`. Bir
  anahtarın iki açık kol gibi birden çok şekli olabilir. Her zorunlu bölgenin bir
  şekli olmalı ve her şekil karenin içinde kalmalıdır. Misko bunları kalibrasyonla
  santimetreye çevirir ve her bölgeyi iki birimde de döndürür. Worker bölgeleri
  işiyle birlikte alır; metrikler santimetre şekilleriyle hesaplanır. Panelde
  görsel kalibratör noktaları, kırpma alanını ve bölgeleri bir videonun karesi
  üzerine yerleştirir: ortam varsayılanı için bilgisayarda seçilen bir video
  dosyası (tarayıcıda okunur, asla yüklenmez), düzeltme için testin güncel
  videosu.
- Dünya noktaları, paradigmanın bölgelerinin tanımlandığı çerçeveyi kullanmalıdır.
  Su labirentinde bu çerçeve tank merkezlidir: (0, 0) tankın merkezi, x doğuyu,
  y kuzeyi gösterir; duvar noktaları r = tank çapı / 2 olmak üzere (r, 0),
  (0, r), (-r, 0) ve (0, -r) olur. Açık alan bir taban köşesinden başlayan arena
  yerel noktaları, aydınlık/karanlık aydınlık taraftaki bir köşeden başlayıp x'i
  karanlık uç duvara doğru ilerleyen kutu yerel noktaları, kalibre edilen
  bölgeleri olan paradigmalar ise herhangi bir çerçeveyi kullanır.
- Bir kalibrasyonu düzeltmek için, `supersedesId` alanına aynı arenanın ya da
  hayvanın en son kalibrasyonunu yazarak yeni bir kalibrasyon gönderin. Önceki
  kalibrasyonlar saklanır.
- `GET /api/tests/{id}/calibration-status` bir testin tüm hayvanlarını
  listeler: `WAITING_FOR_VIDEO`, her zorunlu bölgesi çizilmiş geçerli bir
  kalibrasyon uygulanana kadar `WAITING_FOR_CALIBRATION`, sonra `CALIBRATED`. Rotarod gibi video ölçümü
  olmayan paradigmalar `NOT_REQUIRED` gösterir.

### Video analizi

Takip, API ile konuşan ayrı analiz worker'larında çalışır. `worker/`
klasöründeki worker, takip edilen her paradigmanın sabit kameradan kaydedilmiş
videosunu izler: `OPEN_FIELD`, `EPM`, `Y_MAZE`, `BARNES_MAZE`,
`THREE_CHAMBER`, `MWM` ve `LIGHT_DARK`, sürüm 1. Şimdiye kadar yalnızca
sentetik videoyla test edildi; elle işaretlenmiş bir kayda karşı doğrulanana
kadar sonuçlarını araştırmada kullanmayın.

- LAB_MANAGER bir worker'ı `POST /api/analysis/workers` ile kaydeder; adını, model
  sürümünü ve desteklediği paradigma sürümlerini verir. Paketle gelen worker'ı
  `misko-tracker-bgsub 2.0.0` model sürümüyle ve takip edilen yedi paradigmanın
  hepsiyle, sürüm 1 olarak kaydedin (bkz. [Kurulum](../installation/)). Gözlemci
  puanlamalı paradigmalar `worker.unknownCapability` ile reddedilir. Yanıt
  worker'ın token'ını yalnızca bir kez gösterir; bunu worker'ın gizli
  yapılandırmasında saklayın. `POST /api/analysis/workers/{id}/disable` token'ı iptal eder.
  Worker'ı `MISKO_WORKER_TOKEN` ayarlıyken başlatın, örneğin
  `docker compose --profile worker up -d worker`.
- `jobs` süreci her `JOB_INTERVAL` aralığında (varsayılan 10 saniye) bir
  testteki her hayvan için bir çalıştırma kuyruğa alır: deney çalışıyor veya
  tamamlanmış, hayvan dışlanmamış, testin videosu doğrulanmış, hayvanın
  kalibrasyonu (paradigma gerektiriyorsa) geçerli ve paradigmayı destekleyen bir
  worker varsa. Gözlemci puanlamalı paradigmalar asla böyle bir çalıştırma
  almaz.
- Worker hayvanı her karede arenasının kırpma alanında bulur, konumunu
  santimetreye çevirir ve konumları (yörüngeyi) ve işaretlenmiş bir videoyu
  yükler. Hayvanın grubunu veya tedavisini hiç öğrenmez. Mişko her metriği ve
  olayı yörüngeden hesaplar; worker kendi sayılarını gönderemez. Çok fazla kare
  kaybedilirse veya takip güveni çok düşükse çalıştırma `QC_FAILED` gerekçesiyle
  `FAILED` olur ve hiçbir şey yayınlanmaz. Aydınlık/karanlık kutuda hayvanın
  karanlık bölmede gizli olduğu kareler çıkarımlı örnek olarak gönderilir:
  takip edilmiş sayılırlar ama güven ortalamasına girmezler.
- Her çalıştırmanın bir `method` alanı vardır: worker çalıştırması için
  `AUTOMATED`, bir gözlemcinin puanlamasından hesaplanan çalıştırma için
  `SCORED` (`scoredBy` ile; worker, kalibrasyon ve video çifti olmadan).
- Bir çalıştırma neyin analiz edildiğini tam olarak kaydeder: video sürümü, klip,
  kalibrasyon, paradigma sürümü ve parametreler. Bir hayvanı yeniden analiz
  etmek için TECHNICIAN (veya üstü bir rol)
  `POST /api/analysis/test-subjects/{testSubjectId}/runs` çağırır. Önceki
  sonuçlar kalır.
- `GET /api/tests/{id}/analysis-runs` ve `GET /api/analysis-runs/{runId}`
  çalıştırmaları, metriklerini ve olaylarını gösterir.
  `GET /api/analysis-runs/{runId}/video-pair`, orijinal ve analiz videosuna kısa
  süreli bağlantıları, ikisini eş zamanlı oynatmak için gereken kaymalarla
  döndürür.

### Gözlemci puanlaması

Rotarod, direk, koşu bandı ve yeni nesne testleri testin güncel videosunda bir
kişi tarafından puanlanır. TECHNICIAN ve üstü roller puanlayabilir; giriş yapmış
herkes okuyabilir.

- `GET /api/tests/{id}/subjects/{testSubjectId}/scoring` paradigmanın girdi
  olaylarını (tür, `POINT` ya da `INTERVAL`, `novel` ve `familiar` gibi
  etiketler, bir kez puanlanıp puanlanmadığı), güncel videonun klibini ve en
  yeni puanlama sayfasını olaylarıyla döndürür.
- `POST .../scoring/events`, `{type, label?, startUs, endUs?}` ile taslak
  sayfaya bir olay ekler (ilk olay sürüm 1'i başlatır). Zamanlar klip
  başlangıcından itibaren mikrosaniyedir. `PATCH .../events/{eventId}` bir olayı
  değiştirir, `DELETE` kaldırır.
- `POST .../scoring/complete`, `{scoredEventTypes?, durationUs?}` ile hangi olay
  türlerinin izlendiğini (varsayılan hepsi) ve kayıt uzunluğunu (varsayılan
  klip) sabitler. Motor metrikleri hesaplar ve `SCORED` yöntemli `SUCCEEDED` bir
  çalıştırma saklanır; sonucu diğerleri gibi onay kuyruğunda `PENDING` olarak
  açılır.
- `POST .../scoring/reopen`, olayların bir kopyasıyla bir sonraki sayfa
  sürümünü başlatır; tamamlanması, inceleyenin seçebileceği başka bir
  çalıştırma ekler. Tamamlanmış sayfalar asla değişmez.
- Puanlama için test sürüyor ya da tamamlanmış, deney çalışıyor, hayvan
  dışlanmamış ve video doğrulanmış olmalıdır.

### Onay

Her sonuç sayılmadan önce bir kişi tarafından onaylanır. Bir hayvanın ilk
başarılı çalıştırması, otomatik ya da puanlanmış, sonucunu `PENDING` olarak
açar.

- `GET /api/analysis/results` onay kuyruğudur (`status`, `experimentId`). Kör
  bir deneyde grup, hastalık, madde veya doz göstermez.
- TECHNICIAN ve üstü roller bir sonucu
  `POST /api/analysis/results/{testSubjectId}/accept` ile kabul eder ya da
  `POST .../reject` ile bir `reason` (`TRACKING_LOST`, `WRONG_ANIMAL`,
  `VIDEO_DEFECT`, `PROTOCOL_DEVIATION`, `OTHER`) vererek reddeder.
  `POST .../run` başka bir başarılı çalıştırmayı incelemeye alır;
  `POST /api/analysis/results/bulk-accept` tek seferde en fazla 100 sonucu kabul
  eder.
- Daha yeni bir çalıştırma, incelenmiş bir sonucun yerini kendiliğinden almaz.
  Deney tamamlandıktan sonra kararlar değiştirilemez.

### Çalışma raporu ve gösterge paneli

Giriş yapmış herkes raporları okuyabilir. Raporlar yalnızca kayıtları okur.

- `GET /api/experiments/{id}/report` her sonlanım noktasını, birincil olan önce
  gelecek şekilde döndürür. Birim hayvandır: her hayvan her zaman noktasında bir
  değer katar ve yalnızca dışlanmamış hayvanların kabul edilmiş sonuçları sayılır.
  Grup başına n, ortalama, SD, SEM, en küçük, en büyük, başlangıca göre değişim ve
  kaç hayvanın onay beklediğini, analiz beklediğini, reddedildiğini veya
  dışlandığını gösterir.
- Karşılaştırmalar grup rollerinden çıkar: model geçerliliği (hastalık kontrolü
  ile sağlıklı kontrol ve sham), test duyarlılığı (pozitif kontrol ile hastalık
  kontrolü), her tedavi grubunun kendi hastalık kontrolüne göre etkinliği ve
  kurtarma yüzdesi, ve aynı maddenin tedavi grupları için doz yanıtı.
- Farklı paradigma veya metrik motoru sürümlerinin sonuçları hiç karıştırılmaz;
  rapor sürümleri listeler ve karşılaştırmayı yapmaz.
- `GET /api/experiments/{id}/report/export` hayvan başına (`level=animals`) ya
  da grup başına (`level=groups`) bir satırla CSV indirir; `excel=1` Excel'in
  doğrudan açtığı bir dosya yazar.
- `GET /api/dashboard/summary` deneyleri, testleri, çalıştırmaları ve sonuçları
  durumlarına göre, kalibrasyon bekleyen hayvanları da sayar; son sonuçları ve
  sıradaki planlı oturumları listeler.

### Panel

`frontend/` içindeki Vue paneli bu API'ye taşınmaktadır. Katalog, deney
sekmeleri (tasarım, atama, dozlama, testler, rapor) ve onay kuyruğu ekranlarının
iskeleti var, ancak tamamlanmadı. Tamamlanana kadar API'yi doğrudan kullanın.

### Hatalar

Her hatanın `auth.forbidden`, `experiment.notReady` veya `allocation.frozen`
gibi sabit bir `code` değeri ve İngilizce bir mesajı vardır. 401 durumu giriş
yapmadığınız ya da token'ınızın artık geçerli olmadığı anlamına gelir. 403 durumu
rolünüzün bu işleme izin vermediği anlamına gelir. 409 durumu, kaydın mevcut
halinin değişikliğe izin vermediği anlamına gelir; örneğin çalışan bir deneyin
tasarımını düzenlemek.
