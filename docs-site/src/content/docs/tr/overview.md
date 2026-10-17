---
title: Genel bakış
description: Mişko ne yapar, kimler için ve temel kavramlar.
---

Mişko, laboratuvar kemirgenleri üzerindeki preklinik davranış çalışmalarını tek
bir yerden yönetir: çalışma tasarımı, denekler ve aldıkları, kaydedilen test
oturumları, video analizi ve onaylanmış sonuçlar.

## Kimler için

Bir maddenin hastalık modelindeki bozukluğu geri çevirip çevirmediğini davranış
paradigmalarıyla test eden ve **hangi deneğin hangi grupta olduğunu, ne
aldığını, nasıl test edildiğini ve sonucun ne olduğunu** ham videoyu elle
yönetmeden güvenilir biçimde kaydetmesi gereken biyoloji ve sinirbilim
laboratuvarları için.

## Temel kavramlar

- **Etken maddeler ve hastalık modelleri**: maddeler (bileşikler, referans
  ilaçlar, taşıyıcılar, indükleyici ajanlar) ve hastalık modelleri; laboratuvarın
  tüm çalışmalarında ortaktır. Panelde her birinin kendi sayfası vardır.
- **Paradigma**: bilimsel bir test türü (Morris su labirenti, yeni nesne
  tanıma, direk testi ve koşu bandı). Kodda, hiç değişmeyen
  yayınlanmış sürümler halinde tanımlıdır.
- **Ortam**: tek bir paradigmanın ölçüm revizyonlu fiziksel düzeneği. Bir testte
  denek bir arenaya konur; arena bir ortam revizyonudur.
- **Denek**: denek (kod, tür, cinsiyet) ve vücut ağırlıkları.
- **Deney**: çalışmanın kendisi. Taslak olarak tasarlanır (gruplar, rejimler,
  zaman noktaları, protokoller, sonlanım noktaları), gün 0'da başlatılır ve
  tamamlanır.
- **Grup**: rolü (sağlıklı kontrol, sham, hastalık kontrolü, pozitif kontrol veya
  tedavi), hastalık modeli ve rejimi olan bir çalışma kolu. Bir denek tam olarak
  bir gruba atanır ve bir gerekçeyle dışlanabilir.
- **Test**: bir zaman noktasında, bir protokol adımında, tek videolu ve her biri
  kendi arenasında olan bir veya daha fazla denekle kaydedilen tek bir oturum.
- **Sonuç**: bir deneğin bir testteki analizi. Her sonuç sayılmadan önce bir
  kişi tarafından kabul veya reddedilir. Sonuçlar veridir, geçti/kaldı verdikti
  değil.
- **Rapor**: sonlanım noktası başına grup istatistikleri ve grup rollerinden
  çıkan karşılaştırmalar (model geçerliliği, test duyarlılığı, kurtarma
  yüzdesiyle etkinlik, doz yanıtı).

## İçeriden SaaS modeli

Mişko **on-prem, kurulum başına bir laboratuvar** olarak dağıtılır. Public kayıt
yoktur: **superadmin kurulum komutuyla oluşturulur** (sistem güçlü bir şifre
üretir ve bir kez yazar), diğer tüm kullanıcılar yönetici tarafından içeriden
açılır. Erişim, kişinin laboratuvardaki rolüne bağlı **rol tabanlı izinlerle**
yönetilir.

## Sistem sınırı

API görüntü işlemeyi kendisi yapmaz. Ayrı analiz worker'ları bir testin bir
deneğini üstlenir, onu videoda izler ve bir yörünge ile işaretlenmiş bir videoyu
nesne depolamasına yükler. API yüklemeleri doğrular ve her metriği yörüngeden
hesaplar; böylece her ölçümün tek bir tanımı olur. Video dosyaları nesne
depolamasında kalır; veritabanı kayıtları, metrikleri, olayları ve nesne
referanslarını tutar. Bkz. [Mimari](../architecture/).
