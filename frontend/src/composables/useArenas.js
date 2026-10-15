// The arenas a session can film: the latest revision of every environment of
// a paradigm version. Loaded once per screen from the revision summaries.
import { ref } from "vue";
import { lookups } from "../api/tests.js";

export function useArenas() {
  const revisions = ref([]);

  async function load() {
    revisions.value = (await lookups.environmentRevisions()).data || [];
  }

  // Latest revision per environment, for one paradigm version, by name.
  function forParadigm(paradigmKey, paradigmVersion) {
    const latest = new Map();
    for (const r of revisions.value) {
      if (r.paradigmKey !== paradigmKey || r.paradigmVersion !== paradigmVersion) continue;
      const known = latest.get(r.environmentId);
      if (!known || known.number < r.number) latest.set(r.environmentId, r);
    }
    return [...latest.values()].sort((a, b) => a.environmentName.localeCompare(b.environmentName));
  }

  return { revisions, load, forParadigm };
}
