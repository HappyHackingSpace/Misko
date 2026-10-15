---
title: Kullanım
description: Mişko panelinin katalogdan çalışma tasarımına, dozlamadan test oturumlarına, sonuç onayından raporlara kadar günlük kullanımı.
---

Bu sayfa, Mişko kurulduktan sonra onu kullanmaya yönelik pratik rehberdir.
Sadece geliştiriciler için değil, laboratuvardaki herkes için yazılmıştır.
Mişko'yu henüz kurmadıysanız [Kurulum](../installation/) ile başlayın.

## Genel resim

Mişko, klinik öncesi bir davranış çalışmasının kayıt defteridir. Bir çalışma tek
bir soru sorar: belirli bir dozdaki madde, bir hastalık modelinin yol açtığı
bozukluğu geri çeviriyor mu? Paneldeki her şey bu soruyu izler:

1. **Katalog**, laboratuvarın kullandığı maddeleri ve hastalık modellerini listeler.
2. **Denekler** hayvanlardır; vücut ağırlıklarıyla birlikte tutulur.
3. **Ortamlar** arenalardır; her biri sabit kamerası için bir kez kalibre edilir.
4. Bir **deney**, gruplar (her birinin rolü, hastalık modeli ve aldığı uygulama),
   zaman noktaları, protokoller ve ölçütler olarak tasarlanır ve başlatılır.
5. Hayvanlar gruplara **atanır**, her gün **dozlanır** ve **test oturumlarında**
   filme alınır.
6. Bir analiz işçisi oturum videosunda her hayvanı ölçer. Her sonucu bir kişi
   **inceler** ve yalnızca kabul edilen sonuçlar **rapora** girer.

Panel varsayılan olarak Türkçedir. Üst çubuktaki dil seçiciyle İngilizceye
geçebilirsiniz; seçiminiz hatırlanır.

## İlk giriş

