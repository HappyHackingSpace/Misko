---
title: Kullanım
description: Mişko panelinin etken maddeler ve hastalık modellerinden çalışma tasarımına, dozlamadan test oturumlarına, sonuç onayından raporlara kadar günlük kullanımı.
---

Bu sayfa, Mişko kurulduktan sonra onu kullanmaya yönelik pratik rehberdir.
Sadece geliştiriciler için değil, laboratuvardaki herkes için yazılmıştır.
Mişko'yu henüz kurmadıysanız [Kurulum](../installation/) ile başlayın.

## Genel resim

Mişko, klinik öncesi bir davranış çalışmasının kayıt defteridir. Bir çalışma tek
bir soru sorar: belirli bir dozdaki madde, bir hastalık modelinin yol açtığı
bozukluğu geri çeviriyor mu? Paneldeki her şey bu soruyu izler:

1. **Etken maddeler** ve **Hastalık modelleri**, laboratuvarın verdiği maddeleri
   ve oluşturduğu bozuklukları listeler; her birinin menüde kendi sayfası vardır.
2. **Denekler** deneklerdir; vücut ağırlıklarıyla birlikte tutulur.
3. **Ortamlar** arenalardır; her biri sabit kamerası için bir kez kalibre edilir.
4. Bir **deney**, gruplar (her birinin rolü, hastalık modeli ve aldığı uygulama),
   zaman noktaları, protokoller ve ölçütler olarak tasarlanır ve başlatılır.
5. Denekler gruplara **atanır**, her gün **dozlanır** ve **test oturumlarında**
   filme alınır.
6. Her denek oturum videosunda otomatik olarak ölçülür: her paradigmada bir
   analiz işçisi deneği izler. Her sonucu bir kişi **inceler** ve yalnızca kabul edilen sonuçlar **rapora**
   girer.
7. **Paradigma testi**, bir paradigmayı herhangi bir çalışmanın dışında tek bir
   videoda dener; örneğin bir kamerayı ya da analiz eşiklerini sınamak için.

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
| `RESEARCHER` | Kullanıcılar ve laboratuvar ayarları dışında her şey: etken maddeler, hastalık modelleri, denekler, ortamlar, çalışma tasarımı, oturum planlama, dozlama, oturum yürütme, sonuç onayı. |
| `TECHNICIAN` | Laboratuvar günü: doz ve ağırlık kaydı, oturumu başlatma ve tamamlama, deneme kaydı, video yükleme, bir deneği bir videoda kalibre etme, paradigma testi yürütme, sonuç onayı. Oturum planlayamaz, iptal edemez ve çalışmayı değiştiremez. |
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

