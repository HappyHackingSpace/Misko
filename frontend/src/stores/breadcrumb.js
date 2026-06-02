import { defineStore } from "pinia";

// Dynamic breadcrumb override for detail pages. Most pages derive their crumb
// automatically from the route meta `titleKey`; data-driven titles (e.g. a
// paradigm name) must be fetched first, so this store lets a page temporarily
// override its own crumb trail.
// `trail`: array of [{ label, to? }]; items with `to` render as links, the last is text.
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
