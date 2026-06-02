# Mişko - Proje Kuralları

## Dokümantasyon

- **Her kod/davranış değişikliğinde dokümantasyon da güncellenir.** Bir özellik eklenir, değiştirilir veya kaldırılırsa ilgili dokümantasyon aynı PR içinde güncellenmelidir. Dokümantasyon ikinci sınıf bir iş değildir.
- Dokümantasyon `docs-site/` altında Starlight (Astro) ile tutulur. İçerik `docs-site/src/content/docs/` (İngilizce, kök) ve `docs-site/src/content/docs/tr/` (Türkçe) dizinlerindedir. İki dil de senkron tutulur.
- Sidebar yapısı `docs-site/astro.config.mjs` içinde tanımlıdır; yeni sayfa eklenince sidebar'a da eklenir.
- Dokümantasyon şu bölümleri içermelidir:
  - **Installation / Kurulum**: bağımlılıklar, ortam değişkenleri, `docker-compose` ile ayağa kaldırma adımları.
  - **Kullanım / Usage**: uygulamanın nasıl çalıştırılacağı ve temel akışlar.
- Dokümantasyon GitHub Pages'e `.github/workflows/docs-deploy.yml` ile deploy edilir (yayın: https://happyhackingspace.github.io/Misko/). Yalnızca `docs-site/**` değişikliklerinde tetiklenir.

## Yazım kuralları

- Em dash karakteri ("—") kullanma. Bunun yerine normal tire ("-"), virgül veya iki ayrı cümle kullan. Bu kural hem dokümantasyon hem de bana yazdırılan tüm metinler için geçerlidir.
