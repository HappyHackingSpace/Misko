---
title: Yol haritası
description: Mişko bugün neler yapıyor ve sırada ne var.
---

Mişko **yalnızca videoya dayanan** bir davranış testi platformudur: her ölçüm bir
kayıttan, onun kalibrasyonundan ve kodda tanımlı bir paradigma sürümünden gelir.
Planda sensör yoktur. Açık işler
[GitHub issue'larında](https://github.com/HappyHackingSpace/Misko/issues) izlenir.

## Bitenler

- **Laboratuvar ve erişim**: kurulum başına tek laboratuvar, kurulum komutuyla
  oluşturulan süper yönetici, izin matrisiyle beş rol, kullanıcı yönetimi.
- **Katalog**: etken maddeler, hastalık modelleri ve hiç değişmeyen
  yayımlanmış sürümlerde dört paradigma (Morris su labirenti, yeni nesne,
  direk testi, koşu bandı); parametreleri, bölgeleri, ölçümleri, olayları ve
  kalite kuralları İngilizce ve Türkçe.
- **Ortamlar**: ölçüm revizyonlarıyla tek paradigmalı düzenekler ve arena
  başına kalibrasyon.
- **Çalışma tasarımı**: rollü gruplar, rejimler, zaman noktaları, protokoller
  ve sonlanım noktalarıyla deneyler; 0. günden önce denetlenen başlatma
  kuralları.
- **Yürütme**: atama ve dışlama, günlük dozlama ve indüksiyon sonuçları, tek
  videolu ve arenalarında birden çok hayvanlı test oturumları, denemeler ve
  yorumlar.
- **Analiz**: işçilerin aldığı işler, doğrulanan ve kalite kurallarıyla
  denetlenen yörüngeler, Go motorunun hesapladığı her ölçüm ve olay; herhangi
  bir çalışmanın dışında paradigma denemeleri.
- **Sonuçlar**: her sonucun insan onayı, grup rollerinden çıkan
  karşılaştırmalarla çalışma raporu, CSV ve Excel dışa aktarma, gösterge
  paneli ve genel arama.

## Sırada

- **İzlemeyi gerçek kayıtlarla doğrulamak.** Paketle gelen işçi yalnızca
  sentetik videoyla test edildi. Sonuçlar araştırmada kullanılmadan önce,
  sayıları gerçek oturumların kör el ölçümleriyle karşılaştırılmalı ve hareketsiz
  bir hayvanın mesafeye ve hareketsizliğe kattığı hata (izleme titreşimi)
  ölçülmelidir.
- **Ölçümleri yalnızca yeni sürümlerle değiştirmek.** Doğrulama yumuşatma ya da
  başka değişiklikler gerektirirse, bunlar yeni bir paradigma sürümü olarak
  gelir; böylece yayımlanmış sonuçlar hesaplandıkları tanımı korur.
