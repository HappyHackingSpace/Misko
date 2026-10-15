// Paradigm names in the panel language, so a session says "Open field" rather
// than OPEN_FIELD. The catalog answers in the request language, so the names
// are fetched again when the language changes.
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { lookups } from "../api/tests.js";

const names = ref({});
let loadedFor = "";

export function useParadigmNames() {
  const { locale } = useI18n();

  async function load() {
    if (loadedFor === locale.value) return;
    loadedFor = locale.value;
    try {
      const list = (await lookups.paradigms()).data || [];
      names.value = Object.fromEntries(list.map((p) => [p.key, p.name]));
    } catch {
      loadedFor = "";
    }
  }
  load();
  watch(locale, load);

  const paradigmName = (key) => names.value[key] || key;
  return { paradigmName, names };
}
