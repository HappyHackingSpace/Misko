import { useI18n } from "vue-i18n";

// Excel in Turkish (and most European) locales splits columns on ";", so a
// comma-separated file opens as a single column.
const CSV_DELIMITER = ";";

/**
 * CSV / printable-PDF export for a server-driven table, decoupled from how the
 * table itself renders cells (Naive UI's column `render` returns vnodes, not
 * text, so export needs its own plain-value getters).
 *
 * @param {object} options
 * @param {object} options.state    a useDataTable() state (reads .rows live)
 * @param {Array<{key:string,label:string,value?:(row)=>unknown}>} options.columns
 *   value(row) defaults to row[key] when omitted
 * @param {string} options.exportName  file name prefix, e.g. "subjects"
 * @param {string} [options.entityLabel]  title used in the printable PDF
 */
export function useTableExport({ state, columns, exportName, entityLabel }) {
  const { locale } = useI18n();

  function cellText(col, row) {
    const v = col.value ? col.value(row) : row[col.key];
    return v == null ? "" : String(v);
  }

  function buildMatrix() {
    const headers = columns.map((c) => c.label);
    const body = state.rows.map((row) => columns.map((c) => cellText(c, row)));
    return { headers, body };
  }

  function stamp() {
    return new Date().toISOString().slice(0, 10);
  }

  function csvCell(value) {
    // Neutralize spreadsheet formula injection: a cell starting with =, +, -, @ or
    // a leading tab/CR can be evaluated as a formula by Excel/Sheets. Prefix it
    // with a single quote so it is treated as plain text.
    let v = value;
    // Plain numbers (e.g. -3.5) are left alone so they stay numeric.
    if (/^[=+\-@\t\r]/.test(v) && !/^-?\d+([.,]\d+)?$/.test(v)) v = `'${v}`;
    if (/[";,\r\n]/.test(v)) return `"${v.replace(/"/g, '""')}"`;
    return v;
  }

  function exportCsv() {
    if (!state.rows.length) return;
    const { headers, body } = buildMatrix();
    const lines = [headers, ...body].map((r) => r.map(csvCell).join(CSV_DELIMITER));
    // Prepend a BOM so spreadsheets open the UTF-8 file with the right encoding.
    const content = "﻿" + lines.join("\r\n");
    const blob = new Blob([content], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${exportName}-${stamp()}.csv`;
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
    if (!state.rows.length) return;
    const { headers, body } = buildMatrix();
    const title = entityLabel || exportName;
    const subtitle = new Date().toLocaleString(locale.value);
    const thead = `<tr>${headers.map((h) => `<th>${escapeHtml(h)}</th>`).join("")}</tr>`;
    const tbody = body.map((r) => `<tr>${r.map((c) => `<td>${escapeHtml(c)}</td>`).join("")}</tr>`).join("");
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
    w.focus();
    w.print();
  }

  return { exportCsv, exportPdf };
}
