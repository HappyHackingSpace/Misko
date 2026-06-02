import { createI18n } from "vue-i18n";

// Auto-discover every locale in ./locales/*.js. Adding a new language is just
// dropping a file there — no other code changes needed.
const modules = import.meta.glob("./locales/*.js", { eager: true });

const messages = {};
export const availableLocales = []; // [{ code, name }]

for (const path in modules) {
  const code = path.match(/\/([^/]+)\.js$/)[1];
  const mod = modules[path].default;
  messages[code] = mod.messages;
  availableLocales.push({ code, name: mod.name });
}
availableLocales.sort((a, b) => a.name.localeCompare(b.name));

export const DEFAULT_LOCALE = "tr";
export const FALLBACK_LOCALE = "en";

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: DEFAULT_LOCALE,
  fallbackLocale: FALLBACK_LOCALE,
  messages,
});
