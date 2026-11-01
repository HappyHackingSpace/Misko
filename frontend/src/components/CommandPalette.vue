<script setup>
// One search across studies, their reports, test sessions and subjects. The
// header button or Ctrl+K (Cmd+K on a Mac) opens it from any page.
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NModal } from "naive-ui";
import { experiments } from "../api/experiments.js";
import { subjects } from "../api/subjects.js";
import { tests } from "../api/tests.js";

const { t } = useI18n();
const router = useRouter();

const LIMIT = 5;
const shortcut = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘K" : "Ctrl K";

const open = ref(false);
const query = ref("");
const groups = ref([]);
const active = ref(0);
const loading = ref(false);
const failed = ref(false);
const input = ref(null);

function onKey(e) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    open.value = true;
  }
}
onMounted(() => window.addEventListener("keydown", onKey));
onUnmounted(() => window.removeEventListener("keydown", onKey));

watch(open, async (isOpen) => {
  if (!isOpen) return;
  query.value = "";
  groups.value = [];
  await nextTick();
  input.value?.focus();
});

// Typing waits a moment before asking, and an older answer that arrives late
// never replaces a newer one.
let timer;
let latest = 0;
watch(query, (text) => {
  clearTimeout(timer);
  const search = text.trim();
  latest++;
  loading.value = !!search;
  failed.value = false;
  if (!search) groups.value = [];
  else timer = setTimeout(() => find(search, latest), 200);
});

async function find(search, ticket) {
  const studyQuery = (status) => experiments.list({ search, status, pageSize: LIMIT, sort: "code", order: "asc" });
  const answers = await Promise.allSettled([
    studyQuery(),
    // Reports are asked for apart, so matching drafts (no report yet) do not
    // use up their places.
    studyQuery("RUNNING"),
    studyQuery("COMPLETED"),
    tests.worklist({ search, pageSize: LIMIT }),
    subjects.list({ search, pageSize: LIMIT, sort: "code", order: "asc" }),
  ]);
  if (ticket !== latest) return;
  // A list the user may not read just stays out of the results.
  const [studies, running, completed, sessions, animals] = answers.map((a) => (a.status === "fulfilled" ? a.value.data || [] : []));
  failed.value = answers.every((a) => a.status === "rejected");
  const reported = [...running, ...completed].sort((a, b) => a.code.localeCompare(b.code)).slice(0, LIMIT);
  groups.value = [
    {
      key: "studies",
      items: studies.map((e) => ({ id: `study-${e.id}`, title: e.code, detail: e.title, meta: t(`experiments.status.${e.status}`), to: `/experiments/${e.id}` })),
    },
    {
      key: "reports",
      items: reported.map((e) => ({ id: `report-${e.id}`, title: e.code, detail: e.title, to: `/experiments/${e.id}?tab=results` })),
    },
    {
      key: "sessions",
      items: sessions.map((s) => ({
        id: `session-${s.id}`,
        title: `${s.experimentCode} · ${s.timepointName}`,
        detail: (s.subjects || []).map((x) => x.subjectCode).join(", "),
        meta: t(`dashboard.testStatus.${s.status}`),
        to: `/tests/${s.id}`,
      })),
    },
    {
      key: "subjects",
      items: animals.map((a) => ({
        id: `subject-${a.id}`,
        title: a.code,
        detail: a.allocation ? `${a.allocation.experimentCode} · ${a.allocation.groupCode}` : "",
        to: `/subjects/${a.id}`,
      })),
    },
  ].filter((g) => g.items.length);
  active.value = 0;
  loading.value = false;
}

const items = computed(() => groups.value.flatMap((g) => g.items));

function go(item) {
  if (!item) return;
  open.value = false;
  router.push(item.to);
}

function onInputKey(e) {
  const n = items.value.length;
  if (e.key === "ArrowDown" && n) active.value = (active.value + 1) % n;
  else if (e.key === "ArrowUp" && n) active.value = (active.value - 1 + n) % n;
  // Until the answer for the current text arrives, the marked result belongs
  // to the previous text.
  else if (e.key === "Enter") {
    if (!loading.value) go(items.value[active.value]);
  }
  else return;
  e.preventDefault();
  nextTick(() => document.querySelector(".palette .item.active")?.scrollIntoView({ block: "nearest" }));
}
</script>

