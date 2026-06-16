---
title: Kurulum
description: Mişko'nun hem Docker ile hem de yerel geliştirme ortamında nasıl kurulup çalıştırılacağı.
---

Bu sayfa Mişko'yu çalıştırmayı anlatır. İster sistemin tamamını ayağa kaldırmak
isteyen bir laboratuvar yöneticisi olun, ister kod üzerinde çalışmak isteyen bir
geliştirici, ihtiyacınız olan adımlar burada.

## Ne kuruyorsunuz

Mişko birlikte çalışan üç parçadan oluşur:

- **Veritabanı** (PostgreSQL) tüm laboratuvar kayıtlarını tutar.
- **Backend** (Node.js / Express API) iş mantığını barındırır ve veritabanıyla konuşur.
- **Frontend** (Vue 3 tek sayfa uygulaması, Nginx ile sunulur) tarayıcıda kullandığınız web panelidir.

Docker ile üçünü tek komutla başlatırsınız. Geliştirme için veritabanını
Docker'da, backend ve frontend'i ise doğrudan makinenizde çalıştırabilirsiniz.

## Ön gereksinimler

| Araç | Neden gerekli | Not |
|------|---------------|-----|
| Docker + Docker Compose | Tüm yığını çalıştırmak için | Çoğu kullanıcı için önerilen yol. |
| Node.js 20+ | Backend/frontend yerel geliştirme | Sadece uygulamaları Docker dışında çalıştıracaksanız gerekir. |
| Git | Kaynak kodu almak için | Önce repoyu `git clone` ile indirin. |

Sadece Mişko'yu kullanmak istiyorsanız tek ihtiyacınız Docker'dır.

## Seçenek A: Docker (önerilen)

Çalışan bir sisteme ulaşmanın en hızlı yolu. PostgreSQL, API ve web panelini
birlikte başlatır.

```bash
git clone https://github.com/HappyHackingSpace/Misko.git
cd Misko

cp .env.example .env       # isterseniz değerleri düzenleyin (aşağıdaki tabloya bakın)
docker compose up -d --build
```

Tamamlandığında:

- Web paneli: [http://localhost:8080](http://localhost:8080)
- API: [http://localhost:4000/api](http://localhost:4000/api)

Veritabanı migrasyonları açılışta otomatik uygulanır, elle bir şey yapmanız
gerekmez.

### İlk giriş şifresini alın

Sabit bir varsayılan şifre ve public kayıt yoktur. İlk açılışta sistem,
`ADMIN_EMAIL` ile bir **superadmin** oluşturur ve güçlü bir şifre üretir; bu
şifre backend log'una **bir kez** yazılır:

```bash
docker compose logs backend | grep -A6 SUPERADMIN
```

Bu şifreyi kopyalayın, giriş yapın, şifreyi değiştirin ve ardından diğer
kullanıcıları paneldeki **Kullanıcılar** ekranından oluşturun. Günlük kullanım
için bkz. [Kullanım](../usage/).

### Örnek veri yükleyin (opsiyonel)

Birkaç örnek senaryo ve örnek bir denek eklemek için:

```bash
docker compose exec backend node prisma/seed.js
```

### Durdurma ve sıfırlama

```bash
docker compose down            # konteynerleri durdurur, veriyi korur
docker compose down -v         # durdurur VE veritabanı volume'unu siler (tam sıfırlama)
```

## Seçenek B: Yerel geliştirme

Sadece veritabanını Docker'da çalıştırın; backend ve frontend'i canlı yeniden
yükleme ile doğrudan çalıştırarak kodu düzenleyin.

```bash
docker compose up -d db        # sadece veritabanı
```

**Backend** (bir terminal):

```bash
cd backend
cp .env.example .env
npm install
npm run db:migrate             # veritabanı şemasını uygular
npm run db:bootstrap           # superadmin oluşturur (şifreyi yazar)
npm run db:seed                # örnek veri (opsiyonel)
npm run dev                    # API: http://localhost:4000
```

**Frontend** (ikinci terminal):

```bash
cd frontend
npm install
npm run dev                    # panel: http://localhost:5173
```

Geliştirmede frontend, `/api` çağrılarını 4000 portundaki backend'e yönlendirir;
paneli [http://localhost:5173](http://localhost:5173) adresinden kullanırsınız.

## Ortam değişkenleri

Bunlar Docker Compose tarafından `.env` dosyasından okunur. Yerel kullanım için
makul varsayılanlar vardır, ancak gerçek bir dağıtımdan önce gizli değerleri
değiştirmelisiniz.

| Değişken | Varsayılan | İşlevi |
|----------|------------|--------|
| `POSTGRES_USER` | `misko` | Veritabanı kullanıcısı. |
| `POSTGRES_PASSWORD` | `misko` | Veritabanı şifresi. Üretimde değiştirin. |
| `POSTGRES_DB` | `misko` | Veritabanı adı. |
| `DB_PORT` | `5432` | PostgreSQL host portu. |
| `BACKEND_PORT` | `4000` | API host portu. |
| `JWT_SECRET` | `change-me-in-production` | Giriş tokenlarını imzalayan gizli anahtar. Üretimde **mutlaka** değiştirin. |
| `JWT_TTL` | `7d` | Giriş oturumunun geçerlilik süresi. |
| `CORS_ORIGIN` | `*` | API'yi hangi web origin'lerin çağırabileceği. Üretimde kısıtlayın. |
| `ADMIN_EMAIL` | `admin@miskolab.com` | İlk açılışta oluşturulan superadmin e-postası. |
| `LAB_NAME` | `Mişko Laboratuvarı` | Panelde markalama için gösterilen laboratuvar adı. |
| `FRONTEND_PORT` | `8080` | Web paneli host portu. |

:::caution
Gerçek bir dağıtımdan önce güçlü bir `JWT_SECRET`, gerçek bir
`POSTGRES_PASSWORD` ayarlayın ve `CORS_ORIGIN` değerini panel adresinizle
sınırlayın.
:::

## Kurulumu doğrulama

1. API sağlık kontrolünü açın: [http://localhost:4000/api/health](http://localhost:4000/api/health). Başarılı bir yanıt dönmelidir.
2. Frontend adresinden paneli açın ve log'daki superadmin şifresiyle giriş yapın.
3. Giriş çalışıyor ve panel açılıyorsa kurulum sağlıklıdır.

## Sorun giderme

- **Port kullanımda**: `.env` içinde `FRONTEND_PORT`, `BACKEND_PORT` veya `DB_PORT` değerini değiştirip tekrar `docker compose up -d` çalıştırın.
- **Şifreyi bulamıyorum**: log grep komutunu tekrar çalıştırın ya da temiz bir başlangıç için `docker compose down -v` yapıp yığını yeniden ayağa kaldırın; superadmin yeniden oluşturulur.
- **Backend veritabanına ulaşamıyor**: `db` konteynerinin sağlıklı olduğundan emin olun (`docker compose ps`). Bozuk bir volume soruna yol açabilir; bu durumda `docker compose down -v` ile sıfırlayın.
