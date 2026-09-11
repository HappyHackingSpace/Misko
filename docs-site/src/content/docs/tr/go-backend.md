---
title: Go backend (önizleme)
description: new-backend branch'inde yeniden yazılan Go backend'in kurulumu ve kullanımı; giriş, kullanıcılar, roller, laboratuvar ayarları, denekler ve deneyler.
---

:::caution
Bu sayfa henüz yayınlanmamış `new-backend` branch'ini anlatır. Go API şu an Vue
panelini beslemiyor; Kurulum ve Kullanım sayfaları hâlâ mevcut sürümü anlatır.
Bu branch'i canlıya almayın.
:::

## Şu an neler var

| Alan | Durum |
|------|-------|
| Sağlık ve hazır olma kontrolleri | Hazır |
| Giriş, mevcut kullanıcı, kendi şifresini değiştirme | Hazır |
| Beş rolle kullanıcı yönetimi | Hazır |
| Laboratuvar ayarları (kurulum başına tek laboratuvar) | Hazır |
| Denekler, deneyler, aşamalar, gruplar ve katılımlar | Hazır |
| Hastalık modelleri, maddeler, müdahale planları ve uygulamalar | Hazır |
| On bir paradigmanın metrik tanımlarıyla paradigma kataloğu | Hazır |
| Ölçüm revizyonlarıyla ortamlar ve deney test protokolleri | Hazır |
| Testler, denemeler ve test yorumları | Hazır |
| Google Cloud Storage'a video yükleme | Hazır, gerçek depolama testi bekliyor |
| Video başına kalibrasyon | Hazır |
| Analiz çalıştırmaları ve worker protokolü | Hazır, görüntü işleme worker'ı henüz yok |
| Raporlar, CSV/JSON dışa aktarma ve grup karşılaştırmaları | Hazır |
| Yeni API üzerinde Vue paneli | Henüz yok |

## Kurulum

Docker ve Docker Compose gerekir. Komutları `new-backend` branch'inde, reponun
kök dizininde çalıştırın:

```bash
docker compose up -d db
docker compose --profile setup run --rm schema
docker compose --profile setup run --rm setup
docker compose up --build -d backend
curl --fail http://127.0.0.1:4000/api/ready
```

1. `db`, PostgreSQL'i yeni bir volume ile başlatır. Eski kurulumun verisine dokunulmaz.
2. `schema`, boş veritabanında tabloları oluşturur. Tekrar çalıştırılırsa reddeder ve hiçbir şey silmez.
3. `setup`, laboratuvarı ve ilk yöneticiyi oluşturur; yöneticinin e-postasını ve
   üretilen şifreyi terminalinize **yalnızca bir kez** yazar. Tekrar
   çalıştırılırsa hiçbir şey oluşturmaz.
4. `backend`, API'yi `127.0.0.1:4000` adresinde başlatır.

`setup` terminal olmadan çalışırsa (örneğin CI içinde) `-credentials-file YOL`
verilmedikçe başlamayı reddeder. Bu durumda şifre, yalnızca sahibinin
okuyabildiği yeni bir dosyaya yazılır. Şifre hiçbir zaman loglara yazılmaz.

Compose veritabanı yerine kendi PostgreSQL sunucunuzu kullanıyorsanız sürüm
PostgreSQL 18 veya üzeri olmalıdır. Şema, PostgreSQL ile birlikte gelen
`btree_gist` eklentisini etkinleştirdiği için `schema` komutunu çalıştıran
kullanıcının veritabanında nesne oluşturma izni olmalıdır.

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
| `LAB_TIMEZONE` | `UTC` | Laboratuvarın saat dilimi; `Europe/Istanbul` gibi bir IANA adı. |

