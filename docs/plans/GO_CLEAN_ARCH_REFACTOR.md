# Mişko — Go Clean Architecture refactor planı

Tarih: 2026-09-10. Son karar: sıfırdan Go backend, GCP Cloud Storage, her adımla birlikte gerekli testler. Durum: uygulama öncesi plan; issue açılması implementasyonun tamamlandığı anlamına gelmez.
Bu plan, kullanıcının yeni deney/grup/video iş akışı ve Go Clean Architecture kararını kaydeder. Eski ROADMAP.md içindeki dar domain kapsamı, scenario merkezli hedef ve canlı kamera önceliği yeni hedef için geçerli değildir. Eski dokümanlardaki “done” etiketleri uygulama kanıtı sayılmaz.

## 1. Sabit kararlar

- Vue UI ve Go backend ayrı uygulamalar olacak. CV worker Python olarak ayrı kalacak.
- Go backend domainlere ayrılmış modüler monolit olacak. Clean Architecture bağımlılık yönü CI ile denetlenecek.
- Domain ve application katmanları HTTP, SQL sürücüsü, dosya deposu, JWT kütüphanesi veya CV implementasyonu bilmeyecek.
- Paradigma, metrik, birim, event ve hesaplama tanımları Go kodunda hardcode olacak; düzenlenebilir DB kataloğu, admin CRUD veya runtime script olmayacak.
- Kullanıcının seçtiği izinli fiziksel değerler, protokol parametreleri ve sürüm referansları DB'de tutulacak. Hardcode tanım, her tankın aynı ölçüde olması demek değildir.
- Mevcut beş rol, permission matrisi ve kaynak sahipliği kuralları korunacak. Yeni domain yetkileri ayrıca açıkça tanımlanacak.
- Denek kimliği, deney katılımı, grup üyeliği, aşama, planlanan uygulama ve gerçekleşen uygulama birbirinden ayrılacak.
- Test ve analiz farklı yaşam döngüleridir. Bir videonun yeniden analizi yeni test veya yeni denek sayılmaz.
- Tamamlanmış analizin girdileri ve çıktıları değiştirilmeyecek; düzeltme yeni analiz/sürüm üretecek.
- Tek laboratuvar / kurulum yaklaşımı korunacak.
- Eski veri migration/backfill, eski API/token/hash uyumluluğu, dual-write ve legacy adapter yok. Yeni domain modeli doğrudan kurulacak. RBAC ve bilimsel doğruluk korunur; eski implementasyon hataları kopyalanmaz.
- GCP Cloud Storage orijinal ve analiz edilmiş videoları saklar. Bir orijinal video farklı AnalysisRun kayıtlarının çıktılarıyla ayrı ayrı eşleşebilir.
- Her implementasyon PR’ı gerekli davranış testlerini içerir: önce başarısız davranış testi, sonra implementasyon, sonra sadeleştirme.
- Entegrasyon branch’i tam olarak `new-backend`; çalışma branch’leri `codex/...`, tüm adım PR’larının base’i `new-backend`. Tamamlanmadan main merge/release/deploy yok.

## 2. Mevcut durumun kanıtları

| Kaynak | Doğrulanan davranış |
|---|---|
| backend/prisma/schema.prisma | Subject.groupName serbest metin; Test subject+scenario; sonuç ortam bazlı JSON |
| backend/src/modules/tests/test.service.js | Eventlerden metrik hesaplama; aynı sonuç üzerinde güncelleme; test listesinde sınırlı filtre |
| backend/src/config/{paradigms,metrics,units,metricEngine}.js | 11 kod tanımlı paradigma ve bilimsel sözleşme |
| backend/src/config/permissions.js | Beş rol ve statik permission matrisi |
| backend/src/middleware/authenticate.js | JWT doğrulamasından sonra kullanıcı/rol DB'den yükleniyor |
| backend/src/modules/users/user.service.js | Son yetkili kullanıcı koruması Serializable transaction içinde; kendini silmek yasak |
| backend/src/modules/comments/comment.service.js | Her oturumlu kullanıcı yorum oluşturur; yalnız yazar düzenler; yazar/yönetici siler; test-comment kapsam kontrolü |
| backend/src/modules/environments/environment.service.js | Ortam konfigürasyonu güncellenebilir; ayrı tarihsel sürüm yok |
| frontend/src/views/TestNew.vue | Mevcut form scenarioId ve subjectId kullanıyor |

