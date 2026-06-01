import { defineStore } from "pinia";
import { i18n, availableLocales, DEFAULT_LOCALE } from "../i18n/index.js";

const THEME_KEY = "misko.theme";
const LOCALE_KEY = "misko.locale";
const THEMES = ["dark", "light"];

function knownLocale(code) {
  return availableLocales.some((l) => l.code === code) ? code : null;
}

function initialTheme() {
  const saved = localStorage.getItem(THEME_KEY);
  if (THEMES.includes(saved)) return saved;
  // First visit: honor the OS preference, default to dark.
  return window.matchMedia?.("(prefers-color-scheme: light)").matches
    ? "light"
    : "dark";
}

function initialLocale() {
  return (
    knownLocale(localStorage.getItem(LOCALE_KEY)) ||
    knownLocale(navigator.language?.slice(0, 2)) ||
    DEFAULT_LOCALE
  );
}

export const usePrefs = defineStore("prefs", {
  state: () => ({
    theme: "dark",
    locale: DEFAULT_LOCALE,
    locales: availableLocales,
  }),
  actions: {
    // Called once at startup to sync store ↔ DOM ↔ i18n from saved prefs.
    init() {
      this.setTheme(initialTheme());
      this.setLocale(initialLocale());
    },
    setTheme(theme) {
      if (!THEMES.includes(theme)) return;
      this.theme = theme;
      document.documentElement.setAttribute("data-theme", theme);
      localStorage.setItem(THEME_KEY, theme);
    },
    toggleTheme() {
      this.setTheme(this.theme === "dark" ? "light" : "dark");
    },
    setLocale(code) {
      const next = knownLocale(code) || DEFAULT_LOCALE;
      this.locale = next;
      i18n.global.locale.value = next;
      document.documentElement.setAttribute("lang", next);
      localStorage.setItem(LOCALE_KEY, next);
    },
  },
});
