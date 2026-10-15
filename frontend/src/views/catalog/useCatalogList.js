import { onMounted, onUnmounted, reactive, watch } from "vue";

// State of one catalog list. The API returns the whole (small) list ordered by
// name and filters it on the server by name and by one enum; the table pages it
// on the client. Typing is debounced, a newer answer always wins.
export function useCatalogList(fetch, filterKey) {
  const state = reactive({ rows: [], loading: false, error: "", search: "", filter: null });
  let seq = 0;
  let timer = null;

  async function load() {
    const my = ++seq;
    state.loading = true;
    state.error = "";
    try {
      const res = await fetch({ search: state.search.trim(), [filterKey]: state.filter });
      if (my === seq) state.rows = res.data || [];
    } catch (e) {
      if (my === seq) state.error = e.message;
    } finally {
      if (my === seq) state.loading = false;
    }
  }

  watch(() => state.filter, load);
  watch(
    () => state.search,
    () => {
      clearTimeout(timer);
      timer = setTimeout(load, 250);
    },
  );
  onMounted(load);
  onUnmounted(() => clearTimeout(timer));

  return { state, load };
}