## 3. Hedef paket yapısı ve bağımlılık kuralları

Yeni backend doğrudan backend/ altında kurulacak; eski JavaScript backend bu branch üzerinde kaldırılabilir. Legacy paralel çalışma katmanı kurulmaz. Eski veritabanı veya GCP kaynakları bu plan kapsamında silinmez.

```text
backend/
  cmd/api/main.go
  cmd/jobs/main.go
  cmd/bootstrap/main.go
  internal/bootstrap/                 # constructor wiring / composition root
  internal/identity/{domain,application,adapters}/
  internal/laboratory/{domain,application,adapters}/
  internal/subjects/{domain,application,adapters}/
  internal/experiments/{domain,application,adapters}/
  internal/interventions/{domain,application,adapters}/
  internal/paradigms/{domain,application,adapters}/
  internal/environments/{domain,application,adapters}/
  internal/protocols/{domain,application,adapters}/
  internal/tests/{domain,application,adapters}/
  internal/media/{domain,application,adapters}/
  internal/analysis/{domain,application,adapters}/
  internal/reporting/{application,adapters}/
  internal/platform/                  # altyapı kurulumu; iş kuralı içermez
  api/openapi.yaml
  schema/                            # temiz kurulum SQL şeması
  tests/contract/
  tests/integration/
```

Her domainin adapters/ altında gerektiği kadar http/, postgres/, worker/ bulunur. Boş katman ve her struct için interface oluşturulmaz.

| Katman | Kural |
|---|---|
| domain | Entity, value object, invariant ve saf hesaplama; yalnız gereken standart kütüphane |
| application | Use case, transaction/clock/repository gibi tüketiciye ait küçük portlar; domain'i kullanır |
| adapters | Port implementasyonları; HTTP DTO ve sqlc modellerini domain tiplerine dönüştürür |
| bootstrap | Gerçek bağımlılıkları constructor ile bağlar; global servis locator yok |
| reporting | Salt okunur sorgu portları ve DTO'lar; domain tablosuna doğrudan mutation yok |

Domainler birbirlerinin adapter/repository implementasyonunu import etmez. Gerekli etkileşim application portu ve açık DTO üzerinden, composition root'ta bağlanan adapter ile yapılır. İşlem bütünlüğü gereken kayıtlar ortak PostgreSQL transaction'ı içinde application koordinasyonuyla yazılır; eventual consistency ile zorunlu invariant ertelenmez. İlişkiler gerçek FK/unique/check constraint'lerle korunur.

Reporting adapter'ı belgelenmiş view veya salt okunur join kullanabilir; her domain için ayrı DB gerekmiyor. Genel BaseRepository, kalıtım benzeri katmanlar ve iş kurallarıyla dolu shared/utils paketi oluşturulmaz. CI import denetimi domain -> adapter/application ve application -> adapter bağımlılıklarını, ayrıca domainler arası adapter erişimini reddeder.

## 4. Go ve bağımlılık politikası

Uygulama başlangıcında en güncel kararlı ve uyumlu sürümler resmi kaynaklardan yeniden çözümlenir. Bu plan sürüm kilit dosyası değildir; doğrulanmamış patch numarası yazılmaz. RC/beta ve CI sırasında kayan @latest kullanılmaz. Seçilen tam sürüm, doğrulama tarihi, Go uyumluluğu ve kaynak URL'si dependency manifestine yazılır; go.mod/go.sum ve araç/container sürümleri sabitlenir.

| İhtiyaç | Tercih | Gerekçe / sınır |
|---|---|---|
| Runtime | Güncel kararlı Go | Resmi release ve download kaynağından doğrulanır |
| HTTP | net/http + ServeMux | İlk tercih standart kütüphane; ihtiyaç kanıtlanmadan framework eklenmez |
| PostgreSQL | github.com/jackc/pgx/v5 + pgxpool | Driver yalnız persistence adapter'ında |
| SQL üretimi | sqlc | Parametreli SQL, tipli adapter kodu; üretilen tipler domain'e sızmaz |
| Şema | Sürümlü temiz-kurulum SQL dosyaları | Eski veri migration aracı veya Prisma geçişi yok |
| JWT | github.com/golang-jwt/jwt/v5 | Yeni auth sözleşmesi; algoritma açık allowlist |
| Parola | golang.org/x/crypto/bcrypt | Yeni hesaplar; byte/Unicode/uzunluk sınırları açıkça test edilir |
| Log / test / hata | log/slog, testing, httptest, errors | Yapılandırılmış log; standart test araçları |
| Analiz kuyruğu | PostgreSQL job/outbox + lease | İlk aşamada ek broker zorunlu değil; timeout, retry ve sahiplik açık |
| Dosya deposu | cloud.google.com/go/storage | Native GCS adapter; ADC/workload identity; domain GCP SDK bilmez |