Public kayıt yoktur. Kurulumdan sonra tek bir **superadmin** hesabı vardır ve
şifresi backend log'una bir kez yazılmıştır (bkz.
[Kurulum](../installation/#ilk-giriş-şifresini-alın)).

1. Paneli açın (varsayılan [http://localhost:8080](http://localhost:8080)).
2. `ADMIN_EMAIL` ve üretilen şifre ile giriş yapın.
3. Ekibin geri kalanını **Kullanıcılar** ekranından oluşturun.

## Roller

Beş rol vardır. Her rolün ne yapabileceğine API karar verir; panel, API'nin
reddedeceği düğmeleri gizler.

| Rol | Neler yapabilir |
|---|---|
| `SUPERADMIN`, `LAB_MANAGER` | Kullanıcılar ve laboratuvar ayarları dahil her şey. |
| `RESEARCHER` | Kullanıcılar ve laboratuvar ayarları dışında her şey: katalog, denekler, ortamlar, çalışma tasarımı, oturum planlama, dozlama, oturum yürütme, sonuç onayı. |
| `TECHNICIAN` | Laboratuvar günü: doz ve ağırlık kaydı, oturumu başlatma ve tamamlama, deneme kaydı, video yükleme, bir hayvanı bir videoda kalibre etme, sonuç onayı. Oturum planlayamaz, iptal edemez ve çalışmayı değiştiremez. |
| `VIEWER` | Her şeyi okur, hiçbir şeyi değiştirmez. |

**Kullanıcılar** ekranında kullanıcı oluştururken şifreyi boş bırakıp sistemin
güçlü bir şifre üretmesine izin verebilirsiniz. Şifre yalnızca bir kez
gösterilir; kapatmadan önce kopyalayın.

## Ortak kontroller

Listeler her yerde aynı çalışır: bir arama kutusu, filtreler, sıralanabilir
sütunlar ve altta sayfalama. Yeni kayıtlar ve düzenlemeler yan çekmecede ya da
bir pencerede açılır; yeşil bir mesaj kaydı onaylar, hata ise formun yanında
sözle gösterilir. Silme her zaman onay ister. Birçok ekran filtrelerini adres
çubuğunda tutar; böylece filtrelenmiş bir listeyi yer imine ekleyebilir veya
paylaşabilirsiniz.

## Katalog

Menüden **Katalog**'u açın. İki sekmesi vardır.

**Maddeler**; bileşikler, referans ilaçlar, taşıyıcılar (serum fizyolojik, CMC,
DMSO) ve hastalık oluşturucular (MPTP, LPS) içindir. Eklemek için:

1. **Yeni madde**'ye tıklayın.
2. Adı yazın, türünü seçin ve isterseniz bir açıklama ekleyin.
3. **Kaydet**'e tıklayın.

Tür yalnızca seçicilerdeki sırayı belirler; bir maddenin çalışmadaki rolü
nerede kullanıldığından gelir. Adlar benzersizdir: ikinci bir "Saline" (büyük
küçük harf fark etmeksizin) reddedilir. Bir rejimde ya da kayıtlı bir dozda
kullanılan madde silinemez; panel bunu söyler.

**Hastalık modelleri**, bozukluğun nasıl oluşturulduğunu (kimyasal, cerrahi,
diyet, stres veya diğer) ve nasıl doğrulandığını tanımlar. Aynı şekilde
eklenir. Bir grubun kullandığı model silinemez.

## Denekler ve ağırlıklar

**Denekler**'i açın. Her hayvanın bir kodu (harf, rakam, `.`, `_`, `/`, `-`),
türü (fare veya sıçan) ve cinsiyeti vardır.

1. **Yeni denek**'e tıklayın, kodu yazın, tür ve cinsiyeti seçin ve kaydedin.
2. Hayvanları bulmak için arama kutusunu, tür ve cinsiyet filtrelerini kullanın.
   Henüz hiçbir gruba ait olmayanları görmek için **Yalnızca gruba
   atanmamışlar**'ı işaretleyin.
3. Hayvanın sayfasını açmak için koduna tıklayın. Sayfa, çalışmasını ve grubunu
   (ya da atanmadığını) ve en yenisi üstte olmak üzere vücut ağırlıklarını,
   bir önceki tartıma göre farkla gösterir.
4. Bir hayvanı tartmak için sayfasında gramı ve zamanı yazıp **Ağırlığı
   kaydet**'e tıklayın. Ağırlıklar sonradan düzenlenmez.

Kodlar benzersizdir. Bir gruptaki veya ağırlığı olan hayvan silinemez.

## Ortamlar ve kalibrasyon

Ortam, bir paradigmadan (açık alan, rotarod, Morris su labirenti gibi)
oluşturulan fiziksel bir arenadır. **Ortamlar**'ı açıp **Yeni ortam**'a
tıklayın:

1. Bir ad yazın (örneğin "Box A").
2. Paradigmayı seçin. Form, o paradigmanın gerektirdiği ölçüleri tipik
   değerlerle doldurulmuş olarak gösterir.
3. Gerçek ölçüleri girin (örneğin arenanın cm cinsinden eni ve boyu). Analiz
   ayarları katlanmıştır ve nadiren değiştirilmesi gerekir.
4. **Kaydet**'e tıklayın. Ortamın sayfası açılır.

**Varsayılan kalibrasyon.** Yeri değişmeyen bir kamera her arena için bir kez
kalibre edilir ve o arenada çekilen her video bunu kullanır:

1. Ortamın sayfasında **Bu revizyonu kalibre et**'e tıklayın.
2. Kamerayı, görüntü boyutunu ve bu arenanın görüntüdeki kırpma alanını girin.
3. Dört uydurma noktası (piksel konumu ve cm cinsinden gerçek konum, genellikle
   köşeler) ve en az üç kontrol noktası girin.
4. **Kaydet**'e tıklayın. Panel uydurma ve kontrol hatalarını ve toleransı
   gösterir. Kontrol hatası toleransı aşan kalibrasyon reddedilir; daha iyi
   noktalarla yeniden girin.

**Revizyonlar.** Ölçüler yerinde değiştirilmez, böylece her test çalıştırıldığı
değerlerle kalır. Yeni bir revizyon oluşturmak için **Ölçüleri değiştir**'e
tıklayın. Yeni revizyonun kendi kalibrasyonu gerekir.

## Bir çalışmayı tasarlama

**Deneyler**'i açıp **Yeni deney**'e tıklayın. Kodu, başlığı, hipotezi,
hayvanların nasıl rastgele dağıtıldığını ve çalışmanın kör olup olmadığını
doldurup kaydedin. Deney **Gruplar ve rejimler** sekmesinde açılır. Yeni bir
çalışma **Taslak**'tır; başlayana kadar tasarımı değiştirilebilir.

**Gruplar ve rejimler.**

1. **Grup ekle**'ye tıklayın. Bir kod (G1, G2), ad, rol ve hedef hayvan sayısı
   verin. Sağlıklı kontrol dışındaki her rol bir hastalık modeli ister.
2. Grubun kartında **Kalem ekle**'ye tıklayarak grubun ne aldığını belirtin:
   amaç (indüksiyon, taşıyıcı veya tedavi), madde, doz ve birim (örneğin
   10 mg/kg), uygulama yolu, sıklık (bir kez, günlük, günde iki kez, haftalık
   veya N günde bir) ve ilk ile son çalışma günü.

Roller şunlardır: sağlıklı kontrol, sham, hastalık kontrolü, pozitif kontrol ve
tedavi. Tedavi veya pozitif kontrol grubunun tam olarak bir tedavi kalemi (doz
düzeyi) ve bir indüksiyon kalemi vardır; hastalık kontrolünün bir indüksiyon ve
bir taşıyıcı kalemi vardır; sham grubunun taşıyıcısı vardır, indüksiyonu yoktur.

**Zaman noktaları ve protokoller.** **Zaman noktaları ve protokoller**
sekmesinde:

1. Hayvanların test edildiği her an için **Zaman noktası ekle**'ye tıklayın;
   bir ad (Baseline, 7. gün) ve çalışma günü verin (0. günden önce eksi).
2. **Yeni protokol**'e tıklayın, ad verin ve her test için bir adım ekleyin:
   paradigma, deneme türü ve deneme sayısı. Oturum ayarları olan paradigmalar
   (örneğin rotarod hızları) bunları adımda gösterir.
3. Bir protokolü sonradan değiştirmek için **Yeni sürüm**'e tıklayın. Önceki
   sürümler değişmez; oturumlar planlandıkları sürümü korur.

**Ölçütler.** **Ölçütler** sekmesinde **Ölçüt ekle**'ye tıklayarak çalışmanın
neye göre değerlendirileceğini ekleyin: protokollerdeki bir paradigmanın bir
metriği, bir zaman noktasında, isteğe bağlı olarak bir başlangıç zaman
noktasıyla karşılaştırmalı, ve hangi yönün daha iyi olduğu. Tam olarak bir ölçüt
**birincil**'dir; ikinci bir birincil ölçüt reddedilir.

**Başlatma.** **Genel bakış** sekmesi henüz karşılanmayan her başlatma kuralını,
düzelteceği sekmeye bir bağlantıyla birlikte listeler. Liste çalışmanın hazır
olduğunu söylediğinde **Çalışmayı başlat**'a tıklayın, 0. günü (çalışma
günlerinin sayıldığı tarih, genellikle indüksiyon) seçin ve onaylayın. Çalışma
**Çalışıyor** olur: gruplar, rejimler, zaman noktaları ve ölçütler artık salt
okunur gösterilir. Yeni protokol sürümleri hâlâ eklenebilir.

## Atama ve dışlama

**Hayvanlar** sekmesinde:

1. **Hayvan ata**'ya tıklayın. Çekmece hiçbir gruba ait olmayan hayvanları
   listeler.
2. Arayın ve hayvanları işaretleyin.
3. **Tek grup**'u seçip grubu belirleyin ya da sistemin onları hedeflerine kadar
   gruplara yayması için **Rastgele dağıt**'ı seçin (isterseniz yalnızca bazı
   gruplara). Bir kafes eklenebilir.
4. **Ata** veya **Rastgele dağıt**'a tıklayın.

Üstteki grup kartları her grubun ne kadar dolu olduğunu gösterir. Bir hayvan tam
olarak bir gruba aittir ve çalışma başladıktan sonra grubu değişmez. Bir hayvan
ölürse ya da çalışmadan çıkması gerekirse **Dışla**'ya tıklayın, nedeni seçin
(öldü, insani son nokta, indüksiyon başarısız, teknik sorun, diğer) ve bir not
ekleyin. Dışlanan hayvanlar nedenleriyle listede kalır ve istatistiklere
girmez.

## Günlük dozlama

**Dozlama** sekmesi bir günün verilecek dozlarını grup ve hayvana göre, her
hayvanın son ağırlığıyla birlikte gösterir. Başka bir güne geçmek için okları
veya tarih seçiciyi, geri dönmek için **Bugün**'ü kullanın. Bir doz bugün
**Bekliyor**, kaydedilince **Verildi**, geçmiş bir günde **Eksik** ve gelecek
bir günde **Planlandı** olarak görünür.

1. Bekleyen dozun yanındaki **Kaydet**'e tıklayın. Pencere planlanan dozu
   gösterir; zamanı, miktarı veya yolu yalnızca verilen farklıysa değiştirin.
2. Kilogram başına doz son yedi günden bir ağırlık ister. Yoksa pencere önce
   ağırlığı sorar, kaydeder ve dozu onunla kaydeder. Liste, planlanan mg/kg
   dozun yanında mg cinsinden dozu gösterir.
3. Planda olmayan bir doz için **Plan dışı doz**'a tıklayıp hayvanı, amacı,
   maddeyi, miktarı, birimi ve yolu seçin.

**İndüksiyon sonuçları.** İndüksiyon uygulanan hayvanlar **İndüksiyon
sonuçları** altında listelenir. **Sonucu kaydet**'e tıklayın, doğrulandı veya
doğrulanmadı seçip kaydedin. Bir sonuç bir kez kaydedilir.

Alttaki **Doz geçmişi** kaydedilen her dozu listeler ve hayvana ve amaca göre
filtrelenebilir.

## Test oturumlarını planlama ve yürütme

Bir test oturumu; bir zaman noktası, bir protokol adımı ve **tek bir videodur**.
Bir veya daha fazla hayvan içerir, her biri kendi arenasındadır (örneğin tek
kameranın çektiği üç açık alan kutusu).

**Planlama** (araştırmacı ve üstü), deneyin **Testler** sekmesinde:

1. **Oturum planla**'ya tıklayın.
2. Zaman noktasını seçin; protokol, sürüm ve adım önceden seçilidir.
3. Zaman varsayılan olarak 0. gün artı zaman noktasının çalışma günüdür.
4. İlk hayvanı ve arenasını seçin, ardından her ek hayvan için **Hayvan
   ekle**'ye tıklayın. Yalnızca dışlanmamış ve aynı zaman noktası ile adımda
   zaten planlanmamış hayvanlar, ve yalnızca adımın paradigmasına ait arenalar
   sunulur.
5. **Planla**'ya tıklayın.

Oturum planlıyken sayfasındaki **Hayvanları düzenle** hayvan ekler veya
çıkarır. **Oturumu iptal et** bir neden ister ve bu neden oturumda saklanır.

**İş listesi.** **Testler** menüsü, oturumları çalışmalar arasında dört görünümde
listeler: **Bugün**, **Geciken** (daha önceki bir günden planlı ya da hâlâ
süren), **Yaklaşan** ve **Tümü**; deney, durum ve paradigma filtreleriyle.
Görünüm ve filtreler adreste tutulur.

**Yürütme** (teknisyen ve üstü), oturumun sayfasında:

1. **Başlat**'a tıklayın. Dışlanmış bir hayvanı olan oturum, o hayvan
   çıkarılmadan başlamaz.
2. Her hayvan için kartındaki **Deneme kaydet**'e tıklayın, tekrar numarasını
   ve zamanları kontrol edip kaydedin. Kart yapılan denemeleri planlanan sayıyla
   karşılaştırır.
3. Oturum videosunu **Video yükle** ile yükleyin. Video yüklendikten sonra
   doğrulanır ve kullanılan video olarak gösterilir. **Videoyu değiştir** bir
   onaydan sonra yenisini yükler; önceki videolar **Önceki videolar** altında
   listelenir.
4. Her hayvanın kartı, görüntüdeki kendi bölümünün hangi kalibrasyonu
   kullandığını gösterir: normalde **Arena varsayılanı**. Kamera veya bir arena
   kaydıysa o hayvanda **Bu videoda kalibre et**'e tıklayıp noktaları yalnızca
   bu video için girin. Diğer hayvanlar arena varsayılanını korur.
5. Her hayvanın en az bir denemesi olduğunda **Tamamla**'ya tıklayın; o zamana
   kadar düğme pasif kalır ve sayfa nedenini söyler.

Sayfanın altındaki yorumlar, laboratuvarın olağan dışı her şeyi not etmesini
sağlar. Herkes yorum yazabilir; kendi yorumunuzu düzenleyebilirsiniz,
laboratuvar yöneticileri herhangi bir yorumu silebilir.

## Analiz ve sonuç onayı

Süren bir oturumun videosu doğrulandığında analiz işçisi her hayvanı kendi
arenasında ölçer. Oturum sayfasının **Analiz** bölümü her hayvanın durumunu
gösterir; **Yeniden analiz et** yeni bir çalıştırma kuyruğa ekler (örneğin yeni
bir kalibrasyondan sonra).

Her sonuç sayılmadan önce bir kişi tarafından onaylanmalıdır. **Sonuç onayı**'nı
açın:

1. Liste, deneye ve duruma (bekleyen, kabul edilen, reddedilen, tümü) göre
   filtrelenmiş, onay bekleyen sonuçları gösterir. Filtreler adreste tutulur.
2. Analiz edilmiş videoyu, kalite kontrollerini ve metrikleri açmak için bir
   satıra tıklayın.
3. **Kabul et**'e tıklayın ya da bir neden seçin (takip kaybedildi, yanlış
   hayvan, video kusuru, protokol sapması, diğer), not ekleyip **Reddet**'e
   tıklayın. Çekmece ardından bir sonraki bekleyen sonuca geçer.
