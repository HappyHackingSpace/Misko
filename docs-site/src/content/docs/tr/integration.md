---
title: Entegrasyon (analiz işçileri)
description: Bir analiz işçisinin işi nasıl aldığı, videoyu nasıl okuduğu, çıktılarını nasıl yüklediği ve sonucu API'nin nasıl hesapladığı.
---

Video analizi **işçilerde** (worker) çalışır: Mişko API'sine kimlik doğrulayan,
her seferinde bir iş alan ve ürettiklerini yükleyen süreçler. İşçinin kendi
veritabanı yoktur ve API işçiyi hiç çağırmaz; işçi yalnızca aşağıdaki
`/api/worker` yollarını çağırır. Paketle gelen işçi `worker/` klasöründedir
(bkz. [Kurulum](../installation/#analiz-workerı)), ama bu protokolü izleyen her
program onun yerini alabilir.

## İş nedir

Bir iş, tek bir videoyu tek bir paradigma sürümü, onun parametreleri ve
paradigma gerektiriyorsa bir kalibrasyonun kopyasıyla analiz eder. Girdileri
hiç değişmez: yeniden analiz yeni bir iş açar, öncekiler kalır. İşi bir testin
hayvanı ya da bir paradigma denemesi ister; iş hayvanın grubunu veya
tedavisini hiç söylemez, bu yüzden işçi bunlardan etkilenemez.

`jobs` süreci, çalışma devam ediyor ya da tamamlanmışsa, hayvan dışlanmamışsa,
testin videosu doğrulanmışsa ve hayvanın arenası gereken her bölgeyle
kalibre edilmişse, testin her hayvanı için bir iş sıraya koyar. Paradigma
denemeleri, videoları ve kalibrasyonları hazır olduğunda bir iş açar; deneme
değiştirilmiş eşiklerle yeniden analiz edildiğinde bir tane daha.

## İşçiyi kaydetme

Bir laboratuvar yöneticisi (veya süper yönetici) işçiyi
`POST /api/analysis/workers` ile kaydeder: adını, model sürümünü ve analiz
edebildiği paradigma sürümlerini (yeteneklerini) verir. Paradigma kataloğunda
olmayan sürümler `worker.unknownCapability` ile reddedilir. Yanıt işçinin
belirtecini (token) **yalnızca bir kez** gösterir; onu işçinin gizli
yapılandırmasında saklayın (paketle gelen işçi için `MISKO_WORKER_TOKEN`).
`POST /api/analysis/workers/{id}/disable` belirteci iptal eder.

İşçinin her çağrısı `Authorization: Worker <token>` gönderir. Bu belirteç
yalnızca `/api/worker` yollarında geçerlidir ve kullanıcı belirteçleri orada
kabul edilmez.

## Protokol

1. **Alma.** `POST /api/worker/claim`, işçinin desteklediği paradigma sürümüne
   ait en eski bekleyen işi kiralar (lease); iş yoksa `204` döner. Yanıtta iş
   (video, klip, sabitlenmiş parametreler), paradigma kontratı, kalibrasyon
   kopyası (kare boyutu, arenanın kırpımı, santimetreye homografi ve kalibre
   edilmiş bölgeler), çıktı öneki ve üretilecek yörünge biçimi bulunur.
   Ayrıca **deneme** (attempt) numarası gelir: sonraki her çağrı onu belirtir,
   eski bir denemeyle yapılan çağrı `409` ile reddedilir.
2. **Nabız.** `POST /api/worker/runs/{runId}/heartbeat` kiralamayı uzatır.
   Kiralama 5 dakika sürer; paketle gelen işçi onu her 60 saniyede yeniler.
3. **Videoyu okuma.** `POST /api/worker/runs/{runId}/source-url`, işin
   sabitlediği video sürümüne kısa ömürlü imzalı bir URL döner.
4. **Çıktıları yükleme.** Her çıktı için
   `POST /api/worker/runs/{runId}/outputs` türünü, dosya adını, içerik türünü,
   boyutunu ve CRC32C değerini bildirir ve denemenin önekinde
   (`jobs/{jobId}/attempts/{attempt}/`) imzalı bir yükleme URL'si döner. Bir
   sonuç tam olarak bir `TRAJECTORY` (`application/json`) ve bir
   `ANALYZED_VIDEO` (`video/mp4` veya `video/webm`) ister; `THUMBNAIL` görseli
   isteğe bağlıdır.
5. **Gönderme.** `POST /api/worker/runs/{runId}/result` yüklenen çıktıları,
   model sürümünü ve analiz edilmiş videonun özgün videoyla nasıl
   hizalandığını (`pair`: kaydırmalar ve zaman eşleme sürümü) bildirir. İstek
   ölçüm ya da olay içermez; API bunları reddeder.
6. **Ya da başarısızlık.** `POST /api/worker/runs/{runId}/failure` bir neden ve
   yeniden denenip denenemeyeceğini bildirir.

Yörünge `misko.trajectory.v1` JSON'udur: iş kimliği, deneme, kayıt süresi ve
eşit uzunlukta diziler: örnek zamanları (`tUs`, klip başından mikrosaniye),
santimetre cinsinden konumlar (`xCm`, `yCm`), `tracked`, `confidence` (0 ile 1
arası) ve `inferred`. İşçinin kendi tanılamaları gibi diğer alanlar saklanır
ama okunmaz.

## API sonuçla ne yapar

- **Nesneleri doğrular.** Bildirilen her çıktı, bu iş ve deneme altında,
  bildirilen boyut, sağlama toplamı ve içerik türüyle var olmalıdır.
- **Yörüngeyi denetler.** Yörünge bu işe ve denemeye ait olmalı, kayıt
  süresiyle uyuşmalı, zamanları kesin artan olmalı ve izlenen her örneğin
  konumu bulunmalıdır. En fazla 64 MiB olabilir (30 fps'te yaklaşık dört saat).
- **Kalite kurallarını uygular.** Paradigmanın `max_lost_frame_ratio` kuralı
  izlenemeyen örneklere, `min_tracking_confidence` kuralı izlenen örneklerin
  ortalama güvenine göre denetlenir. Çıkarımla konumlanan örnekler izlenmiş
  sayılır ama güven ortalamasına girmez. Geçemeyen yörünge `QC_FAILED`
  nedeniyle `FAILED` olur ve yeniden denenmez; kalite değerleri her durumda
  saklanır.
- **Sonucu hesaplar.** Go ölçüm motoru her ölçümü ve olayı, işin sabitlenmiş
  parametreleri ve kalibrasyonuyla yörüngeden hesaplar. İzlemeyi ne üretmiş
  olursa olsun, her ölçümün tek tanımı API'dedir. Bkz.
  [Ölçüm mimarisi](../measurements/).
- **Tek seferde yayımlar.** Ölçümler, olaylar, kalite ve çıktılar birlikte
  görünür. Kabul edilmiş aynı sonucu yeniden göndermek, yeni bir şey
  yayımlamadan işi döner.

Bir test hayvanının ilk başarılı sonucu insan incelemesini bekler; çalışma
raporunda yalnızca incelemecinin kabul ettiği sonuçlar sayılır. Paradigma
denemelerinin sonuçları incelemeye, raporlara veya panoya hiç girmez.

## Yeniden denemeler ve kiralamalar

Bir iş 3 kez denenebilir. Yeniden denenebilir bir başarısızlık ya da nabızsız
dolan bir kiralama, deneme hakkı kaldıkça işi sıraya geri koyar; sonuncudan
sonra iş `FAILED` olur (dolan kiralama için `LEASE_EXPIRED`). Yeniden
denenemeyen bir başarısızlık işi hemen bitirir. Her çağrı denemesini
belirttiği için kiralamasını yitiren bir işçi, işi devralan işçinin
çalışmasının üzerine yazamaz.

## Kalibrasyon önerileri

İşçiler kalibrasyon önerilerini de yanıtlar. Panel tek bir referans kareyi
(JPEG veya PNG, en fazla 700 KiB) `POST /api/calibration-proposals` ile
yükler; sırada iş yoksa işçi desteklediği bir paradigma sürümüne ait öneriyi
`POST /api/worker/proposals/claim` ile kiralar (kare yanıtta gelir) ve tespiti
denemesini belirterek `POST /api/worker/proposals/{id}/result` ile döner.
Kiralama 2 dakika sürer; dolan kiralama yeniden alınır, toplamda en fazla 3 kez. Öneri
yalnızca bir öneridir: araştırmacı her noktayı onaylar ve kalibrasyon
yollarıyla kaydeder. Öneriler bir gün sonra silinir.

## Depolama

Videolar ve çıktılar veritabanında değil, nesne depolamada durur: API'nin
sunduğu yerel bir klasör (`LOCAL_STORAGE_DIR`) ya da özel bir Google Cloud
Storage kovası (`GCS_BUCKET`). İşçiler yalnızca kısa ömürlü imzalı URL'ler
görür; veritabanı nesnelere yalnızca referans tutar.