Üst çubuktaki **Ara** (veya Ctrl+K, Mac'te Cmd+K) her sayfadan çalışmaları,
raporlarını, test oturumlarını ve denekleri bulur. Bir çalışma kodu ya da
başlığı, zaman noktası veya denek kodu yazın; ok tuşları ve Enter bir sonucu
açar, Esc kapatır.

## Etken maddeler

Menüden **Etken maddeler**'i açın (adres `/substances`). Bileşikler, referans
ilaçlar, taşıyıcılar (serum fizyolojik, CMC, DMSO) ve hastalık oluşturucular
(MPTP, LPS) burada tutulur. Liste ada göre aranabilir ve türe göre
filtrelenebilir. Eklemek için:

1. **Yeni madde**'ye tıklayın.
2. Adı yazın, türünü seçin ve isterseniz bir açıklama ekleyin.
3. **Kaydet**'e tıklayın.

Bir satırı değiştirmek için **Düzenle**'yi, kaldırmak için **Sil**'i kullanın;
silme onay ister. Tür yalnızca seçicilerdeki sırayı belirler; bir maddenin
çalışmadaki rolü nerede kullanıldığından gelir. Adlar benzersizdir: ikinci bir
"Saline" (büyük küçük harf fark etmeksizin) reddedilir. Bir rejimde ya da
kayıtlı bir dozda kullanılan madde silinemez; panel bunu söyler.

## Hastalık modelleri

Menüden **Hastalık modelleri**'ni açın (adres `/disease-models`). Bir hastalık
modeli, bozukluğun nasıl oluşturulduğunu (kimyasal, cerrahi, diyet, stres veya
diğer) ve nasıl doğrulandığını tanımlar. Liste ada göre aranabilir ve oluşturma
yöntemine göre filtrelenebilir.

1. **Yeni hastalık modeli**'ne tıklayın.
2. Adı yazın, oluşturma yöntemini seçin; isterseniz bir doğrulama ölçütü (örneğin
   "direk iniş süresi başlangıcın %150'sinin üzerinde") ve bir açıklama ekleyin.
3. **Kaydet**'e tıklayın.

Adlar, maddelerde olduğu gibi benzersizdir. Bir deney grubunun kullandığı model
silinemez; panel bunu söyler.

Bir deneydeki grup veya rejim formu henüz hastalık modeli ya da madde
bulamazsa ilgili sayfaya bağlantı verir.

## Denekler ve ağırlıklar

**Denekler**'i açın. Her deneğin bir kodu (harf, rakam, `.`, `_`, `/`, `-`),
türü (fare veya sıçan) ve cinsiyeti vardır.

1. **Yeni denek**'e tıklayın, kodu yazın, tür ve cinsiyeti seçin ve kaydedin.
2. Denekleri bulmak için arama kutusunu, tür ve cinsiyet filtrelerini kullanın.
   Henüz hiçbir gruba ait olmayanları görmek için **Yalnızca gruba
   atanmamışlar**'ı işaretleyin.
3. Deneğin sayfasını açmak için koduna tıklayın. Sayfa, çalışmasını ve grubunu
   (ya da atanmadığını) ve en yenisi üstte olmak üzere vücut ağırlıklarını,
   bir önceki tartıma göre farkla gösterir.
4. Bir deneği tartmak için sayfasında gramı ve zamanı yazıp **Ağırlığı
   kaydet**'e tıklayın. Ağırlıklar sonradan düzenlenmez.

Kodlar benzersizdir. Bir gruptaki veya ağırlığı olan denek silinemez.

## Ortamlar ve kalibrasyon

**Paradigmalar** sayfası Mişko'nun bildiği testleri, her birinin neyi
ölçtüğünü anlatan kısa bir açıklamayla listeler.

Ortam, bir paradigmadan (Morris su labirenti, yeni nesne kutusu, direk, koşu bandı gibi)
oluşturulan fiziksel bir arenadır. **Ortamlar**'ı açıp **Yeni ortam**'a
tıklayın:

1. Bir ad yazın (örneğin "Box A").
2. Paradigmayı seçin. Form, o paradigmanın gerektirdiği ölçüleri tipik
   değerlerle doldurulmuş olarak gösterir.
3. Gerçek ölçüleri girin (örneğin arenanın cm cinsinden eni ve boyu). Analiz
   ayarları katlanmıştır ve nadiren değiştirilmesi gerekir.
4. **Kaydet**'e tıklayın. Ortamın sayfası açılır.

**Varsayılan kalibrasyon.** Yeri değişmeyen bir kamera her arena için bir kez
kalibre edilir ve o arenada çekilen her video bunu kullanır. Kalibrasyon,
kamera piksellerinin düzenek üzerinde santimetreye nasıl karşılık geldiğini ve
çizilen bölgeler tanımlayan bir paradigmada bu bölgelerin nerede olduğunu
Mişko'ya söyler. Her paradigma kalibre edilir ve kalibratör paradigmaya göre
kısa bir rehber gösterir: kameranın nereye konacağı ve neyin çizileceği.

| Paradigma | Kamera | Düzenek çerçevesi | Bölgeler |
|---|---|---|---|
| Morris su labirenti | Tankın üstünde | Tankın merkezi, x doğuya, y kuzeye | çizilmez |
| Yeni nesne | Arenanın üstünde | Zeminin başlangıç köşesi, x genişlik, y derinlik boyunca | **Yeni nesne** ve **tanıdık nesne**: her nesnenin çevresinde, ondan yaklaşık 2 cm geniş bir daire |
| Direk | Direğin yanında | x direğe uzaklık, y tabandan yükseklik (taban 0, tepe direk uzunluğu) | **Direk** (deneğin tırmandığı yer) ve **taban** (dibindeki zemin), çokgen |
| Koşu bandı | Kulvarın yanında | x kulvar boyunca şok ızgarası ucundan (0) öne, y banttan yukarı | Kulvarın arkasındaki **şok ızgarası**, çokgen |

1. Ortamın sayfasında **Bu revizyonu kalibre et**'e tıklayın. Görsel
   kalibratör açılır.
2. **Kare için video** altında **Video dosyası seç**'e tıklayıp bu kamerayla
   çekilmiş bir video seçin. Dosya yalnızca bir kare göstermek için
   tarayıcınızda okunur; asla yüklenmez. Kare boyutu videodan okunur. Düzeneğin
   açıkça göründüğü bir kareyi kaydırıcıyla ve kare adımı düğmeleriyle seçin.
3. **Kamera** adını yazın ve **Ölçüm düzlemi**'ni kontrol edin (arena tabanı,
   su labirentinde su yüzeyi).