Standartlar: gofmt, go vet, anlamlı kapsamda go test -race, govulncheck; golangci-lint ve gerekiyorsa sqlc araç sürümleri sabitlenir. Context ilk parametre; request context global saklanmaz. Açık timeout, iptal, graceful shutdown ve sınırlı goroutine/worker sayısı kullanılır. Hatalar wrap edilir ve errors.Is/As ile eşlenir; adapter hatası domain hatasına çevrilir. UTC timestamp + laboratuvar timezone sunumu; süre/birim semantiği açık; liste sorguları parametreli, filtre/sıralama alanları allowlist ve kararlı tie-breaker taşır. Birden çok kaydı değiştiren işlemler transaction içinde yürür; serializable retry sınırlı ve idempotent olur. İstemciye SQL, stack, parola veya token dökülmez.

Resmi kaynaklar (2026-09-10 plan araştırması; tam sürümler uygulamada tekrar doğrulanacak):
- https://go.dev/doc/devel/release
- https://go.dev/dl/?mode=json
- https://go.dev/doc/modules/layout
- https://github.com/jackc/pgx
- https://docs.sqlc.dev/
- https://pkg.go.dev/github.com/golang-jwt/jwt/v5
- https://docs.cloud.google.com/go/docs/reference/cloud.google.com/go/storage/latest
- https://docs.cloud.google.com/storage/docs/resumable-uploads
- https://docs.cloud.google.com/storage/docs/request-preconditions
- https://docs.cloud.google.com/storage/docs/access-control/signed-urls

## 5. RBAC koruma sözleşmesi

| Permission | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|---|---|---|---|---|
| user:manage | Evet | Evet | Hayır | Hayır | Hayır |
| lab:configure | Evet | Evet | Hayır | Hayır | Hayır |
| study:write | Evet | Evet | Evet | Hayır | Hayır |
| subject:write | Evet | Evet | Evet | Hayır | Hayır |
| weight:write | Evet | Evet | Evet | Evet | Hayır |
| apparatus:write | Evet | Evet | Evet | Hayır | Hayır |
| test:write | Evet | Evet | Evet | Hayır | Hayır |
| test:run | Evet | Evet | Evet | Evet | Hayır |
| *:read | Evet | Evet | Evet | Evet | Evet |

Bu permission tablosu tüm GET endpoint'lerinin herkese açık olduğu anlamına gelmez: mevcut /users okumaları da user:manage ister. Route matrisi fiili davranıştan çıkarılır.

Korunacak özel kurallar:
- JWT sub üzerinden mevcut kullanıcı yüklenir; token içindeki eski role güvenilmez. Rol değişikliği sonraki isteğe yansır; silinen kullanıcı reddedilir.
- Yeni login/me ve token sözleşmesi tanımlanır; Bearer, expiry, issuer/audience, algoritma allowlist ve 401/403 testleri yazılır. Eski token/hash/API uyumluluğu kapsam dışıdır; rol ve kaynak yetkileri korunur.
- Son SUPERADMIN/LAB_MANAGER silinemez veya yetkisiz role indirilemez; eşzamanlı işlem testi gerekir. Kullanıcı kendini silemez.
- Public signup açılmaz; bootstrap tekrarı ikinci laboratuvar/admin üretmez.
- VIEWER dahil herkes yorum oluşturabilir. Yalnız yazar düzenler; yazar veya iki yönetici rolü siler. Başka testin comment ID'si kabul edilmez. Rate limit ve düz metin sözleşmesi korunur.
- UI gizleme güvenlik sınırı değildir. Application use case actor+permission+resource kontrolü yapar; HTTP ve job adapter'ı bunu atlayamaz. Bilinmeyen rol/permission reddedilir.
- Worker kimliği kullanıcı rolünden ayrıdır; yalnız yetkili olduğu analysis run için sonuç teslim eder.