Veritabanı ve zaman aşımı ayarları dahil tüm değişkenler backend README dosyasındadır.

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
| SUPERADMIN, LAB_MANAGER | Kullanıcı ve laboratuvar ayarları yönetimi dahil her şey |
| RESEARCHER | Çalışma, denek, düzenek ve testleri yönetmek; her şeyi okumak |
| TECHNICIAN | Ağırlık kaydetmek ve test çalıştırmak; her şeyi okumak |
| VIEWER | Yalnızca okuma |

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

### Denekler

Giriş yapmış her kullanıcı denekleri okuyabilir. SUPERADMIN, LAB_MANAGER ve
RESEARCHER denekleri değiştirebilir.

- `POST /api/subjects`, `code`, `species` (`MOUSE` veya `RAT`), `sex` (`FEMALE`,
  `MALE` veya `UNKNOWN`) ve isteğe bağlı `strain`, `birthDate` (`YYYY-AA-GG`) ve
  `notes` ile denek oluşturur. Kodlar büyük/küçük harf farkı gözetmeden benzersizdir.
- `GET /api/subjects`, denekleri `search`, `species`, `sex`, `sort`, `order`,
  `page` ve `pageSize` ile listeler.
- `PATCH /api/subjects/{id}`, gönderdiğiniz alanları değiştirir. Boş `strain`,
  `birthDate` veya `notes` değeri alanı temizler.
- `DELETE /api/subjects/{id}`, hiçbir deneye katılmamış bir deneği siler.
- `GET /api/subjects/{id}/enrollments`, deneğin katıldığı tüm deneyleri gösterir.

Denek kendi başına bir kayıttır; aynı hayvan kopyalanmadan birden fazla deneye
katılabilir.

### Deneyler

Giriş yapmış her kullanıcı deneyleri okuyabilir. SUPERADMIN, LAB_MANAGER ve
RESEARCHER deney tasarlayabilir ve denek katabilir.

1. Deneyi `POST /api/experiments` (`code`, `title`, isteğe bağlı `description`
   ve `requiresControl`) ile oluşturun. Deney detayındaki
   `controlRequirementMet`, kontrol grubu eklendiğinde true olur.
2. Ölçüm aşamalarını `POST /api/experiments/{id}/phases` (`name`, `position`)
   ile ekleyin; örneğin 1. sırada Baseline, 2. sırada Post-treatment.
3. Grupları `POST /api/experiments/{id}/groups` (`name`, `CONTROL` veya
   `TREATMENT` değerli `role`, isteğe bağlı `targetSize`) ile ekleyin.
   `GET /api/experiments/{id}/groups` her grubun hedefini ve şu an gruptaki denek
   sayısını gösterir.
4. Deneği `POST /api/experiments/{id}/enrollments` (`subjectId`, `enrolledAt`,
   isteğe bağlı `groupId`) ile deneye katın. Bir denek aynı deneye yalnızca bir
   kez katılabilir.
5. Deneği başka bir gruba
   `POST /api/experiments/{id}/enrollments/{enrollmentId}/assignments`
   (`groupId`, `effectiveFrom`) ile taşıyın. Önceki grup dönemi o anda biter ve
   `GET /api/experiments/{id}/enrollments/{enrollmentId}` ile görülen geçmişte kalır.

Aşamalar ve gruplar bilerek ayrıdır. Aşama deneğin ne zaman ölçüldüğünü, grup
hangi kola ait olduğunu söyler. Sağlıklı baseline ölçümü yapılan, tedavi grubuna
atanmış bir denek tedavi grubunda kalır.

Sistem şunları reddeder:

- başka bir deneye ait aşama, grup veya katılım kayıtları,
- başka bir dönemle çakışan, katılımdan önce başlayan ya da mevcut dönemden sonra
  başlamayan grup dönemleri,
- hâlâ ataması olan bir grubun ya da deneye katılmış bir deneğin silinmesi.

Deneyler ve katılımlar henüz silinemez; araştırma kayıtları korunur.

### Müdahaleler