4. **Uyum noktaları** aracıyla her uyum noktasına karede tıklayın, sonra
   sürükleyerek ayarlayın. Vurgulanan satır, bir sonraki tıklamanın
   yerleştireceği noktadır; başka bir nokta seçmek için satırına tıklayın.
   **Düzenek noktalarını kullan** gerçek konumları ortamın ölçülerinden
   doldurur, siz yalnızca sırayla tıklarsınız:
   - su labirenti: tank duvarının kuzey, doğu, güney ve batı noktaları, tankın
     merkezinden ölçülür (x doğuya, y kuzeye doğru artar).
   - yeni nesne kutusu: tabanın dört iç köşesi, (0, 0) başlangıç köşesinden
     genişlik ve derinlik boyunca.
   - direk: direğin tabanı (yükseklik 0) ve tepesi (direk uzunluğu). İkisi aynı
     dikey doğru üzerinde olduğundan direkten uzakta düzenek üzerinde iki
     işaret daha gerekir (örneğin arka panelde): direğe uzaklıklarını ve
     tabandan yüksekliklerini ölçüp yazın. Uyum noktaları tek bir doğru
     üzerindeyse kalibratör bunu söyler.
   - koşu bandı: kulvar duvarının ızgara ucundaki (0) ve öndeki (kulvar
     uzunluğu) köşeleri, bant üzerinde ve duvarın üstünde. Üst noktaları
     doldurmak için **Kulvar duvarı yüksekliği**'ni yazın.

   Hazır nokta yoksa tıkladığınız her noktanın cm cinsinden gerçek konumunu,
   istediğiniz bir çerçevede (örneğin düzeneğin merkezine göre) yazın.
5. **Kontrol noktaları** aracıyla uyum noktalarından farklı yerlere en az üç
   kontrol noktası koyun (hazır noktalar kenarların ortalarını, tankın çapraz
   duvar noktalarını, iki düzenek işaretiyle birlikte direğin ortasını ya da
   kulvarın ortasını kullanır).
6. **Kırpma** aracıyla bu arenanın çevresine bir dikdörtgen sürükleyin.
   Noktalar ve bölgeler bunun içinde kalmalıdır. **Tüm kare** onu sıfırlar.
7. Paradigmanın çizilen bölgeleri varsa (yeni nesne, direk, koşu bandı)
   **Bölgeler** aracını açın. Her bölge **Zorunlu** ya da **İsteğe bağlı**
   olarak listelenir. Bir bölgenin yanındaki **Daire çiz** ya da **Çokgen
   çiz**'e tıklayıp onu karede çizin (nesne bölgeleri önce daire olmak üzere
   ikisini de sunar; direk, taban ve şok ızgarası bölgeleri çokgendir):
   - çokgen: her köşeye tıklayın, kapatmak için ilk köşeye yeniden tıklayın (ya
     da **Çokgeni kapat**); **Son köşeyi geri al** son köşeyi kaldırır;
   - daire: merkeze basıp kenara kadar sürükleyin.

   Bir bölge birden çok şekil gerektirebilir; bunları aynı bölge altında art
   arda çizin. Çizerken önceden çizilmiş bir şeklin köşesine
   ya da merkezine tıklamak oraya yapışır, böylece komşu bölgeler kenarlarını
   tam olarak paylaşır. Biten bir şekli tutamaçlarını sürükleyerek ayarlayın ya
   da **Yeniden çiz** ve **Sil**'i kullanın. **Çizimi bitir** o anki bölgeyi
   bitirir. Yeni nesnede **Yer değiştir: Yeni nesne ve Tanıdık nesne** çizilen
   iki bölgenin yerini değiştirir; örneğin yeni nesnesi öbür tarafta duran bir deneğin
   kalibrasyonunda (dengeleme).