4. Bir hayvan yeniden analiz edildiyse kontroller **Bu hayvanın daha yeni bir
   çalıştırması var** der. Çalıştırma listesinden birini seçip **Bunun yerine bu
   çalıştırmayı incele**'ye tıklayın; sonuç o çalıştırmayla bekleyene döner.
5. **Temiz satırları seç**, tüm metrikleri ölçülmüş, daha yeni çalıştırması
   olmayan ve dışlanmamış her bekleyen sonucu işaretler; **Seçilenleri kabul
   et** hepsini birlikte kabul eder.

**Kör** bir çalışmada onay ekranı yalnızca hayvanı, arenayı ve oturumu gösterir;
grubu, dozu veya maddeyi asla göstermez.

## Sonuçlar ve raporlar

Deneyin **Sonuçlar** sekmesi ve **Raporlar** ekranındaki aynı rapor, her ölçütü
kendi zaman noktasında gösterir:

- grup başına: n, ortalama, SD, SEM ve başlangıca göre değişim; ayrıca kaç
  sonucun beklediği, analiz beklediği, reddedildiği, eksik olduğu veya
  dışlandığı;
- grup ortalamalarının grafiği;
- karşılaştırmalar sözle: model geçerliliği, yöntem duyarlılığı, kurtarma
  yüzdesiyle etkinlik, doz yanıtı ve zaman seyri.

