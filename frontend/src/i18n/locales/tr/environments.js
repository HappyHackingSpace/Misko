export default {
  environments: {
    title: "Ortamlar",
    intro:
      "Bir paradigma şablonundan oluşturulan, adlandırılmış arenalar. Aynı paradigmadan birden çok ortam olabilir (örneğin iki Morris su tankı). Ölçüler burada girilir ve her test çalıştırıldığı revizyonu korur.",
    allParadigms: "Tüm paradigmalar",
    noMatch: "Bu filtrelere uyan ortam yok.",
    created: "{name} ortamı oluşturuldu",
    revisionSaved: "Yeni revizyon oluşturuldu",
    calibration: {
      intro: "Düzenek kurulurken bir kez kalibre edin. Bu arenada test edilen her hayvan, testi elle kalibre edilmedikçe bunu kullanır.",
      waiting: "Henüz kalibre edilmedi. Kalibre edilene kadar bu arenada test edilen hayvanlar santimetre cinsinden analiz edilemez.",
      rejected: "Reddedildi: kontrol noktaları toleranstan fazla sapıyor. Düzeltilene kadar bu arenadaki analizler bekler.",
      fixedCamera: "Bu, kamera ve arenanın yerinde kaldığını varsayar. Videosu uymayan bir hayvan, kendi testinden elle kalibre edilebilir.",
      saved: "Kalibrasyon kaydedildi",
    },
  },
  errors: {
    common: {
      invalidNotes: "Notlar en fazla 2000 karakter olabilir",
    },
    environment: {
      notFound: "Ortam bulunamadı",
      revisionNotFound: "Ortamın bu revizyonu bulunamadı",
      nameTaken: "Bu ad başka bir ortamda kullanılıyor",
      unknownParadigm: "Bilinmeyen paradigma",
      unknownParadigmVersion: "Bu paradigma sürümü yayımlanmamış",
      invalidParameters: "Değerler paradigmaya uymuyor: her düzenek ölçüsünü aralığı içinde ve diğerleriyle tutarlı girin",
      invalidParadigmKey: "Bir paradigma seçin",
      invalidParadigmVersion: "Paradigma sürümü geçerli değil",
    },
    calibration: {
      invalidScope: "Bu kalibrasyon buraya ait değil",
    },
  },
};