Yeni domain önerisi: deney/grup/katılım ve müdahale planı yönetimi study:write; paradigma salt okunur; ortam/protokol apparatus:write; test oluşturma test:write; var olan teste video yükleme, gerçek uygulama kaydı, analiz başlatma test:run. Her yeni endpoint için olumlu/olumsuz rol matrisi ilgili domain PR’ında tamamlanır. Permission anlamları yeniden yazım sırasında korunur.

## 6. Paradigmaların Go'ya taşınması

Mevcut 11 anahtar korunur: MWM, OPEN_FIELD, EPM, ROTAROD, Y_MAZE, NOVEL_OBJECT, BARNES_MAZE, THREE_CHAMBER, LIGHT_DARK, POLE, TREADMILL.

- internal/paradigms/domain altında tipli tanımlar ve paradigma başına dosya; ortak metric/unit/event tipleri. Registry salt okunur API sağlar, mutable map/slice dışarı sızdırmaz.
- Kimlik, kategori, türler, trial tipleri, parametre aralıkları/default'lar, zone hesapları, metric/event/detect/QC sözleşmesi ve artifact gereksinimleri taşınır.
- ParadigmVersion, MetricEngineVersion ve ResultSchemaVersion ayrı tutulur. Eski sürümler replay için erişilebilir kalır; değişiklik eskisini yeniden yorumlamaz.
- Go katalog ve metrik semantiğinin tek sahibi olur. Python worker sürümlü manifest ve iş girdisini tüketir; ayrı hardcode bilimsel katalog oluşturmaz.
- CV pose/trajectory ve sürümlü sinyal özetleri üretir. Go metrik motoru canonical sonuçları üretir/doğrular. CV'ye özgü hesapların sahibi ve algorithmVersion açıkça tanımlanır; aynı formül iki dilde bağımsız gelişmez.
- Önce OPEN_FIELD pilotu, ardından kalan 10 paradigma. Registry listesinde bulunmak otomatik analiz desteği anlamına gelmez; worker capability/support durumu ayrıca sunulur.
- Her paradigma için bağımsız elle hesaplanan örnekler (JS yalnız incelenecek referanstır, parity zorunluluğu yok): boş olay, eksik CV verisi, sıfır payda, tekrar event, süre sınırı, NaN/Inf, geçersiz key, zone ve birim kontrolü. Tolerans metrik bazında belgelenir; eksik değer 0 yapılmaz.
- Bilimsel mantık hatası yeni implementasyonda korunmaz; beklenen formül, sürüm ve bağımsız doğrulama örneği belgelenir.

## 7. Adım adım uygulama sırası

Issue ve PR listesi: [ISSUES.md](ISSUES.md). Her issue kapsam, bağımlılık, gerekli testler ve kabul koşulu taşır. Gerekli davranış testi issue'nun implementasyonuyla yazılır; testler sona bırakılmaz. Sırf coverage yükseltmek için getter/constructor, framework ve mock çağrı sayısı testleri yazılmaz.

| Adım | Kapsam | Gerekli doğrulama |
|---|---|---|
| 01 | Go iskeleti, CI ve temiz DB şeması altyapısı | Katman import sınırı, config hatası, readiness ve shutdown |
| 02 | Kimlik, laboratuvar, RBAC | Rol × işlem, güncel rol, son yönetici concurrency, bootstrap |
| 03 | Denek, deney, aşama, grup ve katılım | Yanlış deney/grup bağlamı, çakışan üyelik, bağımsız sorgu |
| 04 | Hastalık, madde ve uygulama | Plan/gerçek ayrımı, doz/birim, zaman ve kaynak yetkisi |
| 05 | Open Field kod kataloğu ve metrik motoru | Elle hesaplı sayısal örnekler; eksik/sıfır/geçersiz veri |
| 06 | Diğer 10 kod paradigması | Her yöntem için formül, birim, zone/event/QC sınırları |
| 07 | Ortam sürümleri ve protokol | Sürüm değişmezliği, paradigma uyumu, adım sırası |
| 08 | Test, trial ve yorum | Bağlam, durum geçişi, tekrar, yorum sahipliği ve RBAC |
| 09 | GCS medya ve video pair sözleşmesi | Upload/finalize, generation/checksum, yetkisiz okuma/yazma |
| 10 | Kalibrasyon | FIT/CHECK ayrımı, kamera/ortam/video uyumu, hata toleransı |
| 11 | Analiz işi, sonuç ve event interval modeli | Retry/crash/late delivery, atomik sonuç, zaman ekseni |
| 12 | Gerçek CV ve işaretlenmiş video çıktısı | Elle işaretli tek denek Open Field videosu, pair ve QC |
| 13 | Vue iş akışı ve eşlenik video/event paneli | Upload→analiz→interval click→seek, yetki ve URL yenileme |
| 14 | Domain sorguları ve raporlar | Filtre, birim/sürüm/QC, analiz/denek çift saymama |
| 15 | Tam sistem kabulü ve yayın hazırlığı | Temiz kurulum, tüm roller, gerçek GCS/pair E2E, secret taraması |

