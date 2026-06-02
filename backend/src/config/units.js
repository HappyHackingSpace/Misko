/**
 * Kanonik ölçüm birimleri (Step 2 - bilimsel kontrat).
 *
 * Mişko sonuç metriklerine yalnızca bu birimler girebilir. Piksel değerleri
 * asla sonuç metriklerinde saklanmaz; CV servisi her şeyi apparatus
 * koordinatlarına (cm) çevirdikten sonra gönderir.
 *
 * Kaynak: docs/MEASUREMENTS.md, bolum 3.2 (Unit policy).
 */
export const UNITS = Object.freeze({
  CM: "cm", // konum ve mesafe
  CM_S: "cm_s", // hiz
  S: "s", // sure
  COUNT: "count", // giris, gecis, dusme gibi sayimlar
  RATIO: "ratio", // 0..1 araligi, UI'da yuzde gosterilebilir
  PERCENT: "percent", // dogrudan yuzde tasiyan alanlar
  DEG: "deg", // aci (heading/pose ileride)
  RPM: "rpm", // rotarod devri
  G: "g", // denek agirligi (fizyoloji, CV sonucu degil)
  BOOLEAN: "boolean", // olay var/yok
});

export const UNIT_LIST = Object.freeze(Object.values(UNITS));

const UNIT_SET = new Set(UNIT_LIST);

/** Verilen birim kanonik birim listesinde mi? */
export function isCanonicalUnit(unit) {
  return UNIT_SET.has(unit);
}
