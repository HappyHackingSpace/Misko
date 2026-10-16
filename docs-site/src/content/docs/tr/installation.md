---
title: Kurulum
description: Mişko'nun hem Docker ile hem de yerel geliştirme ortamında nasıl kurulup çalıştırılacağı.
---

Bu sayfa Mişko'yu çalıştırmayı anlatır. İster sistemin tamamını ayağa kaldırmak
isteyen bir laboratuvar yöneticisi olun, ister kod üzerinde çalışmak isteyen bir
geliştirici, ihtiyacınız olan adımlar burada.

## Ne kuruyorsunuz

Mişko birlikte çalışan birkaç parçadan oluşur:

- **Veritabanı** (PostgreSQL) tüm laboratuvar kayıtlarını tutar.
- **Backend** (Go API) iş mantığını barındırır ve veritabanıyla konuşur.
- **Jobs**, analiz çalıştırmalarını kuyruklayan ve süresi dolanları yeniden kuyruğa alan küçük bir süreçtir.
- **Frontend** (Vue 3 tek sayfa uygulaması, Nginx ile sunulur) tarayıcıda kullandığınız web panelidir.
- **Worker** (Python) kaydedilen videoları analiz eder. Opsiyoneldir.

Docker ile hepsini birlikte başlatırsınız. Geliştirme için veritabanını Docker'da,
uygulamaları ise kaynaktan çalıştırabilirsiniz.

## Ön gereksinimler

| Araç | Neden gerekli | Not |
|------|---------------|-----|
| Docker + Docker Compose | Tüm yığını çalıştırmak için | Çoğu kullanıcı için önerilen yol. |
| Go 1.27+ ve Node.js 22+ | Yerel geliştirme | Sadece uygulamaları Docker dışında çalıştıracaksanız gerekir. |
| Git | Kaynak kodu almak için | Önce repoyu `git clone` ile indirin. |

Sadece Mişko'yu kullanmak istiyorsanız tek ihtiyacınız Docker'dır.

## Seçenek A: Docker (önerilen)

```bash
git clone https://github.com/HappyHackingSpace/Misko.git
cd Misko

cp .env.example .env                               # opsiyonel: her değerin varsayılanı var
docker compose --profile setup run --rm migrate    # şemayı kurar veya günceller
docker compose --profile setup run --rm setup      # laboratuvarı ve yöneticiyi oluşturur
docker compose up -d                               # API'yi, jobs sürecini ve paneli başlatır
```

Tamamlandığında:

- Web paneli: [http://localhost:8080](http://localhost:8080)
- API: [http://localhost:4000/api](http://localhost:4000/api)

Şema ve yönetici, konteyner açılışında değil, bu iki açık komutla kurulur. Yeni
bir sürüme geçtikten sonra `docker compose up -d` öncesinde `migrate` komutunu
tekrar çalıştırın: veritabanınızda henüz olmayan migration'ları tek bir
transaction içinde uygular, güncel bir veritabanında hiçbir şey yapmaz.

### İlk giriş şifresi

Sabit bir varsayılan şifre ve public kayıt yoktur. `setup` komutu yöneticiyi
`ADMIN_EMAIL` ile oluşturur, güçlü bir şifre üretir ve bunu terminalinize
**bir kez** yazar:

```
Administrator created. This password is shown only once; change it after signing in.
email: admin@misko.local
password: ...
```

Şifre hiçbir log'a yazılmaz. Terminali kapatmadan kopyalayın. Dosya olarak almayı
tercih ederseniz bir klasör bağlayın:

```bash
docker compose --profile setup run --rm -v "$PWD/secrets:/out" setup -credentials-file /out/admin.txt
```

Dosya 0600 izniyle oluşturulur ve komut var olan bir dosyanın üzerine yazmayı
reddeder. Giriş yapın, şifreyi değiştirin, ardından diğer kullanıcıları paneldeki
**Kullanıcılar** ekranından oluşturun. Günlük kullanım için bkz.
[Kullanım](../usage/).

### Video depolama

Varsayılan olarak videolar ve analiz çıktıları bu makinede, `local-videos` Docker
volume'unda tutulur. Backend `LOCAL_STORAGE_DIR=/data/videos` ile çalışır ve
yükleme ile oynatma URL'lerini `/api/local-storage/` altında kendisi sunar; bulut
hesabı gerekmez. URL'ler `JWT_SECRET` ile imzalanır ve bulut depolamadaki gibi
süresi dolar.

Bunun yerine özel bir Google Cloud Storage bucket'ı kullanmak için `GCS_BUCKET`
(ve `GCS_SIGNER_EMAIL`) değerlerini verin. Bucket ayarlıysa yerel klasörün önüne
geçer.

### Analiz worker'ı

Worker opsiyoneldir: o olmadan da testler kaydedilebilir ve gözlemci puanlamalı
paradigmalar (direk, koşu bandı, yeni nesne) panelde puanlanır, ama takip
edilen paradigma analiz edilmez. Worker takip edilen tek paradigmayı, `MWM`
(Morris su labirenti) sürüm 1'i izler. Yalnızca sentetik videoyla test edildi.

1. Laboratuvar yöneticisi ya da yönetici olarak giriş yapın ve worker'ı
   `misko-tracker-bgsub 2.0.0` model sürümüyle ve `MWM` yeteneğini
   sürüm 1 olarak kaydedin. Gözlemci puanlamalı paradigmalar yetenek olamaz.

   ```bash
   TOKEN=$(curl -s http://localhost:4000/api/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"email": "admin@misko.local", "password": "SIFRENIZ"}' | jq -r .token)
   curl -s http://localhost:4000/api/analysis/workers \
     -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{
    "name": "worker-1",
    "modelVersion": "misko-tracker-bgsub 2.0.0",
    "capabilities": [
      {"paradigmKey": "MWM", "paradigmVersion": 1}
    ]
  }'
   ```

2. Yanıt worker token'ını (`mw_...`) yalnızca bir kez gösterir. Bunu `.env`
   içine `MISKO_WORKER_TOKEN` olarak yazın ve worker'ı başlatın:

   ```bash
   docker compose --profile worker up -d worker
   ```

`GET /api/analysis/capabilities` etkin worker'ların neyi analiz edebildiğini
listeler. Worker'ın kendi ayarları ve sınırları `worker/README.md` içindedir.

### Önceki bir sürümden yükseltme

Veritabanı volume'unun adı artık `postgres-data`. Eski `foundation-db` ve `db-data`
volume'ları artık bağlanmaz ve olduğu gibi bırakılır, asla silinmez; bu yüzden
yükseltmeden sonra yığın boş bir veritabanıyla başlar: `migrate` ve `setup`
komutlarını tekrar çalıştırın. Eski bir volume'a artık ihtiyacınız yoksa
`docker volume rm` ile kaldırın (`docker volume ls` hepsini listeler).

### Durdurma ve sıfırlama

```bash
docker compose down            # konteynerleri durdurur, veriyi korur
docker compose down -v         # durdurur VE veritabanı ile video volume'larını siler (tam sıfırlama)
```

`down -v` sonrasında veritabanı ve saklanan videolar silinmiştir; yığını başlatmadan önce `migrate`
ve `setup` komutlarını tekrar çalıştırın.

## Seçenek B: Yerel geliştirme

Sadece veritabanını Docker'da çalıştırın, uygulamaları kaynaktan başlatın.

```bash
docker compose up -d db        # sadece veritabanı
```

**Backend** (bir terminal). Go süreçleri `.env` dosyalarını okumaz, bu yüzden
değişkenleri dışa aktarın. Tam liste `backend/.env.example` içindedir.

```bash
cd backend
export DATABASE_URL='postgres://misko:misko_local_only@127.0.0.1:5432/misko_foundation?sslmode=disable'
export JWT_SECRET="$(openssl rand -base64 48)"
go run ./cmd/bootstrap migrate  # şemayı kurar veya günceller
go run ./cmd/bootstrap setup    # laboratuvarı ve yöneticiyi oluşturur
export LOCAL_STORAGE_DIR="$PWD/.local-storage"   # opsiyonel: videoları bir klasörde tutar
go run ./cmd/api                # API: http://localhost:4000
```

`LOCAL_STORAGE_DIR` ayarlıyken API videoları o klasörde tutar ve panel aynı `/api`
yönlendirmesi üzerinden oraya yükler. Kaynaktan çalışan bir worker
`MISKO_API_URL=http://localhost:4000` ile dosyalara aynı yoldan ulaşır.

**Frontend** (ikinci terminal):

```bash
cd frontend
npm install
npm run dev                     # panel: http://localhost:5173
```

Geliştirmede frontend, `/api` çağrılarını 4000 portundaki backend'e yönlendirir;
paneli [http://localhost:5173](http://localhost:5173) adresinden kullanırsınız.

## Ortam değişkenleri

`docker-compose.yml` bunları `.env` dosyasından okur. Hepsinin bir varsayılanı
vardır ve yerel veritabanı kimlik bilgileri compose dosyasında sabittir, çünkü o
yığın yalnızca geliştirme içindir.

| Değişken | Varsayılan | İşlevi |
|----------|------------|--------|
| `JWT_SECRET` | yerel geliştirme değeri | Giriş tokenlarını imzalar. Kendi makineniz dışındaki her yerde en az 32 rastgele bayt kullanın. |
| `ADMIN_EMAIL` | `admin@misko.local` | `setup` komutunun oluşturduğu yöneticinin e-postası. |
| `LAB_NAME` | `Misko Laboratory` | Panelde gösterilen laboratuvar adı. |
| `LAB_TIMEZONE` | `UTC` | Laboratuvar kayıtlarının kullandığı saat dilimi. |
| `FRONTEND_PORT` | `8080` | Web panelinin host portu. |
| `JOB_INTERVAL` | `10s` | Jobs sürecinin analiz çalıştırmalarını kuyruklama sıklığı. |
| `GCS_BUCKET` | boş | Videolar için özel bucket. Ayarlıysa yerel depolama yerine kullanılır. |
| `GCS_SIGNER_EMAIL` | boş | Yükleme ve okuma URL'lerini imzalayan servis hesabı. |
| `MISKO_WORKER_TOKEN` | boş | Yalnızca `worker` profili için, API'de bir worker kaydettikten sonra. |

`docker-compose.yml` içinde backend ayrıca `local-videos` volume'unda
`LOCAL_STORAGE_DIR=/data/videos` ile çalışır. Docker dışında videoları bir klasörde
tutmak için `LOCAL_STORAGE_DIR` değerini verin; o da `GCS_BUCKET` da yoksa video
uçları 503 döner.

Docker dışında çalışan backend için `TOKEN_TTL`, `BCRYPT_COST`, `UPLOAD_URL_TTL`
ve `READ_URL_TTL` dahil tam liste `backend/.env.example` içindedir.

:::caution
Gerçek bir dağıtımdan önce güçlü bir `JWT_SECRET` ayarlayın, gerçek veritabanı
kimlik bilgileri kullanın ve paneli HTTPS üzerinden sunun. Hazır imajlarla
dağıtım
[DEPLOY.md](https://github.com/HappyHackingSpace/Misko/blob/main/docs/DEPLOY.md)
içinde anlatılır.
:::

## Kurulumu doğrulama

1. API'yi kontrol edin: [http://localhost:4000/api/health](http://localhost:4000/api/health) başarılı yanıt döner; `/api/ready` ayrıca veritabanını da yoklar.
2. Paneli açın ve yönetici e-postası ile `setup` komutunun yazdığı şifreyle giriş yapın.
3. Deneyler ekranı açılıyorsa kurulum sağlıklıdır.

## Sorun giderme

- **Port kullanımda**: `.env` içinde `FRONTEND_PORT` değerini değiştirin ya da 4000 portunu tutan süreci durdurup tekrar `docker compose up -d` çalıştırın.
- **Giriş yapamıyorum, şifre kayboldu**: şifre bir kez gösterilir ve geri alınamaz. `docker compose down -v` ile sıfırlayıp `migrate` ve `setup` komutlarını tekrar çalıştırın.
- **`setup` çalışmayı reddediyor**: şifreyi yazmak için bir terminale ihtiyacı var. Betik içinde çalıştırıyorsanız, henüz var olmayan bir yol ile `-credentials-file` verin.
- **Video yükleme 503 dönüyor**: backend'de ne `LOCAL_STORAGE_DIR` ne de `GCS_BUCKET` ayarlı.
- **Yükseltmeden sonra veritabanı boş**: yığın artık `postgres-data` volume'unu kullanır. `migrate` ve `setup` çalıştırın; verisine ihtiyacınız olursa eski volume hâlâ yerindedir.
- **Backend veritabanına ulaşamıyor**: `db` konteynerinin sağlıklı olduğundan emin olun (`docker compose ps`).