## 8. Uygulama ve teslim kuralları

- Eski verinin taşınması, eski ID korunması, API parity, token geçişi, expand/backfill/contract ve legacy rollback işleri tamamen kaldırıldı.
- Temiz veritabanını yeniden üretilebilir SQL ile kurmak veri migration'ı değildir; yeni sistem için gereklidir. Mevcut gerçek DB silinmez; geliştirme/test ayrı boş DB kullanır.
- Her issue için küçük PR, açık test kanıtı ve `new-backend` base kontrolü gerekir. Otomatik merge açılmaz. Non-default branch'e merge issue'yu otomatik kapatmayabilir; tamamlanma ayrıca doğrulanır.
- Güncel kararlı bağımlılıklar ilgili adımda resmi kaynaktan çözülür ve sabitlenir; CI sırasında kayan latest yok.
- PostgreSQL davranışı gerçek disposable PostgreSQL ile; domain kuralları saf unit test ile; UI kritik akışları browser E2E ile doğrulanır. GCS fake testleri gerçek GCS entegrasyonunun kanıtı sayılmaz.
- GCP project/bucket/region/service-account değerleri config girdisidir; canlı doğrulama aşamasında gerçek değerler gerekir. Bu plan/issue turu cloud resource oluşturmaz veya deploy yapmaz.
- Tamamlanma tüm issue kabul kriterleriyle ölçülür. Katalogdaki yöntemlerin CV destek kapsamı görünürdür; desteklenmeyen davranışlar sessiz sonuç üretmez. İlk gerçek pipeline tek denek Open Field; diğer yöntemlerin worker desteği ürün kapsamı olarak açık işlenir.

## 9. Analiz ve domain invariant'ları

- Enrollment deneği deneye bağlar; GroupAssignment aynı deneyin grubunu referanslar. Aynı anda tek aktif kol varsayımı ilk sürümde constraint/use case ile korunur; crossover geçmiş atamayla modellenir.
- Test, aşama, grup ataması ve protokol aynı deney bağlamıyla uyumlu olmalıdır. Aynı deneğin başlangıçta sağlıklı olması grup değiştirmez.
- Grup planı gerçek uygulama yerine geçmez. Gerçek uygulama miktar+birim+yol+zaman; gerektiğinde kullanılan ağırlık ölçümü referansı taşır.
- AnalysisRun girdileri: kayıt/video checksum, zaman aralığı, test/trial, paradigma sürümü, protokol ve ortam sürümü, kalibrasyon, worker/model/engine sürümü ve parametre snapshot'ı.
- Job outbox aynı transaction'da oluşur. Lease süresi dolunca retry mümkün; attempt/fencing token eski worker'ın geç teslimini reddeder. “Exactly once” varsayımı yok; teslim idempotent ve transactional olur.
- Status değişebilir; tamamlanmış input/output değişmez. QC başarısı bilimsel deney için geçti/kaldı hükmü değildir.
- Bir analiz eksik metrikle biterse missing/unsupported nedeni görünür; sıfır değerle tamamlanmış gösterilmez.
- Her test/trial için raporda kullanılan analiz açık seçimle belirlenir; tarihçe silinmez. Aynı hayvanın tekrarları bağımsız denek gibi sayılmaz.

## 10. GCP storage ve video/event ilişkisi