8. Her değer **Değerleri elle gir** altında elle de yazılabilir. Karenin
   altındaki liste neyin eksik olduğunu söyler.
9. **Kalibrasyonu kaydet**'e tıklayın. Panel uyum ve kontrol hatalarını,
   toleransı ve her bölgeyi piksel ve cm cinsinden gösterir. Kontrol hatası
   toleransı aşan kalibrasyon reddedilir; daha iyi noktalarla yeniden kaydedin.

Bu arenadaki analizler, her zorunlu bölgesi olan geçerli bir kalibrasyon olana
kadar bekler. Kaydedilmiş bir kalibrasyon asla değişmez: **Kalibrasyonu
düzelt** onun yerini alan yenisini kaydeder ve önceki kalibrasyonlar saklanır.

**Revizyonlar.** Ölçüler yerinde değiştirilmez, böylece her test çalıştırıldığı
değerlerle kalır. Yeni bir revizyon oluşturmak için **Ölçüleri değiştir**'e
tıklayın. Yeni revizyonun kendi kalibrasyonu gerekir.

## Bir çalışmayı tasarlama

**Deneyler**'i açıp **Yeni deney**'e tıklayın. Kodu, başlığı, hipotezi,
deneklerin nasıl rastgele dağıtıldığını ve çalışmanın kör olup olmadığını
doldurup kaydedin. Deney **Gruplar ve rejimler** sekmesinde açılır. Yeni bir
çalışma **Taslak**'tır; başlayana kadar tasarımı değiştirilebilir.

**Gruplar ve rejimler.**

1. **Grup ekle**'ye tıklayın. Bir kod (G1, G2), ad, rol ve hedef denek sayısı
   verin. Sağlıklı kontrol dışındaki her rol bir hastalık modeli ister.
2. Grubun kartında **Kalem ekle**'ye tıklayarak grubun ne aldığını belirtin:
   amaç (indüksiyon, taşıyıcı veya tedavi), madde, doz ve birim (örneğin
   10 mg/kg), uygulama yolu, sıklık (bir kez, günlük, günde iki kez, haftalık
   veya N günde bir) ve ilk ile son çalışma günü. Amaç listesi yalnızca grubun
   rolünün alabileceklerini sunar; tek bir amaç uyuyorsa o zaten seçilidir.
   Her kartta **Rejim** başlığının altındaki kısa satır, rolün neyi alması
   gerektiğini ve neyi almadığını söyler. Sağlıklı kontrol hiçbir madde
   almadığı için onun **Kalem ekle** penceresi yalnızca hiçbir şey almadığı
   dönemin **İlk çalışma günü** ve **Son çalışma günü** bilgisini ister;
   aşağıya bakın.

Roller şunlardır: sağlıklı kontrol, sham, hastalık kontrolü, pozitif kontrol ve
tedavi. **Her çalışmada en az bir sağlıklı kontrol grubu olmalıdır**: model
geçerliliği ve kurtarma yüzdesi bununla ölçülür, bu yüzden sağlıklı kontrolü
olmayan bir çalışma başlatılamaz. Her rolün alabilecekleri:

| Rol | Almalı | Ayrıca alabilir | Asla almaz |
| --- | --- | --- | --- |
| Sağlıklı kontrol | hiçbir madde (yalnızca dönemi kaydedilir) | | taşıyıcı, indüksiyon, tedavi |
| Sham | taşıyıcı | | indüksiyon, tedavi |
| Hastalık kontrolü | indüksiyon ve taşıyıcı | | tedavi |
| Pozitif kontrol | indüksiyon ve bir tedavi (referans ilaç) | taşıyıcı | ikinci bir tedavi |
| Tedavi | indüksiyon ve bir tedavi (doz düzeyi) | taşıyıcı | ikinci bir tedavi |

Panel ve API, rolün asla almadığı bir kalemi ("Bu grubun rolü bu amaçla bir
kalem alamaz") ve bir grubun ikinci tedavi kalemini reddeder.

**Sağlıklı kontrol: madde yok, yalnızca dönem.** Sağlıklı kontrol *naif*
kontroldür ve hiçbir madde almaz: taşıyıcı, indüksiyon ve tedavi yoktur.
Kartında "Bu gruba madde uygulanmaz (naif kontrol)" yazar. Grubun hangi
günlerde hiçbir madde almadığını kaydetmek için kartındaki **Kalem ekle**'ye
tıklayın ve yalnızca **İlk çalışma günü** ile **Son çalışma günü**'nü girin;
amaç, madde, doz, birim, yol ve sıklık yoktur. Kalem "Madde uygulanmaz: gün 0
ile 14 arası" olarak görünür ve Dozlama sekmesi grubu "Madde uygulanmaz" ve bu
dönemle listeler. Dönem isteğe bağlıdır; kalemi olmayan bir sağlıklı kontrol
de geçerlidir. Eski bir taslakta sağlıklı kontrole girilmiş bir taşıyıcı,
çalışmanın başlamasını engeller ("Sağlıklı kontrol grubunda olmaması gereken
bir taşıyıcı kalemi var"): kalemi düzenleyip iki günü kaydederek döneme
çevirin ya da silin. Başlamış çalışmaların tasarımı değişmeden kalır.

**Zaman noktaları ve protokoller.** **Zaman noktaları ve protokoller**
sekmesinde:

1. Deneklerin test edildiği her an için **Zaman noktası ekle**'ye tıklayın;
   bir ad (Baseline, 7. gün) ve çalışma günü verin (0. günden önce eksi).
2. **Yeni protokol**'e tıklayın, ad verin ve her test için bir adım ekleyin:
   paradigma, deneme türü ve deneme sayısı. Oturum ayarları olan paradigmalar
   (örneğin koşu bandı hızları) bunları adımda gösterir.
3. Bir protokolü sonradan değiştirmek için **Yeni sürüm**'e tıklayın. Önceki
   sürümler değişmez; oturumlar planlandıkları sürümü korur.

**Ölçütler.** **Ölçütler** sekmesinde **Ölçüt ekle**'ye tıklayarak çalışmanın
neye göre değerlendirileceğini ekleyin: protokollerdeki bir paradigmanın bir
metriği, bir zaman noktasında, isteğe bağlı olarak bir başlangıç zaman
noktasıyla karşılaştırmalı, ve hangi yönün daha iyi olduğu. Tam olarak bir ölçüt
**birincil**'dir; ikinci bir birincil ölçüt reddedilir.

**Başlatma.** **Genel bakış** sekmesi henüz karşılanmayan her başlatma kuralını,
düzelteceği sekmeye bir bağlantıyla birlikte listeler. Sağlıklı kontrol
eksikse **Sağlıklı kontrol grubu ekle** grup penceresini bu rol seçili olarak
açar. Liste çalışmanın hazır
olduğunu söylediğinde **Çalışmayı başlat**'a tıklayın, 0. günü (çalışma
günlerinin sayıldığı tarih, genellikle indüksiyon) seçin ve onaylayın. Çalışma
**Çalışıyor** olur: gruplar, rejimler, zaman noktaları ve ölçütler artık salt
okunur gösterilir. Yeni protokol sürümleri hâlâ eklenebilir.

## Atama ve dışlama

**Denekler** sekmesinde:

1. **Denek ata**'ya tıklayın. Çekmece hiçbir gruba ait olmayan denekleri
   listeler.
2. Arayın ve denekleri işaretleyin.
3. **Tek grup**'u seçip grubu belirleyin ya da sistemin onları hedeflerine kadar
   gruplara yayması için **Rastgele dağıt**'ı seçin (isterseniz yalnızca bazı
   gruplara). Bir kafes eklenebilir.
