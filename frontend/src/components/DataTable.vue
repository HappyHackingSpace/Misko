<script setup>
import { computed, useSlots } from "vue";
import { useI18n } from "vue-i18n";

/**
 * Server-driven data table: row index, global search, sortable headers,
 * pagination and export (CSV + printable PDF). State is controlled by the parent
 * (usually via useDataTable); this component renders the UI and emits change events.
 *
 * Columns: { key, label, sortable?, cellClass?, exportValue? }
 *   exportValue(row): value used for CSV/PDF export when the cell is rendered
 *     from a nested/translated field (defaults to row[col.key])
 *
 * Per-column cell rendering: use a scoped slot named `cell-<key>` (receives { row }).
 * Trailing action buttons: use the `actions` slot (receives { row }).
 */
const props = defineProps({
  columns: { type: Array, required: true },
  rows: { type: Array, default: () => [] },
  total: { type: Number, default: 0 },
  page: { type: Number, default: 1 },
  pageSize: { type: Number, default: 10 },
  sort: { type: Object, default: () => ({ field: "createdAt", order: "desc" }) },
  search: { type: String, default: "" },
  loading: { type: Boolean, default: false },
  rowKey: { type: String, default: "id" },
  pageSizes: { type: Array, default: () => [10, 20, 50] },
  indexed: { type: Boolean, default: true },
  exportable: { type: Boolean, default: true },
  exportName: { type: String, default: "export" },
  entityLabel: { type: String, default: "" },
  emptyHint: { type: String, default: "" },
});

const emit = defineEmits(["page", "pageSize", "sort", "search"]);

const { t, locale } = useI18n();
const slots = useSlots();

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)));
const rangeFrom = computed(() => (props.total === 0 ? 0 : (props.page - 1) * props.pageSize + 1));
const rangeTo = computed(() => Math.min(props.total, props.page * props.pageSize));
const colSpan = computed(
  () => props.columns.length + (props.indexed ? 1 : 0) + (slots.actions ? 1 : 0),
);

// Distinguish "this domain has no records yet" from "nothing matched the search".
const isSearching = computed(() => !!props.search);
const emptyMessage = computed(() => {
  if (isSearching.value) return t("datatable.noResults");
  if (props.entityLabel) return t("datatable.emptyDomain", { entity: props.entityLabel });
  return t("datatable.noResults");
});

function rowIndex(i) {
  return (props.page - 1) * props.pageSize + i + 1;
}

function toggleSort(col) {
  if (!col.sortable) return;
  if (props.sort.field === col.key) {
    emit("sort", { field: col.key, order: props.sort.order === "asc" ? "desc" : "asc" });
  } else {
    emit("sort", { field: col.key, order: "asc" });
  }
}

function sortIcon(col) {
  if (props.sort.field !== col.key) return "";
  return props.sort.order === "asc" ? "▲" : "▼";
}

// aria-sort value for assistive tech on sortable headers.
function ariaSort(col) {
  if (props.sort.field !== col.key) return "none";
  return props.sort.order === "asc" ? "ascending" : "descending";
}

function prev() {
  if (props.page > 1) emit("page", props.page - 1);
}
function next() {
  if (props.page < totalPages.value) emit("page", props.page + 1);
}

// --- Export (current page) ---------------------------------------------------
function cellText(col, row) {
  const v = col.exportValue ? col.exportValue(row) : row[col.key];
  return v == null ? "" : String(v);
}

function buildMatrix() {
  const headers = [];
  if (props.indexed) headers.push("#");
  for (const c of props.columns) headers.push(c.label);
  const body = props.rows.map((row, i) => {
    const cells = [];
    if (props.indexed) cells.push(String(rowIndex(i)));
    for (const c of props.columns) cells.push(cellText(c, row));
    return cells;
  });
  return { headers, body };
}

function stamp() {
  return new Date().toISOString().slice(0, 10);
}