Hastalık modelleri ve maddeler ortak listelerdir. SUPERADMIN, LAB_MANAGER ve
RESEARCHER bunları `POST /api/disease-models` ve `POST /api/substances`
(`name`, isteğe bağlı `description`) ile yönetir.

- **Planlar**: `POST /api/experiments/{id}/intervention-plans` bir grubun ne
  alması gerektiğini tanımlar (`groupId`, `substanceId`, `amount`, `unit`,
  `route`, `schedule`, isteğe bağlı `phaseId`). Plan, bir şeyin verildiğini
  asla kaydetmez.
- **Ağırlıklar**: TECHNICIAN ve üstü roller vücut ağırlığını
  `POST /api/subjects/{id}/weights` (`grams`, `measuredAt`) ile kaydeder.
- **Durumlar**: TECHNICIAN ve üstü roller gözlemleri
  `POST /api/subjects/{id}/conditions` (`diseaseModelId`, `status`,
  `observedAt`) ile kaydeder. İndüksiyon (`INDUCED`) ve doğrulama (`CONFIRMED`
  veya `NOT_CONFIRMED`) ayrı gözlemler olarak kaydedilir.
  `GET /api/subjects/{id}/conditions` geçmişi ve güncel durumu gösterir;
  `GET /api/conditions?current=true&status=CONFIRMED` denekleri güncel
  durumlarına göre bulur.
- **Uygulamalar**: TECHNICIAN ve üstü roller gerçekten verileni
  `POST /api/experiments/{id}/enrollments/{enrollmentId}/administrations`
  (`substanceId`, `amount`, `unit`, `route`, `administeredAt`, isteğe bağlı
  `planId` ve `weightMeasurementId`) ile kaydeder. Kilogram başına dozlar
  (`mg/kg`, `ug/kg`, `IU/kg`) aynı hayvanın son 7 gün içindeki ağırlığını
  gerektirir ve yanıt mutlak dozu gösterir. `GET /api/subjects/{id}/administrations`
  ve `GET /api/administrations`, `substanceId`, `experimentId`, `from` ve `to`
  ile filtrelenir.

Miktarları `"0.25"` gibi ondalık metin olarak yazın. Kayıtlar, madde adı
değişse veya plan sonradan güncellense bile kaydedildikleri ad ve dozu korur.

### Paradigma kataloğu

Giriş yapmış her kullanıcı paradigma kataloğunu okuyabilir. Paradigmalar kodda
tanımlıdır ve API üzerinden oluşturulamaz, değiştirilemez veya silinemez.

- `GET /api/paradigms`, paradigmaları yayımlanmış sürümleriyle ve otomatik video
  analizinin kullanılabilir olup olmadığıyla (`automatedAnalysis`) listeler.
  On bir paradigma yayımlanmıştır: `BARNES_MAZE`, `EPM`, `LIGHT_DARK`, `MWM`,
  `NOVEL_OBJECT`, `OPEN_FIELD`, `POLE`, `ROTAROD`, `THREE_CHAMBER`, `TREADMILL`
  ve `Y_MAZE`. Hiçbiri için otomatik analiz henüz yoktur.
- `GET /api/paradigms/{key}` en son sürümü, `GET /api/paradigms/{key}/versions/{version}`
  ise tam olarak bir sürümü döndürür. Her sürüm; birim, izin verilen aralık ve
  varsayılan değerleriyle düzenek ve oturum parametrelerini, bölgelerini ve
  olaylarını, her metrik için de tanımı, formülü, girdileri ve veri eksik
  olduğunda ne olacağını listeler.

Bazı paradigmalar izlenen konumdan fazlasına ihtiyaç duyar:

- `calibrated` olarak işaretlenen bölgeler (örneğin yükseltilmiş artı labirent
  kolları veya Barnes labirenti delikleri) her video için daire ya da çokgen
  olarak çizilir. Su labirenti platformu gibi diğer bölgeler düzenek
  parametrelerinden hesaplanır.