4. **Ata** veya **Rastgele dağıt**'a tıklayın.

Üstteki grup kartları her grubun ne kadar dolu olduğunu gösterir. Bir denek tam
olarak bir gruba aittir ve çalışma başladıktan sonra grubu değişmez. Bir denek
ölürse ya da çalışmadan çıkması gerekirse **Dışla**'ya tıklayın, nedeni seçin
(öldü, insani son nokta, indüksiyon başarısız, teknik sorun, diğer) ve bir not
ekleyin. Dışlanan denekler nedenleriyle listede kalır ve istatistiklere
girmez.

## Günlük dozlama

**Dozlama** sekmesi bir günün verilecek dozlarını grup ve deneğe göre, her
deneğin son ağırlığıyla birlikte gösterir. Başka bir güne geçmek için okları
veya tarih seçiciyi, geri dönmek için **Bugün**'ü kullanın. Sağlıklı kontrolün
hiçbir zaman verilecek dozu olmaz: altta **Madde uygulanmaz** ve dönemiyle
(örneğin "Madde uygulanmaz: gün 0 ile 14 arası"), rejim kalemi olmayan diğer
gruplar **Uygulama yok** ile listelenir. Bir doz bugün
**Bekliyor**, kaydedilince **Verildi**, geçmiş bir günde **Eksik** ve gelecek
bir günde **Planlandı** olarak görünür.

1. Bekleyen dozun yanındaki **Kaydet**'e tıklayın. Pencere planlanan dozu
   gösterir; zamanı, miktarı veya yolu yalnızca verilen farklıysa değiştirin.
2. Kilogram başına doz son yedi günden bir ağırlık ister. Yoksa pencere önce
   ağırlığı sorar, kaydeder ve dozu onunla kaydeder. Liste, planlanan mg/kg
   dozun yanında mg cinsinden dozu gösterir.
3. Planda olmayan bir doz için **Plan dışı doz**'a tıklayıp deneği, amacı,
   maddeyi, miktarı, birimi ve yolu seçin.

**İndüksiyon sonuçları.** İndüksiyon uygulanan denekler **İndüksiyon
sonuçları** altında listelenir. **Sonucu kaydet**'e tıklayın, doğrulandı veya
doğrulanmadı seçip kaydedin. Bir sonuç bir kez kaydedilir.

Alttaki **Doz geçmişi** kaydedilen her dozu listeler ve deneğe ve amaca göre
filtrelenebilir.

## Test oturumlarını planlama ve yürütme

Bir test oturumu; bir zaman noktası, bir protokol adımı ve **tek bir videodur**.
Bir veya daha fazla denek içerir, her biri kendi arenasındadır (örneğin tek
kameranın çektiği üç yeni nesne kutusu).

**Planlama** (araştırmacı ve üstü), deneyin **Testler** sekmesinde:

1. **Oturum planla**'ya tıklayın.
2. Zaman noktasını seçin; protokol, sürüm ve adım önceden seçilidir.
3. Zaman varsayılan olarak 0. gün artı zaman noktasının çalışma günüdür.
4. İlk deneği ve arenasını seçin, ardından her ek denek için **Denek
   ekle**'ye tıklayın. Yalnızca dışlanmamış ve aynı zaman noktası ile adımda
   zaten planlanmamış denekler, ve yalnızca adımın paradigmasına ait arenalar
   sunulur.
5. **Planla**'ya tıklayın.

Oturum planlıyken sayfasındaki **Denekleri düzenle** denek ekler veya
çıkarır. **Oturumu iptal et** bir neden ister ve bu neden oturumda saklanır.

**İş listesi.** **Testler** menüsü, oturumları çalışmalar arasında dört görünümde
listeler: **Bugün**, **Geciken** (daha önceki bir günden planlı ya da hâlâ
süren), **Yaklaşan** ve **Tümü**; deney, durum ve paradigma filtreleriyle.
Görünüm ve filtreler adreste tutulur.

