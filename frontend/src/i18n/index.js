import { createI18n } from "vue-i18n";

// Auto-discover every locale in ./locales/*.js. Adding a new language is just
// dropping a file there, with no other code changes needed. Each feature area
// keeps its own messages in ./locales/<code>/<area>.js, merged on top.
const modules = import.meta.glob("./locales/*.js", { eager: true });
const areas = import.meta.glob("./locales/*/*.js", { eager: true });

const messages = {};
export const availableLocales = []; // [{ code, name }]

function merge(target, source) {
  for (const [key, value] of Object.entries(source)) {
    if (value && typeof value === "object" && !Array.isArray(value)) {
      target[key] = merge(target[key] && typeof target[key] === "object" ? target[key] : {}, value);
    } else {
      target[key] = value;
    }
  }
  return target;
}

for (const path in modules) {
  const code = path.match(/\/([^/]+)\.js$/)[1];
  const mod = modules[path].default;
  messages[code] = mod.messages;
  availableLocales.push({ code, name: mod.name });
}
for (const path in areas) {
  const code = path.match(/\/locales\/([^/]+)\//)[1];
  if (messages[code]) merge(messages[code], areas[path].default);
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