function csvCell(value) {
  // Neutralize spreadsheet formula injection: a cell starting with =, +, -, @ or
  // a leading tab/CR can be evaluated as a formula by Excel/Sheets. Prefix it with
  // a single quote so it is treated as plain text.
  let v = value;
  if (/^[=+\-@\t\r]/.test(v)) v = `'${v}`;
  if (/[",\r\n]/.test(v)) return `"${v.replace(/"/g, '""')}"`;
  return v;
}

function exportCsv() {
  if (!props.rows.length) return;
  const { headers, body } = buildMatrix();
  const lines = [headers, ...body].map((r) => r.map(csvCell).join(","));
  // Prepend a BOM so spreadsheets open the UTF-8 file with the right encoding.
  const content = "﻿" + lines.join("\r\n");
  const blob = new Blob([content], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${props.exportName}-${stamp()}.csv`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function escapeHtml(value) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function exportPdf() {
  if (!props.rows.length) return;
  const { headers, body } = buildMatrix();
  const title = props.entityLabel || props.exportName;
  const subtitle = new Date().toLocaleString(locale.value);
  const thead = `<tr>${headers.map((h) => `<th>${escapeHtml(h)}</th>`).join("")}</tr>`;
  const tbody = body
    .map((r) => `<tr>${r.map((c) => `<td>${escapeHtml(c)}</td>`).join("")}</tr>`)
    .join("");
  const html = `<!doctype html><html><head><meta charset="utf-8">
    <title>${escapeHtml(title)}</title>
    <style>
      body { font: 13px -apple-system, system-ui, sans-serif; color: #1c1c1c; margin: 24px; }
      h1 { font-size: 18px; margin: 0 0 2px; }
      .sub { color: #666; margin: 0 0 16px; font-size: 12px; }
      table { border-collapse: collapse; width: 100%; }
      th, td { border: 1px solid #ccc; padding: 6px 8px; text-align: left; }
      thead th { background: #f2f2f2; }
      tbody tr:nth-child(even) td { background: #fafafa; }
    </style></head><body>
    <h1>${escapeHtml(title)}</h1>
    <p class="sub">${escapeHtml(subtitle)}</p>
    <table><thead>${thead}</thead><tbody>${tbody}</tbody></table>
    </body></html>`;
  const w = window.open("", "_blank");
  if (!w) return;
  w.document.write(html);
  w.document.close();
  // Content is inline (no external resources), so it is ready synchronously.
  w.focus();
  w.print();
}

// Dropdown acts like a menu: pick a format, run it, then reset to the label.
function onExport(e) {
  const kind = e.target.value;
  e.target.value = "";
  if (kind === "csv") exportCsv();
  else if (kind === "pdf") exportPdf();
}
</script>

<template>
  <div class="dt">
    <div class="dt-toolbar">
      <input
        class="dt-search"
        type="search"
        :value="search"
        :placeholder="$t('datatable.searchPlaceholder')"
        @input="emit('search', $event.target.value)"
      />
      <div class="dt-toolbar-right">
        <span class="dt-count muted">{{ $t("datatable.total", { n: total }) }}</span>
        <div v-if="exportable" class="dt-export">
          <select class="dt-export-select" :disabled="!rows.length" @change="onExport">
            <option value="">{{ $t("datatable.export") }}</option>
            <option value="csv">{{ $t("datatable.exportCsv") }}</option>
            <option value="pdf">{{ $t("datatable.exportPdf") }}</option>
          </select>
        </div>
      </div>
    </div>

    <div class="dt-scroll">
      <table>
        <thead>
          <tr>
            <th v-if="indexed" class="dt-idx">#</th>
            <th
              v-for="col in columns"
              :key="col.key"
              :class="{ sortable: col.sortable }"
              :tabindex="col.sortable ? 0 : null"
              :role="col.sortable ? 'button' : null"
              :aria-sort="col.sortable ? ariaSort(col) : null"
              @click="toggleSort(col)"
              @keydown.enter.prevent="toggleSort(col)"
              @keydown.space.prevent="toggleSort(col)"
            >
              {{ col.label }}
              <span v-if="col.sortable" class="dt-sort">{{ sortIcon(col) }}</span>
            </th>
            <th v-if="$slots.actions"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, i) in rows" :key="row[rowKey]">
            <td v-if="indexed" class="dt-idx muted">{{ rowIndex(i) }}</td>
            <td v-for="col in columns" :key="col.key" :class="col.cellClass">
              <slot :name="`cell-${col.key}`" :row="row">{{ row[col.key] }}</slot>
            </td>
            <td v-if="$slots.actions" class="dt-actions">
              <slot name="actions" :row="row" />
            </td>
          </tr>
          <tr v-if="!rows.length && !loading" class="dt-empty-row">
            <td :colspan="colSpan">
              <div class="dt-empty">
                <span class="dt-empty-title">{{ emptyMessage }}</span>
                <span v-if="!isSearching && emptyHint" class="muted dt-empty-hint">{{ emptyHint }}</span>
              </div>
            </td>
          </tr>
          <tr v-if="loading && !rows.length" class="dt-empty-row">
            <td :colspan="colSpan"><div class="dt-empty muted">{{ $t("datatable.loading") }}</div></td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="dt-footer">
      <div class="dt-pagesize">
        <span class="muted">{{ $t("datatable.rowsPerPage") }}</span>
        <select :value="pageSize" @change="emit('pageSize', Number($event.target.value))">
          <option v-for="n in pageSizes" :key="n" :value="n">{{ n }}</option>
        </select>
      </div>
      <div class="dt-pager">
        <span class="muted">{{ $t("datatable.showing", { from: rangeFrom, to: rangeTo, total }) }}</span>
        <button :disabled="page <= 1" @click="prev">{{ $t("datatable.prev") }}</button>
        <span class="dt-page">{{ $t("datatable.pageOf", { page, pages: totalPages }) }}</span>
        <button :disabled="page >= totalPages" @click="next">{{ $t("datatable.next") }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dt { display: flex; flex-direction: column; gap: 10px; }
.dt-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.dt-toolbar-right { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.dt-search { min-width: 220px; max-width: 320px; }
.dt-export { display: flex; align-items: center; gap: 6px; }
.dt-export-select { font-size: 12px; padding: 4px 8px; cursor: pointer; }
.dt-export-select:disabled { opacity: 0.45; cursor: default; }
.dt-scroll { overflow-x: auto; }
th.sortable { cursor: pointer; user-select: none; white-space: nowrap; }
th.sortable:hover { color: var(--accent); }
.dt-sort { font-size: 10px; opacity: 0.8; }
.dt-idx { width: 1%; white-space: nowrap; text-align: right; }
.dt-actions { white-space: nowrap; }
.dt-actions :deep(button) { margin-right: 6px; }
tbody tr:hover td { background: var(--active-bg, #f6f6f6); }
.dt-empty-row td { border: none; }
.dt-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 32px 12px;
  text-align: center;
}
.dt-empty-title { font-weight: 600; }
.dt-empty-hint { font-size: 13px; }
.dt-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.dt-pagesize, .dt-pager { display: flex; align-items: center; gap: 8px; }
.dt-page { min-width: 90px; text-align: center; }
</style>
