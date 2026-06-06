---
title: Kullanım
description: Mişko panelinin ilk girişten test çalıştırmaya kadar günlük kullanımı.
---

Bu sayfa, Mişko kurulduktan sonra onu kullanmaya yönelik pratik rehberdir.
Sadece geliştiriciler için değil, laboratuvardaki herkes için yazılmıştır.
Mişko'yu henüz kurmadıysanız [Kurulum](../installation/) ile başlayın.

## Genel resim (teknik olmayan)

Mişko'yu davranış deneylerinizin kayıt defteri gibi düşünün. Elektronik tablolar
ve dağınık video dosyaları yerine her test tek bir yerde kayıt altına alınır:
**hangi hayvan test edildi, hangi senaryoyla, kim tarafından ve sonuç neydi**.
Ağır video ve takip işini ayrı bir kamera sistemi yürütür; Mişko
ise düzenli özeti tutar, böylece sonuçları sonradan bulup karşılaştırabilirsiniz.

Normal bir oturum şöyle ilerler:

1. Bir yönetici laboratuvarı kurar: kullanıcılar, denekler (fareler), ortamlar ve senaryolar.
2. Bir operatör; senaryo ve denek seçerek bir test oluşturur.
3. Test yaşam döngüsünden geçer: **beklemede** başlar, deney sürerken
   **çalışıyor** olur ve **tamamlandı** ya da **başarısız** olarak biter.
4. Sonuç özeti saklanır ve panoda görünür.

## İlk giriş