**Yürütme** (teknisyen ve üstü), oturumun sayfasında:

1. **Başlat**'a tıklayın. Dışlanmış bir deneği olan oturum, o denek
   çıkarılmadan başlamaz.
2. Her denek için kartındaki **Deneme kaydet**'e tıklayın, tekrar numarasını
   ve zamanları kontrol edip kaydedin. Kart yapılan denemeleri planlanan sayıyla
   karşılaştırır.
3. Oturum videosunu **Video yükle** ile yükleyin. Video yüklendikten sonra
   doğrulanır ve kullanılan video olarak gösterilir. **Videoyu değiştir** bir
   onaydan sonra yenisini yükler; önceki videolar **Önceki videolar** altında
   listelenir.
4. Her deneğin kartı, görüntüdeki kendi bölümünün hangi kalibrasyonu
   kullandığını gösterir: normalde **Arena varsayılanı**. Kamera veya bir arena
   kaydıysa o denekte **Bu videoda kalibre et**'e tıklayın. Aynı kalibratör,
   arenanın noktaları ve bölgeleriyle başlayarak oturumun kendi videosunun bir
   karesinde açılır: noktaları ve bölgeleri bu videodaki yerlerine taşıyıp
   kaydedin. Kullandığınız kare kalibrasyonla birlikte saklanır. Bu kalibrasyon
   yalnızca bu videodaki bu denek için geçerlidir; diğer denekler arena
   varsayılanını korur. Reddedilirse denek arena varsayılanına dönmez, yeni
   bir kalibrasyon bekler.
5. Her deneğin en az bir denemesi olduğunda **Tamamla**'ya tıklayın; o zamana
   kadar düğme pasif kalır ve sayfa nedenini söyler.

Sayfanın altındaki yorumlar, laboratuvarın olağan dışı her şeyi not etmesini
sağlar. Herkes yorum yazabilir; kendi yorumunuzu düzenleyebilirsiniz,
laboratuvar yöneticileri herhangi bir yorumu silebilir.

## Analiz ve sonuç onayı

Her paradigma otomatik olarak analiz edilir: bir analiz işçisi deneği videoda
izler ve Mişko olayları (her nesnenin keşfi, direkte dönüş, tabana ulaşma ve
düşme, koşu bandında şok ve tükenme) bu izden ve kalibre edilen bölgelerden
çıkarır. Bir kişi yalnızca sonucu inceler.

Süren bir oturumun videosu doğrulandığında ve
deneğin kalibrasyonu geçerli olduğunda analiz işçisi her deneği kendi
arenasında ölçer. Oturum sayfasının **Analiz** bölümü her deneğin durumunu
gösterir; **Yeniden analiz et** yeni bir çalıştırma kuyruğa ekler (örneğin yeni
bir kalibrasyondan sonra).

Her sonuç sayılmadan önce bir kişi tarafından onaylanmalıdır. **Sonuç onayı**'nı
açın:

1. Liste, deneye ve duruma (bekleyen, kabul edilen, reddedilen, tümü) göre
   filtrelenmiş, onay bekleyen sonuçları gösterir. Filtreler adreste tutulur.
2. Analiz edilmiş videoyu, kalite kontrollerini ve metrikleri açmak için bir
   satıra tıklayın.
3. **Kabul et**'e tıklayın ya da bir neden seçin (takip kaybedildi, yanlış
   denek, video kusuru, protokol sapması, diğer), not ekleyip **Reddet**'e
   tıklayın. Çekmece ardından bir sonraki bekleyen sonuca geçer.
4. Bir denek yeniden analiz edildiyse kontroller **Bu deneğin daha yeni bir
   çalıştırması var** der. Çalıştırma listesinden birini seçip **Bunun yerine bu
   çalıştırmayı incele**'ye tıklayın; sonuç o çalıştırmayla bekleyene döner.
5. **Temiz satırları seç**, tüm metrikleri ölçülmüş, daha yeni çalıştırması
   olmayan ve dışlanmamış her bekleyen sonucu işaretler; **Seçilenleri kabul
   et** hepsini birlikte kabul eder.

