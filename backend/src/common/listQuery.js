/**
 * Shared list-query helper: turns pagination / search / per-column filter / sort
 * query params into Prisma arguments and a consistent response envelope.
 *
 * Response envelope: { data, total, page, pageSize }.
 *
 * Query params (all optional):
 *   page       1-based page number (default 1)
 *   pageSize   rows per page (default 10, max 100)
 *   all        "true" returns every row (used by form dropdowns); skips pagination
 *   search     global term matched against `searchFields` (contains, case-insensitive)
 *   filter[x]  per-column filter for field x (see filterFields types below)
 *   sort       column to order by (must be in `sortFields`)
 *   order      "asc" | "desc"
 */

const DEFAULT_PAGE_SIZE = 10;
const MAX_PAGE_SIZE = 100;

function toInt(value, fallback) {
  const n = Number.parseInt(value, 10);
  return Number.isInteger(n) ? n : fallback;
}

/**
 * @param {object} query  req.query
 * @param {object} config
 * @param {string[]} [config.searchFields]  text columns for the global `search` term
 * @param {Record<string,"text"|"enum"|"boolean">} [config.filterFields]  per-column filters
 * @param {string[]} [config.sortFields]  columns allowed in `sort`
 * @param {{field:string,order:"asc"|"desc"}} [config.defaultSort]  fallback ordering
 */
export function buildListQuery(query = {}, config = {}) {
  const {
    searchFields = [],
    filterFields = {},
    sortFields = [],
    defaultSort = { field: "createdAt", order: "desc" },
  } = config;

  const all = query.all === "true" || query.all === true;

  let page = toInt(query.page, 1);
  if (page < 1) page = 1;
  let pageSize = toInt(query.pageSize, DEFAULT_PAGE_SIZE);
  if (pageSize < 1) pageSize = DEFAULT_PAGE_SIZE;
  if (pageSize > MAX_PAGE_SIZE) pageSize = MAX_PAGE_SIZE;

  const and = [];

  const search = typeof query.search === "string" ? query.search.trim() : "";
  if (search && searchFields.length) {
    and.push({
      OR: searchFields.map((f) => ({ [f]: { contains: search, mode: "insensitive" } })),
    });
  }

  const filters = query.filter && typeof query.filter === "object" ? query.filter : {};
  for (const [field, type] of Object.entries(filterFields)) {
    const raw = filters[field];
    if (raw === undefined || raw === null || raw === "") continue;
    if (type === "boolean") {
      if (raw === "true") and.push({ [field]: true });
      else if (raw === "false") and.push({ [field]: false });
      else if (raw === "null") and.push({ [field]: null });
    } else if (type === "enum") {
      and.push({ [field]: raw });
    } else {
      and.push({ [field]: { contains: String(raw), mode: "insensitive" } });
    }
  }
  const where = and.length ? { AND: and } : {};

  const sortField = typeof query.sort === "string" && sortFields.includes(query.sort)
    ? query.sort
    : defaultSort.field;
  let order;
  if (query.order === "asc" || query.order === "desc") order = query.order;
  else order = sortField === defaultSort.field ? defaultSort.order : "asc";
  const orderBy = { [sortField]: order };

  return {
    where,
    orderBy,
    skip: all ? undefined : (page - 1) * pageSize,
    take: all ? undefined : pageSize,
    page,
    pageSize,
    all,
  };
}

/** Wraps rows + total count in the standard list envelope. */
export function listResult(data, total, q) {
  return {
    data,
    total,
    page: q.all ? 1 : q.page,
    pageSize: q.all ? total : q.pageSize,
  };
}

/**
 * In-memory pagination/search/sort for non-database listings (e.g. the static
 * paradigm registry). `rows` is already-localized data; `getText` extracts the
 * searchable text for a row.
 */
export function paginateArray(rows, query = {}, config = {}) {
  const { searchText = () => "", sortFields = [], defaultSort = { field: "name", order: "asc" }, filter } = config;

  let out = rows;

  const search = typeof query.search === "string" ? query.search.trim().toLowerCase() : "";
  if (search) out = out.filter((row) => searchText(row).toLowerCase().includes(search));

  if (typeof filter === "function") out = out.filter(filter(query.filter || {}));

  const sortField = typeof query.sort === "string" && sortFields.includes(query.sort)
    ? query.sort
    : defaultSort.field;
  const dir = query.order === "desc" ? -1 : query.order === "asc" ? 1
    : sortField === defaultSort.field && defaultSort.order === "desc" ? -1 : 1;
  out = [...out].sort((a, b) => {
    const av = a[sortField];
    const bv = b[sortField];
    if (typeof av === "number" && typeof bv === "number") return (av - bv) * dir;
    return String(av ?? "").localeCompare(String(bv ?? "")) * dir;
  });

  const total = out.length;
  const all = query.all === "true" || query.all === true;
  let page = toInt(query.page, 1);
  if (page < 1) page = 1;
  let pageSize = toInt(query.pageSize, DEFAULT_PAGE_SIZE);
  if (pageSize < 1) pageSize = DEFAULT_PAGE_SIZE;
  if (pageSize > MAX_PAGE_SIZE) pageSize = MAX_PAGE_SIZE;

  const data = all ? out : out.slice((page - 1) * pageSize, page * pageSize);
  return { data, total, page: all ? 1 : page, pageSize: all ? total : pageSize };
}