- `inputEvents`, izlenmek yerine puanlanan gözlemleri listeler: rotarod düşüşü,
  pole testi dönüşleri veya yeni nesne keşfi gibi. Puanlanmamış bir olay türü
  sıfır değil, eksik metrik üretir.

Sonuçlar her zaman hesaplandıkları paradigma sürümünü, metrik motoru sürümünü ve
sonuç şeması sürümünü belirtir. Hesaplanamayan bir metrik sıfır olarak değil,
nedeniyle birlikte eksik olarak raporlanır. Örneğin kaçışın olmadığı bir probe
denemesi `EVENT_NOT_OBSERVED`, üçten az kol girişi olan bir Y labirenti denemesi
`INSUFFICIENT_ENTRIES` bildirir. Yayımlanmış bir sürüm asla değişmez; değişen
bir tanım yeni bir sürüm olarak yayımlanır.

### Ortamlar ve protokoller

Ortam, tek bir paradigma için kurulmuş fiziksel bir düzenektir; örneğin belirli
bir su tankı. Araştırmacılar, laboratuvar yöneticileri ve süper yöneticiler ortam
ve protokol oluşturup güncelleyebilir; giriş yapmış her kullanıcı bunları okuyabilir.

- `POST /api/environments` bir ortamı ilk revizyonuyla birlikte oluşturur.
  Paradigma sürümünün tüm düzenek ölçülerini verin; örneğin `MWM` için tank ve
  platform boyutları. İzin verilen aralık dışındaki değerler veya tank dışında
  kalan bir platform gibi tutarsız değerler reddedilir.
- Düzenek değiştiğinde `POST /api/environments/{id}/revisions` ile yeni bir
  revizyon ekleyin. Önceki revizyonlar asla değişmez ve silinemez. Ortamın adını
  ve notlarını yine de düzenleyebilirsiniz.

Protokol, bir deneyin deneklerini nasıl test ettiğini tanımlar. Tek bir deneye
aittir ve birden fazla paradigmayı birleştirebilir; örneğin açık alan testinin
ardından yükseltilmiş artı labirent.

- `POST /api/experiments/{id}/protocols` bir protokolü ilk sürümüyle birlikte
  oluşturur. Her adım; sırasını, paradigmayı ve sürümünü, o paradigma için
  kurulmuş bir ortam revizyonunu, deneme türünü, deneme sayısını, denemeler
  arasındaki bekleme süresini saniye olarak ve varsa oturum parametrelerini verir.
- Adımları boşluk bırakmadan 1'den başlayarak numaralandırın. Başka bir
  paradigmanın ortamı, paradigmanın tanımlamadığı bir deneme türü ve aralık
  dışındaki oturum değerleri reddedilir. Vermediğiniz oturum parametreleri
  paradigmanın varsayılanlarını alır ve kaydedilen sürüm bunları gösterir.
- Bir protokolü değiştirmek için
  `POST /api/experiments/{id}/protocols/{protocolId}/versions` ile yeni bir sürüm
  ekleyin. Kaydedilmiş sürümler asla değişmez; yeni bir ortam revizyonu, önceki
  bir revizyonu kullanan sürümü değiştirmez.

### Testler, denemeler ve yorumlar

Test, katılımı olan bir deneğin bir protokol adımındaki tek oturumudur. Adım;
paradigmayı, ortamı ve kaç deneme planlandığını belirler.

- Araştırmacılar, laboratuvar yöneticileri ve süper yöneticiler testleri
  `POST /api/experiments/{id}/tests` ile planlar: katılımı, isteğe bağlı olarak
  aşamayı, protokol sürümünü, adımı ve planlanan zamanı seçin. Denek ve o
  zamandaki grubu otomatik doldurulur.
- Teknisyenler ve üstündeki roller testleri yürütür: `POST /api/tests/{id}/start`,
  ardından her denemeyi `POST /api/tests/{id}/trials` ile kaydedin ve sonunda
  `POST /api/tests/{id}/complete` çağırın. Bir deneme tekrarlanırsa aynı tekrar
  numarasını yeniden kaydedin; sonraki deneme sayısını alır ve önceki kayıt korunur.