Yalnızca dışlanmamış hayvanların kabul edilen sonuçları sayılır. Üstteki bir
şerit, sayıların **geçici** (bazı sonuçlar hâlâ bekliyor, onaylamak için bir
bağlantıyla), **ara rapor** (hepsi incelendi, çalışma sürüyor) veya **son
rapor** (çalışma tamamlandı) olup olmadığını söyler. **Dışa aktar** raporu CSV
veya Excel olarak, hayvan başına ya da grup başına bir satırla kaydeder. Kör
bir çalışmanın raporu paylaşılmadan önce bir uyarı taşır.

Bir çalışmayı bitirmek için, planlanan her oturum tamamlandığında veya iptal
edildiğinde **Genel bakış** sekmesinde **Çalışmayı tamamla**'ya tıklayın.
Tamamlanan bir çalışma salt okunurdur.

## Panel

**Panel** ana ekrandır. Kutucukları sonuçları (bekleyen, kabul edilen,
reddedilen), analizi (kuyrukta, çalışıyor, başarısız, kalibrasyon bekliyor),
oturumları (planlı, sürüyor, tamamlandı, iptal edildi) ve deneyleri (taslak,
çalışıyor, tamamlandı) sayar. Bir kutucuğa tıklamak saydığı listeyi filtrelenmiş
olarak açar: örneğin **Bekliyor** onay kuyruğunu, **Sürüyor** ise süren
oturumların iş listesini açar. Kutucukların altında son sonuçlar ve yaklaşan
oturumlar yer alır.

## Ağır veri nerede durur

Videolar doğrudan nesne depolamaya yüklenir; Mişko onların kayıtlarını,
kalibrasyonları, analiz çalıştırmalarını, metrikleri ve onay kararlarını tutar.
Analiz işçisiyle sözleşme [Entegrasyon](../integration/) sayfasında
anlatılmıştır.