<template>
  <button class="search-btn" type="button" :aria-label="$t('search.open')" data-test="search-open" @click="open = true">
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true">
      <circle cx="11" cy="11" r="7" />
      <path d="M21 21l-4.3-4.3" />
    </svg>
    <span class="label">{{ $t("search.open") }}</span>
    <kbd>{{ shortcut }}</kbd>
  </button>

  <NModal v-model:show="open" :auto-focus="false" transform-origin="center">
    <div class="palette" role="dialog" aria-modal="true" :aria-label="$t('search.open')" data-test="search-palette">
      <div class="field">
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true">
          <circle cx="11" cy="11" r="7" />
          <path d="M21 21l-4.3-4.3" />
        </svg>
        <input
          ref="input"
          v-model="query"
          type="search"
          maxlength="100"
          :placeholder="$t('search.placeholder')"
          data-test="search-input"
          @keydown="onInputKey"
        />
        <button type="button" class="close" :aria-label="$t('search.close')" :title="$t('search.close')" @click="open = false"><kbd>Esc</kbd></button>
      </div>
      <div class="results">
        <p v-if="!query.trim()" class="note">{{ $t("search.hint") }}</p>
        <p v-else-if="loading && !groups.length" class="note">{{ $t("search.searching") }}</p>
        <p v-else-if="failed" class="note" data-test="search-failed">{{ $t("search.failed") }}</p>
        <p v-else-if="!groups.length" class="note" data-test="search-empty">{{ $t("search.empty", { query: query.trim() }) }}</p>
        <section v-for="group in groups" :key="group.key" :data-test="`search-${group.key}`">
          <h3>{{ $t(`search.groups.${group.key}`) }}</h3>
          <a
            v-for="item in group.items"
            :key="item.id"
            :href="router.resolve(item.to).href"
            class="item"
            :class="{ active: items[active] === item }"
            data-test="search-result"
            @mouseenter="active = items.indexOf(item)"
            @click.prevent="go(item)"
          >
            <span class="title">{{ item.title }}</span>
            <span class="detail">{{ item.detail }}</span>
            <span v-if="item.meta" class="meta">{{ item.meta }}</span>
          </a>
        </section>
      </div>
    </div>
  </NModal>
</template>

<style scoped>
.search-btn {
  display: inline-flex; align-items: center; gap: 8px; height: 34px; padding: 0 8px 0 10px; min-width: 220px;
  border: 1px solid var(--line); border-radius: 8px; background: var(--panel2); color: var(--muted);
  font: inherit; font-size: 13px; cursor: pointer;
}
.search-btn:hover { border-color: var(--accent); color: var(--txt); }
.search-btn .label { flex: 1; text-align: left; }
kbd {
  padding: 1px 6px; border: 1px solid var(--line); border-radius: 5px; background: var(--panel);
  font-family: inherit; font-size: 11px; color: var(--muted);
}

.palette {
  width: min(600px, calc(100vw - 32px)); max-height: min(560px, calc(100vh - 120px));
  display: flex; flex-direction: column; overflow: hidden;
  border: 1px solid var(--line); border-radius: 14px; background: var(--panel); box-shadow: var(--shadow-md);
}
.field { display: flex; align-items: center; gap: 10px; padding: 12px 14px; border-bottom: 1px solid var(--line); color: var(--muted); }
.field input { flex: 1; min-width: 0; border: none; outline: none; background: transparent; color: var(--txt); font: inherit; font-size: 15px; }
.field input::-webkit-search-cancel-button { display: none; }
.close { padding: 0; border: none; background: none; cursor: pointer; }
.results { overflow-y: auto; padding: 6px; }
.note { margin: 0; padding: 22px 12px; text-align: center; color: var(--muted); font-size: 13.5px; }
h3 { margin: 8px 10px 4px; font-size: 11px; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
.item {
  display: flex; align-items: baseline; gap: 10px; padding: 8px 10px; border-radius: 8px;
  color: var(--txt); text-decoration: none; font-size: 13.5px;
}
.item.active { background: var(--active-bg); }
.title { font-weight: 600; white-space: nowrap; }
.detail { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--muted); }
.meta { font-size: 12px; color: var(--muted); white-space: nowrap; }

@media (max-width: 860px) {
  .search-btn { min-width: 0; padding: 0 9px; border-color: transparent; background: transparent; }
  .search-btn .label, .search-btn kbd { display: none; }
}
</style>