**Kör** bir çalışmada onay ekranı yalnızca deneği, arenayı ve oturumu gösterir;
grubu, dozu veya maddeyi asla göstermez.

## Paradigma testi

Ana menüdeki **Paradigma testi**, bir paradigmayı herhangi bir deneyin dışında
tek bir videoda, tek bir ortamda denemenizi sağlar: grup, sonuç onayı ve rapor
yoktur; bir testin sonucu hiçbir zaman bir deneye taşınmaz. Bir çalışmadan önce
yeni bir kamera konumunu, bir ortamı ya da analiz eşiklerini sınamak için
kullanın. Testleri herkes okuyabilir; test başlatmak ve yürütmek `trial:run`
yetkisi ister (`VIEWER` dışındaki her rol).

1. **Yeni test**'e tıklayın, bir ad verin, paradigmayı ve denenecek ortam
   revizyonunu seçin, gerekirse not ekleyip **Testi oluştur**'a tıklayın. Test
   sayfası açılır.
2. **Video yükle**'ye tıklayıp kaydı seçin. Dosya doğrudan depolamaya gider ve
   doğrulanır; ardından oynatıcıda görünür. **Videoyu değiştir** başka bir video
   yükler; önceki analizler kullandıkları videoyu korur.
3. **Kalibrasyon** kartı hangi kalibrasyonun uygulandığını söyler: ortamınki ya
   da bu videoda yapılan testin kendi kalibrasyonu. Ortam kalibre edilmemişse ya
   da kamera oynadıysa **Bu videoda kalibre et**'e tıklayıp noktaları ve
   bölgeleri ortam sayfasındaki gibi yerleştirin. Bu kalibrasyon yalnızca bu
   teste ve bu videoya aittir; ortam hiç değişmez.
4. Video doğrulanıp bir kalibrasyon uygulandığında analiz kendiliğinden başlar.
   **Analizler** kartı her analizi en yenisi üstte listeler: durumu (sırada,
   sürüyor, başarılı, nedeniyle birlikte başarısız), izleme kalitesi, panelin
   dilinde ölçütler ve **Çizimli video ve olaylar** altında işçinin çizdiği video
   ile olaylar; bir olaya tıklamak onu oynatır. Bir analiz sıradayken ya da
   sürerken sayfa kendini yeniler.
5. **Yeniden analiz et**, paradigmanın analiz eşiklerini ortamın değerleriyle
   doldurulmuş olarak açar. Denemek istediğinizi değiştirip **Analiz et**'e
   tıklayın: aynı videonun yeni bir analizi yalnızca değişen değerlerle sıraya
   girer, öncekiler kalır. **Karşılaştır**, başarılı iki analizi her ölçütün
   farkıyla yan yana gösterir.
6. **Sil**, onay istedikten sonra testi listeden kaldırır; analizleri saklı
   kalır.

## Sonuçlar ve raporlar

Deneyin **Sonuçlar** sekmesi ve **Raporlar** ekranındaki aynı rapor, her ölçütü
kendi zaman noktasında gösterir:

- grup başına: n, ortalama, SD, SEM ve başlangıca göre değişim; ayrıca kaç
  sonucun beklediği, analiz beklediği, reddedildiği, eksik olduğu veya
  dışlandığı;
- grup ortalamalarının grafiği;
- karşılaştırmalar sözle: model geçerliliği, yöntem duyarlılığı, kurtarma
  yüzdesiyle etkinlik, doz yanıtı ve zaman seyri.

Yalnızca dışlanmamış deneklerin kabul edilen sonuçları sayılır. Üstteki bir
şerit, sayıların **geçici** (bazı sonuçlar hâlâ bekliyor, onaylamak için bir
bağlantıyla), **ara rapor** (hepsi incelendi, çalışma sürüyor) veya **son
rapor** (çalışma tamamlandı) olup olmadığını söyler. **Dışa aktar** raporu CSV
veya Excel olarak, denek başına ya da grup başına bir satırla kaydeder. Kör
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
