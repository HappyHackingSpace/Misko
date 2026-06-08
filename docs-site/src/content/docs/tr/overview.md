---
title: Genel bakış
description: Mişko ne yapar, kimler için ve temel kavramlar.
---

Mişko, laboratuvar fareleri üzerinde yürütülen davranış testlerini tek bir
yerden yönetir. Denekler, ortamlar, senaryolar ve testlerin tamamı
uygulama üzerinden tanımlanır ve takip edilir.

## Kimler için

Kemirgenler üzerinde davranış paradigmaları yürüten ve **neyin, hangi hayvan
üzerinde, hangi düzenekle test edildiğini ve sonucun ne olduğunu** ham videoyu
elle yönetmeden güvenilir biçimde kaydetmesi gereken biyoloji ve sinirbilim
laboratuvarları için.

## Temel kavramlar

- **Paradigma** — bilimsel bir test türü (Morris Su Tankı, Açık Alan, Yükseltilmiş
  Artı Labirent, Rotarod). Kodda tanımlıdır; katalog sabit ve kararlıdır.
- **Ortam** — bir paradigmanın isimli, somut instance'ı: fiziksel kurulum
  (cm cinsinden geometri, görü kontrastı için yüzey rengi/malzemesi, bölgeler).
- **Denek** — fare; sade tutulur (kod, cinsiyet, grup, doğum tarihi, not).
- **Senaryo** — merkezî nesne. Bir veya birden çok ortamı (paradigmaları
  metrikleri sabitler) paketler; böylece bir deney bir kez tanımlanır.
- **Test** — bir deneğin bir senaryoya göre, ortam ortam koşturulması;
  `PENDING → RUNNING → DONE / FAILED`. Sonuçlar veridir, geç/kal verdikti değil.

## İçeriden SaaS modeli

Mişko **on-prem, kurulum başına bir laboratuvar** olarak dağıtılır. Public kayıt
yoktur: açılışta bir **superadmin oluşturulur** (sistem güçlü bir şifre üretir ve
bir kez yazar), diğer tüm kullanıcılar yönetici tarafından içeriden açılır.
Erişim, kişinin laboratuvardaki rolüne bağlı **rol tabanlı izinlerle** yönetilir.

## Sistem sınırı

Mişko bilinçli olarak görü işi **yapmaz**. System of record'dur ve yalnızca
**özet metrikleri + artefakt URL'lerini** tutar. Ayrı bir CV servisi (Python /
FastAPI / YOLOv8 / ByteTrack, kendi PostgreSQL'i ile) ağır veriyi tutar ve test
bittiğinde özet sonucu geri gönderir. Bkz. [Entegrasyon](../integration/).
