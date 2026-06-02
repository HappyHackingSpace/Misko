import { reactive } from "vue";
import { api } from "../api.js";

/**
 * Drives a server-side DataTable: owns pagination / search / sort state, fetches
 * the `{ data, total, page, pageSize }` envelope and keeps a reactive view of it.
 * Search changes are debounced and reset to page 1.
 *
 * @param {string} endpoint  list endpoint, e.g. "/subjects"
 * @param {object} [options]
 * @param {number} [options.pageSize]   initial page size (default 10)
 * @param {{field:string,order:"asc"|"desc"}} [options.defaultSort]
 * @param {() => Record<string,string|number>} [options.extraParams]  extra query params (e.g. lang)
 */
export function useDataTable(endpoint, options = {}) {
  const state = reactive({
    rows: [],
    total: 0,
    page: 1,
    pageSize: options.pageSize || 10,
    search: "",
    sort: options.defaultSort || { field: "createdAt", order: "desc" },
    loading: false,
    error: "",
  });

  let seq = 0;
  let timer = null;

  function buildQuery() {
    const p = new URLSearchParams();
    p.set("page", state.page);
    p.set("pageSize", state.pageSize);
    if (state.search) p.set("search", state.search);
    if (state.sort?.field) {
      p.set("sort", state.sort.field);
      p.set("order", state.sort.order);
    }
    if (options.extraParams) {
      for (const [k, v] of Object.entries(options.extraParams())) {
        if (v != null && v !== "") p.set(k, v);
      }
    }
    return p.toString();
  }

  async function fetchData() {
    const my = ++seq;
    state.loading = true;
    state.error = "";
    try {
      const res = await api(`${endpoint}?${buildQuery()}`);
      if (my !== seq) return; // a newer request superseded this one
      state.rows = res.data ?? [];
      state.total = res.total ?? 0;
    } catch (e) {
      if (my !== seq) return;
      state.error = e.message;
    } finally {
      if (my === seq) state.loading = false;
    }
  }

  function debouncedFetch() {
    clearTimeout(timer);
    timer = setTimeout(fetchData, 250);
  }

  function setPage(p) {
    state.page = p;
    fetchData();
  }
  function setPageSize(n) {
    state.pageSize = n;
    state.page = 1;
    fetchData();
  }
  function setSearch(s) {
    state.search = s;
    state.page = 1;
    debouncedFetch();
  }
  function setSort(s) {
    state.sort = s;
    fetchData();
  }
  function reload() {
    fetchData();
  }

  fetchData();

  return { state, setPage, setPageSize, setSearch, setSort, reload };
}