- Tamamlanmamış bir test gerekçe yazılarak iptal edilebilir. Denek planlamadan
  sonra başka bir gruba geçtiyse test başlatılamaz; testi iptal edip yeniden planlayın.

Görüntüleyiciler dahil giriş yapmış herkes bir teste yorum yazabilir. Yalnızca
kendi yorumlarınızı düzenleyebilirsiniz. Laboratuvar yöneticileri ve süper
yöneticiler her yorumu silebilir. Her kullanıcı dakikada en fazla 20 yorum
yazabilir veya düzenleyebilir.

### Videolar

Videolar özel bir Google Cloud Storage bucket'ında saklanır. Bir yönetici
`GCS_BUCKET` (ve workload identity kullanılıyorsa `GCS_SIGNER_EMAIL`) ayarlayana
kadar video uç noktaları `media.storageNotConfigured` koduyla 503 döner. Bucket,
CORS ve izin kurulumu için `backend/README.md` dosyasına bakın.

- Teknisyenler ve üstündeki roller `POST /api/tests/{id}/recordings` ile yüklemeyi
  başlatır; dosya adını, türünü (MP4, MOV veya WebM), boyutunu ve CRC32C
  sağlamasını verir. Yanıt imzalı bir istek içerir; tarayıcı dosyayı bununla
  doğrudan depolamaya yükler.
- Yükleme bitince aynı kişi (veya bir laboratuvar yöneticisi ya da süper yönetici)
  `POST /api/tests/{id}/recordings/{recordingId}/finalize` çağırır. Sunucu saklanan
  dosyayı kontrol eder. Eşleşmeyen dosya, `CHECKSUM_MISMATCH` gibi bir nedenle
  `REJECTED` olarak işaretlenir.
- Giriş yapmış herkes doğrulanmış bir video için
  `GET /api/tests/{id}/recordings/{recordingId}/read-url` ile kısa ömürlü bir
  bağlantı alabilir. Oynatıcılar bu bağlantıda ileri geri sarabilir.

### Kalibrasyon

Bir videodan mesafe ve bölgelerde geçen süre hesaplanabilmesi için kamera
piksellerinin santimetreye nasıl karşılık geldiği bilinmelidir.

- Video doğrulandıktan sonra bir teknisyen (veya üstündeki bir rol) düzeneğin
  göründüğü bir kareyi açıp noktaları işaretler: en az 4 FIT noktası ve bunlardan
  ayrı en az 3 CHECK noktası; her biri piksel konumu ve santimetre cinsinden gerçek
  konumuyla. Noktaları
  `POST /api/tests/{id}/recordings/{recordingId}/calibrations` ile gönderin.
- FIT noktaları dönüşümü belirler, CHECK noktaları ise onu sınar. Bir CHECK noktası
  paradigmanın izin verdiğinden (iz izleme paradigmalarında 2 cm) fazla saparsa
  kalibrasyon `REJECTED` olarak saklanır.
- Bir kalibrasyonu düzeltmek için `supersedesId` alanı en son kalibrasyonu gösteren
  yeni bir kalibrasyon gönderin. Kamera ve kare boyutu aynı kalmalıdır. Önceki
  kalibrasyonlar saklanır.
- `GET /api/tests/{id}/recordings/{recordingId}/calibration-status`, geçerli bir
  kalibrasyon olana kadar `WAITING_FOR_CALIBRATION`, sonra `CALIBRATED` gösterir.
  Rotarod gibi video ölçümü olmayan paradigmalar `NOT_REQUIRED` gösterir.

### Video analizi