Public kayıt yoktur. Kurulumdan sonra tek bir **superadmin** hesabı vardır ve
şifresi backend log'una bir kez yazılmıştır (bkz.
[Kurulum](../installation/#ilk-giriş-şifresini-alın)).

1. Paneli açın (varsayılan [http://localhost:8080](http://localhost:8080)).
2. `ADMIN_EMAIL` ve üretilen şifre ile giriş yapın.
3. Şifrenizi hemen hesabınızdan değiştirin.
4. Ekibin geri kalanını **Kullanıcılar** ekranından oluşturun.

## Kullanıcılar ve roller

Kullanıcılar bir yönetici tarafından **Kullanıcılar** ekranından içeriden
yönetilir. Kendi kendine kayıt yoktur; bu, sistemi kapalı ve tek bir
laboratuvara uygun tutar.

Beş rol vardır: `SUPERADMIN`, `LAB_MANAGER`, `RESEARCHER`, `TECHNICIAN` ve
`VIEWER`.

- **SUPERADMIN** veya **LAB_MANAGER** kullanıcıları yönetebilir ve laboratuvarı
  yapılandırabilir.
- `apparatus:write` iznine sahip herkes (RESEARCHER ve üzeri), salt okunur
  paradigma kataloğundan adlandırılmış ortamlar oluşturabilir.
- Diğer kullanıcılar rollerine göre laboratuvar verisiyle çalışır.
- Kullanıcı oluştururken şifreyi boş bırakıp sistemin güçlü bir şifre üretmesine
  izin verebilirsiniz; bu şifre yalnızca bir kez gösterilir, kapatmadan önce
  kopyalayın.

## Laboratuvarı yapılandırma

Mişko kurulum başına tek bir laboratuvar olarak çalışır. Laboratuvar kaydı (adı
ve ayarları) ilk açılışta kurulum sihirbazı tarafından oluşturulur ve panel
genelinde markalama için kullanılır.

**Paradigmalar** ekranı, kodda tanımlı bilimsel test türlerinin salt okunur
kataloğudur. Her paradigmanın kendi detay sayfası vardır (bir karta tıklayarak
açılır); operasyonel kontrat burada salt okunur olarak gösterilir: apparatus
parametreleri, bölgeler, metrikler, önerilen kabul kriterleri ve kalite kontrol
gereksinimleri. Parametre kümesi ve geçerli aralıkları kod tarafından sabittir ve
burada düzenlenemez.

Bir paradigmayı kullanıma almak için `apparatus:write` iznine sahip herkes
(RESEARCHER ve üzeri) ondan bir **ortam** oluşturur. Ortam, bir paradigma
şablonuna dayanan adlandırılmış ve kalıcı bir test düzenidir; bir laboratuvar aynı
paradigma için birçok ortam tutabilir (örneğin "Tank A" ve "Tank B" adlı iki
Morris su tankı). Paradigma detay sayfasından **Bu paradigmadan ortam oluştur**'u
kullanın, bir ad verin ve apparatus değerlerini kod tarafından sabitlenen izin
verilen aralıklar içinde doldurun. Ortamları **Ortamlar** menüsünden yönetin; bu
menü oluşturma, düzenleme ve silmeyi destekler; düzenlemede paradigma
değiştirilemez. Apparatus değerleri test anında kilitlenir.

## Laboratuvar verisini kurma

Test çalıştırmadan önce yapı taşlarını doldurun. Bunlar panelde kendi
ekranlarında bulunur.

### Senaryolar

Senaryo, bir deneyin eksiksiz ve yeniden kullanılabilir tanımıdır. Bir kez
kurarsınız: ad verin, bir veya birden çok **ortam** seçin (böylece bir senaryo
bir veya birden çok paradigmayı kapsayabilir) ve **her ortam için beklenen
sonuçları** tanımlayın - her ortam, kendi paradigma metriklerinden seçilen kendi
kabul kriterlerini alır. Bir ortam eklediğinizde beklenen sonuçlar, paradigmanın
varsayılan kriterleriyle ön-doldurulur; siz de ayarlarsınız. Sonrasında test
başlatmak yalnızca bir denek seçmek demektir; gerisini senaryo taşır.

### Denekler

Denekler farelerdir. Her deneğin bir kodu, cinsiyeti, grubu ve notları vardır.
Deneklerin kolay bulunması için tutarlı bir kodlama şeması kullanın (örneğin
`F-001`).

## Veriyi tarama ve bulma

Her liste ekranı (Kullanıcılar, Denekler, Senaryolar, Paradigmalar,
Testler ve Ortamlar) aynı tabloyu kullanır, dolayısıyla kontroller her yerde
aynı çalışır:

- Üstteki **arama kutusu**, listeyi ana sütunlar üzerinden serbest metinle
  filtreler (örneğin bir denek kodu, kullanıcı e-postası ya da senaryo adı).
- **Sıralanabilir başlıklar** etkin olduğunda bir ok gösterir. Sıralamak için
  başlığa tıklayın, yönü çevirmek için tekrar tıklayın.
- **Sayfalama** alt bilgide bulunur: sayfa başına kaç satır (10, 20 veya 50)
  olacağını seçin, "kaçtan kaça / toplam" özetini görün ve Önceki / Sonraki ile
  ilerleyin.
- Baştaki **# sütunu** satırları numaralandırır ve sayfalar arasında saymaya
  devam eder; yani 2. sayfanın ilk satırı, 1. sayfanın bittiği yerden devam eder.
- Sağ üstteki **dışa aktarma** açılır menüsü geçerli sayfayı kaydeder. CSV, Excel
  veya Google E-Tablolar'da açılır; PDF ise tarayıcının yazdırma penceresini
  açar, oradan "PDF olarak kaydet"i seçebilir veya kâğıda yazdırabilirsiniz. Dışa
  aktarma; geçerli arama ve sıralama dahil, gördüğünüz değerleri kullanır.

Bunların hepsi sunucuda çalışır; böylece laboratuvar ne kadar kayıt biriktirirse
biriktirsin tablo hızlı kalır: yalnızca geçerli sayfa çekilir, arama ve sıralama
veritabanı tarafından uygulanır. Bir ekranda henüz kayıt yoksa, tablo boş bir
ızgara yerine neyin eksik olduğunu belirten kısa bir mesaj gösterir.

Her liste salt görüntülemedir. Sayfa başlığındaki **Oluştur** düğmesi yeni bir
kayıt için ayrı bir form sayfası açar; bir satırın ilk sütununa tıklamak o kaydın
**detay sayfasını** açar. Düzenleme, silme ve kayda özel işlemler (örneğin bir
testi başlatmak veya bitirmek, ya da bir kullanıcının şifresini sıfırlamak)
burada yapılır. Kaydetme veya silme sizi listeye geri döndürür.

## Test çalıştırma

Test, Mişko'daki merkezî kayıttır. Bir test çalıştırmak için:

1. **Testler** ekranına gidin ve yeni test sayfasını açmak için **Oluştur**'a
   tıklayın.
2. Bir **senaryo** ve bir **denek** seçip oluşturun. Operatör, giriş yapan
   kullanıcıdır.
3. Test **beklemede** durumuyla oluşturulur. Yönetmek için (satıra tıklayarak)
   **detay sayfasını** açın.
4. Detay sayfasında **Başlat**, durumu **çalışıyor** yapar.
5. **Bitir**, durumu **tamamlandı** yapar (**İptal** ise **başarısız**). Detay
   sayfası senaryonun ortamlarını ve sonuç geldiğinde metrik JSON'unu ve geç/kal
   verdiktini gösterir. Verdikt, senaryonun beklenen sonuçlarına göre hesaplanır
   (test bazlı değil, senaryoda tanımlı).

Bir test tamamlandığında özet metrikleri ve varsa artefakt bağlantıları Mişko'da
tutulur. Ham video ve kare kare veri ise ayrı kamera servisinde kalır.

## Pano

**Pano** ana ekrandır. Özet sayıları (kaç denek, senaryo, test) ve en son testleri
gösterir; böylece laboratuvar etkinliğini bir bakışta görürsünüz.

## Ağır veri nerede durur

Mişko bilinçli olarak ham video veya görü telemetrisi saklamaz. Bu veri bağımsız
bir kamera servisine aittir. Mişko her test için yalnızca **özet metrikleri ve
artefakt URL'lerini** tutar. İki sistem arasındaki kontrat
[Entegrasyon](../integration/) sayfasında anlatılır.
