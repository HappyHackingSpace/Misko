# new-backend — GitHub iş listesi

Üst iş: [[new-backend] Go Clean Architecture ve GCP video analizi](https://github.com/HappyHackingSpace/Misko/issues/123)

Tüm issue’lar açıktır; bu liste uygulama tamamlandı anlamına gelmez. PR base `new-backend`, çalışma branch’i `codex/...`. Gerekli testler ilgili implementasyon PR’ında yazılır.

| Adım | Issue | Önkoşullar |
|---|---|---|
| 01 | [Go Clean Architecture iskeleti ve test altyapısı](https://github.com/HappyHackingSpace/Misko/issues/124) | — |
| 02 | [Kimlik, laboratuvar ve mevcut RBAC kuralları](https://github.com/HappyHackingSpace/Misko/issues/125) | [124](https://github.com/HappyHackingSpace/Misko/issues/124) |
| 03 | [Denek, deney, aşama, grup ve katılım domainleri](https://github.com/HappyHackingSpace/Misko/issues/126) | [125](https://github.com/HappyHackingSpace/Misko/issues/125) |
| 04 | [Hastalık, madde ve gerçekleşen uygulamalar](https://github.com/HappyHackingSpace/Misko/issues/127) | [126](https://github.com/HappyHackingSpace/Misko/issues/126) |
| 05 | [Hardcode Open Field kataloğu ve metrik motoru](https://github.com/HappyHackingSpace/Misko/issues/128) | [124](https://github.com/HappyHackingSpace/Misko/issues/124) |
| 06 | [Diğer 10 hardcode paradigma ve bilimsel sözleşmeleri](https://github.com/HappyHackingSpace/Misko/issues/129) | [128](https://github.com/HappyHackingSpace/Misko/issues/128) |
| 07 | [Ortam sürümleri ve test protokolleri](https://github.com/HappyHackingSpace/Misko/issues/130) | [126](https://github.com/HappyHackingSpace/Misko/issues/126), [128](https://github.com/HappyHackingSpace/Misko/issues/128) |
| 08 | [Test, trial ve yorum use case’leri](https://github.com/HappyHackingSpace/Misko/issues/131) | [127](https://github.com/HappyHackingSpace/Misko/issues/127), [130](https://github.com/HappyHackingSpace/Misko/issues/130) |
| 09 | [GCP Cloud Storage ve orijinal/analiz video eşlemesi](https://github.com/HappyHackingSpace/Misko/issues/132) | [125](https://github.com/HappyHackingSpace/Misko/issues/125), [131](https://github.com/HappyHackingSpace/Misko/issues/131) |
| 10 | [Video bazlı sürümlü kalibrasyon](https://github.com/HappyHackingSpace/Misko/issues/133) | [130](https://github.com/HappyHackingSpace/Misko/issues/130), [132](https://github.com/HappyHackingSpace/Misko/issues/132) |
| 11 | [Analiz kuyruğu, sonuçlar ve event aralıkları](https://github.com/HappyHackingSpace/Misko/issues/134) | [129](https://github.com/HappyHackingSpace/Misko/issues/129), [131](https://github.com/HappyHackingSpace/Misko/issues/131), [132](https://github.com/HappyHackingSpace/Misko/issues/132), [133](https://github.com/HappyHackingSpace/Misko/issues/133) |
| 12 | [Gerçek Open Field analizi ve işaretlenmiş video üretimi](https://github.com/HappyHackingSpace/Misko/issues/135) | [134](https://github.com/HappyHackingSpace/Misko/issues/134) |
| 13 | [Vue deney akışı ve event tıklamalı eşlenik video paneli](https://github.com/HappyHackingSpace/Misko/issues/136) | [134](https://github.com/HappyHackingSpace/Misko/issues/134), [135](https://github.com/HappyHackingSpace/Misko/issues/135) |
| 14 | [Domain bazlı sorgular, sonuç karşılaştırma ve export](https://github.com/HappyHackingSpace/Misko/issues/137) | [134](https://github.com/HappyHackingSpace/Misko/issues/134) |
| 15 | [Tam sistem kabulü ve yayın öncesi kapanış](https://github.com/HappyHackingSpace/Misko/issues/138) | [136](https://github.com/HappyHackingSpace/Misko/issues/136), [137](https://github.com/HappyHackingSpace/Misko/issues/137) |

Plan: [GO_CLEAN_ARCH_REFACTOR.md](GO_CLEAN_ARCH_REFACTOR.md).

Plan PR’ı bu issue’ları kapatmaz. Non-default branch’e merge sonrasında otomatik kapanma varsayılmaz; kabul kriterleri doğrulanarak tamamlanır.