İz takibi, API ile konuşan ayrı analiz worker'larında çalışır. Sabit kamerayla
kaydedilmiş, tek denekli açık alan videoları için ilk worker `worker/`
klasöründedir. Şimdiye kadar yalnızca sentetik videolarla test edilmiştir; elle
işaretlenmiş gerçek bir kayıtla doğrulanana kadar sonuçlarını araştırmada
kullanmayın.

- Worker her karede hayvanı bulur, konumunu videonun kalibrasyonuyla santimetreye
  çevirir ve konumları (trajectory) ile yolu ve merkez bölgesini gösteren işaretli
  bir videoyu yükler.
- Mişko her metriği ve olayı, paradigma kataloğunun tarif ettiği hesaplarla,
  yüklenen trajectory'den kendisi hesaplar. Worker kendi sayılarını gönderemez.
- Çok fazla kare kaybolursa (açık alanda %10'dan fazla) veya takip güveni çok
  düşükse çalıştırma `QC_FAILED` nedeniyle `FAILED` olur ve hiçbir şey
  yayınlanmaz.
- Çalıştırmak için `OPEN_FIELD` sürüm 1 için bir worker kaydedin, sonra
  `MISKO_WORKER_TOKEN` ayarlıyken başlatın; örneğin
  `docker compose --profile worker up -d worker`.

- Bir laboratuvar yöneticisi `POST /api/analysis/workers` ile worker'ın adını, model
  sürümünü ve desteklediği paradigma sürümlerini vererek worker kaydeder. Yanıt
  worker token'ını yalnızca bir kez gösterir; bunu worker'ın gizli ayarlarında
  saklayın. `POST /api/analysis/workers/{id}/disable` token'ı iptal eder.
- Zamanlayıcıyı API'nin yanında `docker compose up --build -d backend jobs` ile
  başlatın. Her `JOB_INTERVAL` süresinde (varsayılan 10 saniye), doğrulanmış, gerekiyorsa
  geçerli kalibrasyonu olan ve paradigmasını destekleyen bir worker bulunan her
  video için bir çalıştırmayı kuyruğa ekler.
- Bir çalıştırma neyin analiz edildiğini tam olarak kaydeder: depolamadaki video
  sürümü, kesit, kalibrasyon, paradigma sürümü ve parametreler. Bir çalıştırmanın
  hiçbir bilgisi sonradan değişmez.
- Bir videoyu yeniden analiz etmek için (örneğin yeni bir kalibrasyondan sonra veya
  yeni bir worker modeliyle) bir teknisyen (veya üstündeki bir rol)
  `POST /api/tests/{id}/recordings/{recordingId}/analysis-runs` çağırır. Bu yeni
  bir çalıştırma oluşturur; önceki sonuçlar korunur.
- Giriş yapmış herkes çalıştırmaları `GET /api/tests/{id}/analysis-runs` ile
  listeleyebilir ve `GET /api/analysis-runs/{runId}` ile açabilir; çalıştırma
  `SUCCEEDED` olduğunda metrikler ve olaylar da gelir.
  `GET /api/analysis-runs/{runId}/video-pair`, orijinal videoya ve o çalıştırmanın
  analiz edilmiş videosuna kısa ömürlü bağlantılar ile ikisini senkron oynatmak için
  gereken zaman kaymalarını döner.
- Yanıt vermeyi bırakan bir worker 5 dakika sonra çalıştırmayı kaybeder ve başka
  bir worker onu alabilir. Bir çalıştırma en fazla 3 kez denenir, sonra `FAILED`
  olur. Çalıştırmayı kaybetmiş bir worker'ın sonuçları reddedilir ve bir sonuç,
  API yüklenen her dosyayı depolamada kontrol ettikten sonra yayınlanır.

### Panel

`frontend/` içindeki Vue paneli iş akışını bu API üzerinden yürütür. Giriş yapın,
bir deney açın ve testlerinden birini seçin.

- Test ekranı kayıtları video durumu ve kalibrasyon durumuyla birlikte listeler;
  teknisyen (veya üstü bir rol) video yükleyebilir. Dosya tarayıcıdan doğrudan
  depolamaya gider; yükleme kaldığı yerden devam edebilir. Panel dosyanın sağlama
  değerini önce hesaplar, böylece API nesneyi depolamadan geri okuyup eşleşmeyeni
  reddeder. Bir kayıt ancak API bunu yaptıktan sonra doğrulanmış görünür.
- Testin analiz çalıştırmaları düğme olarak görünür. Ekran, sonuç yayınlamış en
  yeni çalıştırmayı açar; böylece sonradan başarısız olan bir deneme okunabilir
  sonucu gizlemez. Kuyruktaki veya çalışan bir iş bunu söyler, başarısız olan ise
  nedenini gösterir, örneğin kalite kontrol hatası.
- Yayınlanmış bir çalıştırmada panel metrikleri ve olayları gösterir. Bir olaya
  tıklamak, örneğin "Merkezde 00:03-00:04", yan panelde tam olarak o aralığı
  oynatır ve sonunda durur. Orijinal ve analiz edilmiş video yan yana, aynı zaman
  çizgisinde durur; ikisi de aynı anı gösterir. Olay zamanları klipten ölçülür ve
  klip videonun başından başlamak zorunda değildir; bu yüzden panel her iki
  oynatıcıyı da çalıştırmanın video çiftindeki kaydırma değerleriyle öteler.
- Her olaya klavyeyle erişilebilir: Tab olaylar arasında gezer, Enter seçili
  olanı oynatır.
- İzleyici rolü sonuçları okur ama yükleme veya yeniden analiz denetimlerini
  görmez; API de bu işlemleri reddederdi.

### Raporlar ve dışa aktarma

Giriş yapmış herkes raporları okuyabilir. Raporlar yalnızca sonuçları okur;
araştırma kayıtlarında hiçbir şeyi değiştirmez.

- `GET /api/reports/metrics`, metrik sonuçlarını nereden geldikleriyle listeler:
  testin deneyi, deneği, grubu ve aşaması, test, ortam, video ve model ile metrik
  motoru sürümleriyle analiz çalıştırması. Deney, denek, grup, aşama, test, ortam,
  video, çalıştırma, hastalık modeli, madde, paradigma, metrik motoru sürümü veya
  metrik anahtarına göre filtreleyin ve satırlarda `page` ile `pageSize` kullanarak
  gezinin.
- Varsayılan olarak her test bir kez sayılır: yalnızca en yeni başarılı
  çalıştırması kullanılır (metrik motoru sürümü başına). Eski çalıştırmaları da
  görmek için `selection=all` ekleyin.
- Worker'ın ölçemediği bir sonuç nedeniyle birlikte boş kalır. Hiçbir zaman sıfır
  olarak gösterilmez.
- `GET /api/reports/events`, olayları ait oldukları denemeyle listeler.
- Eşleşen tüm satırları CSV olarak, `format=json` ile de JSON olarak indirmek için
  iki rapordan birine `/export` ekleyin (`/api/reports/metrics/export`). Bir dışa
  aktarma en fazla 100.000 satır içerir; daha fazlası için filtreleri daraltın.
- `GET /api/reports/metric-summary?experimentId=...&metricKey=...` bir deneyin
  gruplarını karşılaştırır. Her hayvan, testlerinin ortalamasıyla bir kez sayılır;
  böylece tekrarlanan testler hayvan sayısını şişirmez. Sonuçlar farklı paradigma
  veya metrik motoru sürümlerinden geliyorsa API bunları karıştırmayı reddeder ve
  hangi sürümleri seçmeniz gerektiğini söyler.

### Hatalar

Her hata `auth.forbidden` veya `user.lastPrivileged` gibi sabit bir `code` ve
İngilizce bir mesaj içerir. 401 durumu giriş yapmadığınızı ya da token'ınızın
artık geçerli olmadığını gösterir. 403 durumu rolünüzün bu işleme izin
vermediğini gösterir.
