---
title: Genel bakış
description: Mişko ne yapar, kimler için ve temel kavramlar.
---

Mişko, laboratuvar fareleri üzerinde yürütülen davranış testlerini tek bir
yerden yönetir. Çalışmalar, denekler, cihazlar ve testlerin tamamı uygulama
üzerinden tanımlanır ve takip edilir.

## Kimler için

Kemirgenler üzerinde davranış paradigmaları yürüten ve **neyin, hangi hayvan
üzerinde, hangi düzenekle test edildiğini ve sonucun ne olduğunu** ham videoyu
elle yönetmeden güvenilir biçimde kaydetmesi gereken biyoloji ve sinirbilim
laboratuvarları için.

## Temel kavramlar

- **Paradigma** — bilimsel bir test türü (Morris Su Tankı, Açık Alan, Yükseltilmiş
  Artı Labirent, Rotarod). Kodda tanımlıdır; katalog sabit ve kararlıdır.
- **Cihaz (Apparatus)** — bir paradigmayı somutlaştıran fiziksel düzenek
  (cm cinsinden geometri, görü kontrastı için yüzey rengi/malzemesi, bölgeler).
- **Denek** — fare; araştırma seviyesi biyolojik profil (suş, hat/genotip,
  zigotluk, cinsiyet, tüy rengi) ve ağırlık zaman serisi ile.
- **Çalışma → Grup** — karşılaştırma kollu (kontrol, model, tedavi) boylamsal
  deneyler; aynı hayvanın tekrarlı testleri karşılaştırılabilir olur.
- **Test** — merkezî işlem: paradigma + cihaz + denek + operatör + cihaz;
  `PENDING → RUNNING → DONE / FAILED` akışı.

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
