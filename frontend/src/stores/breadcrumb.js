import { defineStore } from "pinia";

// Detay sayfalari icin dinamik kirinti (breadcrumb) gecisi. Cogu sayfa
// route meta'sindaki `titleKey`'den otomatik kirinti uretir; ancak veriye
// bagli basliklar (or. paradigma adi) once getirilmek zorundadir. Bu store
// bir sayfanin kendi kirinti zincirini gecici olarak gecersiz kilmasini saglar.
// `trail`: [{ label, to? }] dizisi; `to` verilen ogeler link, son oge metindir.
export const useBreadcrumb = defineStore("breadcrumb", {
  state: () => ({ trail: null }),
  actions: {
    set(trail) {
      this.trail = trail;
    },
    clear() {
      this.trail = null;
    },
  },
});
