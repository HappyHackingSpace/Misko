---
title: Go backend (önizleme)
description: new-backend branch'inde yeniden yazılan Go backend'in kurulumu ve kullanımı; giriş, kullanıcılar, roller ve laboratuvar ayarları.
---

:::caution
Bu sayfa henüz yayınlanmamış `new-backend` branch'ini anlatır. Go API şu an Vue
panelini beslemiyor; Kurulum ve Kullanım sayfaları hâlâ mevcut sürümü anlatır.
Bu branch'i canlıya almayın.
:::

## Şu an neler var

| Alan | Durum |
|------|-------|
| Sağlık ve hazır olma kontrolleri | Hazır |
| Giriş, mevcut kullanıcı, kendi şifresini değiştirme | Hazır |
| Beş rolle kullanıcı yönetimi | Hazır |
| Laboratuvar ayarları (kurulum başına tek laboratuvar) | Hazır |
| Denekler, deneyler, paradigmalar, video analizi | Henüz yok |
| Yeni API üzerinde Vue paneli | Henüz yok |

## Kurulum

Docker ve Docker Compose gerekir. Komutları `new-backend` branch'inde, reponun
kök dizininde çalıştırın:

```bash
docker compose up -d db
docker compose --profile setup run --rm schema
docker compose --profile setup run --rm setup
docker compose up --build -d backend
curl --fail http://127.0.0.1:4000/api/ready
```

1. `db`, PostgreSQL'i yeni bir volume ile başlatır. Eski kurulumun verisine dokunulmaz.
2. `schema`, boş veritabanında tabloları oluşturur. Tekrar çalıştırılırsa reddeder ve hiçbir şey silmez.
3. `setup`, laboratuvarı ve ilk yöneticiyi oluşturur; yöneticinin e-postasını ve
   üretilen şifreyi terminalinize **yalnızca bir kez** yazar. Tekrar
   çalıştırılırsa hiçbir şey oluşturmaz.
4. `backend`, API'yi `127.0.0.1:4000` adresinde başlatır.

`setup` terminal olmadan çalışırsa (örneğin CI içinde) `-credentials-file YOL`
verilmedikçe başlamayı reddeder. Bu durumda şifre, yalnızca sahibinin
okuyabildiği yeni bir dosyaya yazılır. Şifre hiçbir zaman loglara yazılmaz.

### Ayarlar

Compose dosyasındaki değerler yalnızca geliştirme içindir. Gerçek bir kurulumda
kendi değerlerinizi verin:

| Değişken | Varsayılan | Anlamı |
|----------|------------|--------|
| `JWT_SECRET` | geliştirme değeri | Giriş token'larını imzalayan anahtar. En az 32 rastgele bayt kullanın. |
| `TOKEN_TTL` | `12h` | Bir girişin geçerlilik süresi; 5 dakika ile 7 gün arası. |
| `BCRYPT_COST` | `12` | Şifre hash gücü; 10 ile 14 arası. |
| `ADMIN_EMAIL` | `admin@misko.local` | `setup` ile oluşturulan ilk yöneticinin e-postası. |
| `LAB_NAME` | `Misko Laboratory` | Laboratuvar adı. |
| `LAB_TIMEZONE` | `UTC` | Laboratuvarın saat dilimi; `Europe/Istanbul` gibi bir IANA adı. |

Veritabanı ve zaman aşımı ayarları dahil tüm değişkenler backend README dosyasındadır.

## Kullanım

### Giriş

```bash
curl -X POST http://127.0.0.1:4000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@misko.local","password":"SETUP_CIKTISINDAKI_SIFRE"}'
```

Yanıtta bir `token` bulunur. Diğer tüm isteklerde bunu
`Authorization: Bearer TOKEN` başlığıyla gönderin. `GET /api/auth/me`
kullanıcınızı ve rolünüzün izinlerini döndürür. Üretilen şifreyi hemen
`POST /api/auth/password` (`currentPassword`, `newPassword`) ile değiştirin.
Yanıt yeni bir token içerir ve eski token'ların hepsi geçersiz olur.

Şifreler en az 8 karakter ve en fazla 72 bayt olmalıdır. ş veya ğ gibi İngilizce
dışı harfler birden fazla bayt kaplar.

### Roller

| Rol | Yapabilecekleri |
|-----|-----------------|
| SUPERADMIN, LAB_MANAGER | Kullanıcı ve laboratuvar ayarları yönetimi dahil her şey |
| RESEARCHER | Çalışma, denek, düzenek ve testleri yönetmek; her şeyi okumak |
| TECHNICIAN | Ağırlık kaydetmek ve test çalıştırmak; her şeyi okumak |
| VIEWER | Yalnızca okuma |

Rol değişikliği kullanıcının bir sonraki isteğinde geçerli olur. Kullanıcı
silindiğinde oturumu anında kapanır.

### Kullanıcı yönetimi

Bu uç noktaları yalnızca SUPERADMIN ve LAB_MANAGER kullanabilir:

- `GET /api/users`, kullanıcıları `search`, `role`, `sort`, `order`, `page` ve `pageSize` ile listeler.
- `POST /api/users`, `email`, `name`, `role` ve isteğe bağlı `password` ile
  kullanıcı oluşturur. Şifre verilmezse güçlü bir şifre üretilir ve bir kez döndürülür.
- `PATCH /api/users/{id}`, `name` veya `role` değiştirir.
- `POST /api/users/{id}/reset-password`, yeni şifre belirler (ya da üretir) ve o
  kullanıcının tüm oturumlarını kapatır.
- `DELETE /api/users/{id}`, kullanıcıyı siler.

İki güvenlik kuralı her zaman geçerlidir. Kendi hesabınızı silemezsiniz; son
SUPERADMIN veya LAB_MANAGER silinemez ya da daha düşük bir role alınamaz. Herkese
açık kayıt yoktur.

### Laboratuvar ayarları

Giriş yapmış her kullanıcı `GET /api/lab` ile okuyabilir. SUPERADMIN ve
LAB_MANAGER, `PATCH /api/lab` ile `name`, `code` ve `timezone` alanlarını
değiştirebilir. Boş `code` değeri kodu temizler. `GET /api/meta` herkese açıktır
ve giriş ekranı için laboratuvar adını döndürür.

### Hatalar

Her hata `auth.forbidden` veya `user.lastPrivileged` gibi sabit bir `code` ve
İngilizce bir mesaj içerir. 401 durumu giriş yapmadığınızı ya da token'ınızın
artık geçerli olmadığını gösterir. 403 durumu rolünüzün bu işleme izin
vermediğini gösterir.