| Kayıt | Sahiplik ve alanlar |
|---|---|
| VideoAsset | media; immutable bucket/object/generation/checksum, duration/timebase, codec ve doğrulama durumu |
| TestRecording | tests/media bağlantısı; video, test/trial ve orijinal videodaki başlangıç/bitiş aralığı |
| AnalysisRun | analysis; sabit girdiler, kullanılan sürümler, durum ve attempt |
| AnalysisVideoPair | analysis; analysisRunId, sourceRecordingId, analyzedVideoAssetId, zaman eşleme sürümü |
| AnalysisEvent | analysis; run/trial/subject bağı, eventType, startUs, endUs, confidence, detection sürümü |
| AnalysisArtifact | analysis; overlay video, trajectory, thumbnail ve diğer GCS obje referansları |

Pair, “bir orijinal için sonsuza kadar tek çıktı” değildir: her analiz sürümü aynı orijinalin ayrı işaretlenmiş çıktısını üretir. Seçili run'ın eventleri yalnız o run'ın videosuyla gösterilir. Orijinal ve analiz edilmiş video GCS'de kalıcı saklanır; her event için ayrı fiziksel klip üretmek başlangıçta gerekli değildir.

Event aralığı [startUs,endUs), kayıt başlangıcına göre mikro-saniye; 0 <= start < end <= recordingDuration. Anlık olay gerekiyorsa ayrı point-event tipi kullanılır. Kaynak kesit offset'i ve çıktı zaman dönüşümü açık taşınır; fps×frame varsayımı değişken frame rate için kullanılmaz, medya timestamp/PTS esas alınır. Overlay aynı zaman çizgisini korur veya açık sürümlü mapping sağlar. Farklı event tiplerinin örtüşmesi geçerlidir; aynı tip için merge/gap/min-duration politikası bilimsel tanımda sürümlenir.

UI: event türü filtreleri, timeline/listede hareket gibi aralıklar; tıklama yan video panelini aralık başına götürür ve aralık sonuna kadar oynatır. Orijinal/analiz videoları eşlenik sunulur; isteğe bağlı senkron oynatma ve seçili event vurgusu. Run değiştirme, hızlı ardışık tıklama, URL süresi dolması, buffering ve video hazır olmaması ele alınır. Signed read URL yenilendiğinde oynatma konumu korunur.

GCS adapter ayrıntıları:
- Private bucket, uniform IAM/public access prevention; Go SDK ve workload identity/ADC, repo içinde service account JSON yok.
- Yetki kontrolünden sonra kısa ömürlü read URL; URL DB kimliği değildir. DB immutable object generation saklar. URL/session URI loglanmaz.
- Büyük videolar resumable upload. Session URI bearer yetkisidir; yalnız upload sahibiyle paylaşılır. CORS gerekli UI origin/method/header ile sınırlandırılır.
- Upload finalize yalnız istemci “bitti” dedi diye kabul edilmez: server object generation, size, checksum ve media metadata doğrular. Corrupt/oversized/yanlış tür karantinada/rejected; iş başlatılmaz.
- Çıktı object anahtarları run/attempt bazlı benzersizdir; generation precondition ile overwrite/yanlış silme önlenir. Kazanan attempt'in tüm referansları DB transaction'ında yayınlanır.
- GCS ile PostgreSQL tek transaction değildir: önce obje tamamlanır/doğrulanır, sonra DB finalization; crash sonrası orphan reconciliation gerekir. Başarılı pair eski retry yüzünden silinmez.
- Tarayıcı seek için range request ve uygun codec/container; gerçek bucket ile range/expiry/CORS doğrulanır.
- Orijinal ve sonuç videolarında varsayılan otomatik silme yok. Geçici/başarısız attempt çıktıları ayrı prefix ve güvenli orphan cleanup; retention daha sonra açık politika olarak seçilir.

GCS yaklaşımı resmi resumable upload, signed URL, Go client ve generation precondition belgelerine dayanır (bölüm 4 kaynakları).

## 11. Tamamlanma ölçütü

Go Clean Architecture ve RBAC testleri; yeni domain invariant'ları; 11 sürümlü hardcode paradigma sözleşmesi; gerekli unit/integration/E2E testleri; gerçek GCS'de orijinal/analiz video pair; gerçek pilot videoda kalibrasyon→otomatik analiz→event timeline→yan panel seek; domain raporları ve temiz kurulum geçmeden proje yayına açılmaz. PR merge hedefi new-backend olarak kalır. Sadece katalogda tanımlı olmak gerçek CV desteğini kanıtlamaz; her ilan edilen worker capability ayrıca doğrulanır.
