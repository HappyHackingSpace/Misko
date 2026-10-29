<script setup>
// The review queue: analysis results across experiments waiting for a human
// decision (or, with the status filter, the decided ones). Only accepted
// results enter the statistics. A row opens the animal's analysed video,
// quality checks, metrics and run choice in a drawer. Clean pending rows can be
// accepted together. In a blinded experiment the API leaves the group out and
// so does this screen.
import { computed, h, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { NAlert, NButton, NDataTable, NDrawer, NDrawerContent, NInput, NPopconfirm, NSelect, NSpin, NTag, useMessage } from "naive-ui";
import { experimentOptions, results } from "../../api/analysis.js";
import { useAuth } from "../../stores/auth.js";
import { selectOption, filterByText } from "../../composables/selectOptions.js";
import PageHead from "../../components/PageHead.vue";
import ReviewDetail from "../../components/analysis/ReviewDetail.vue";
import { reviewTag, useNumbers, useParadigmText } from "../../components/analysis/format.js";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const auth = useAuth();
const message = useMessage();
const fmt = useNumbers();
const text = useParadigmText();

const STATUSES = ["PENDING", "ACCEPTED", "REJECTED", "ALL"];
const status = computed(() => (STATUSES.includes(route.query.status) ? route.query.status : "PENDING"));
const experimentId = computed(() => route.query.experimentId || null);
const setQuery = (patch) => router.replace({ query: { ...route.query, ...patch } });

const rows = ref([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(25);
const loading = ref(false);
const error = ref("");
const search = ref("");
const experiments = ref([]);
const checked = ref([]);
const bulkBusy = ref(false);

let seq = 0;
async function load() {
  const my = ++seq;
  loading.value = true;
  error.value = "";
  try {
    const res = await results.list({ experimentId: experimentId.value, status: status.value, page: page.value, pageSize: pageSize.value });
    if (my !== seq) return;
    rows.value = res.data || [];
    total.value = res.total || 0;
    checked.value = checked.value.filter((id) => rows.value.some((row) => row.testSubjectId === id && row.status === "PENDING"));
    for (const row of rows.value) text.need(row.paradigmKey, row.paradigmVersion);
  } catch (e) {
    if (my === seq) error.value = e.message;
  } finally {
    if (my === seq) loading.value = false;
  }
}
watch([status, experimentId], () => {
  page.value = 1;
  checked.value = [];
  load();
});
onMounted(async () => {
  load();
  try {
    experiments.value = (await experimentOptions()).data || [];
  } catch {
    experiments.value = [];
  }
});

const canReview = computed(() => auth.can("test:run"));
// The API has no text search on results; the box narrows the page on screen.
const shown = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return rows.value;
  return rows.value.filter((row) =>
    [row.subjectCode, row.experimentCode, row.timepointName, row.arena.environmentName].some((v) => String(v).toLowerCase().includes(q)),
  );
});

// A clean row needs nothing more than a glance: pending, every metric measured,
// no newer run, the animal not excluded and the experiment still open.
const isClean = (row) =>
  row.status === "PENDING" &&
  !row.excluded &&
  !row.newerRunAvailable &&
  row.experimentStatus !== "COMPLETED" &&
  row.run.metrics.every((m) => m.value !== null);
const cleanIds = computed(() => shown.value.filter(isClean).map((row) => row.testSubjectId));
function selectClean() {
  checked.value = [...new Set([...checked.value, ...cleanIds.value])];
}

async function bulkAccept() {
  bulkBusy.value = true;
  error.value = "";
  try {
    const res = await results.bulkAccept(checked.value);
    message.success(t("review.bulkAccepted", { count: res.data.length }));
    checked.value = [];
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    bulkBusy.value = false;
  }
}

// The two metrics a reader looks at first; the drawer has the rest.
const PREFERRED = ["escape_latency_s", "path_length_cm", "target_quadrant_time_ratio", "mean_swim_speed_cm_s"];
function headline(row) {
  const byKey = Object.fromEntries(row.run.metrics.map((m) => [m.key, m]));
  const chosen = PREFERRED.map((key) => byKey[key]).filter(Boolean);
  for (const m of row.run.metrics) if (!chosen.includes(m)) chosen.push(m);
  return chosen.slice(0, 2);
}

function flags(row) {
  const list = [];
  const missing = row.run.metrics.filter((m) => m.value === null).length;
  if (missing) list.push({ key: "missing", type: "warning", text: t("review.flags.missing", { count: missing }) });
  if (row.newerRunAvailable) list.push({ key: "newer", type: "info", text: t("review.flags.newer") });
  if (row.excluded) list.push({ key: "excluded", type: "error", text: t("review.flags.excluded") });
  return list;
}

const columns = computed(() => [
  ...(canReview.value ? [{ type: "selection", disabled: (row) => row.status !== "PENDING" || row.experimentStatus === "COMPLETED" }] : []),
  {
    title: t("review.animal"),
    key: "subjectCode",
    width: 120,
    render: (row) => h("span", { class: "animal-cell" }, [h("b", null, row.subjectCode), h("span", { class: "muted" }, row.experimentCode)]),
  },
  {
    title: t("review.session"),
    key: "timepointName",
    render: (row) =>
      h("span", { class: "animal-cell" }, [
        h("span", null, `${row.timepointName} · ${text.paradigmName(row.paradigmKey)}`),
        h("span", { class: "muted" }, row.arena.environmentName),
      ]),
  },
  {
    title: t("review.group"),
    key: "group",
    render: (row) =>
      row.blinded || !row.group ? h("span", { class: "muted" }, t("review.hidden")) : h("span", null, `${row.group.code} · ${row.group.name}`),
  },
  {
    title: t("review.metrics"),
    key: "metrics",
    render: (row) =>
      h(
        "span",
        { class: "metric-cell" },
        headline(row).map((m) =>
          h("span", { key: m.key }, [h("span", { class: "muted" }, `${text.metricLabel(row.paradigmKey, m.key)}: `), m.value === null ? t("analysis.missing") : fmt.metric(m.value, m.unit, m.key)]),
        ),
      ),
  },
  {
    title: t("review.checksColumn"),
    key: "flags",
    render: (row) => {
      const list = flags(row);
      if (!list.length) return h(NTag, { size: "small", round: true, bordered: false, type: "success" }, () => t("review.flags.clean"));
      return h("span", { class: "flag-cell" }, list.map((f) => h(NTag, { key: f.key, size: "small", round: true, bordered: false, type: f.type, "data-test": `flag-${f.key}` }, () => f.text)));
    },
  },
  {
    title: t("review.statusColumn"),
    key: "status",
    render: (row) =>
      h("span", { class: "flag-cell" }, [
        h(NTag, { size: "small", round: true, bordered: false, type: reviewTag(row.status) }, () => t(`review.status.${row.status}`)),
        row.reason ? h("span", { class: "muted" }, t(`review.reasons.${row.reason}`)) : null,
      ]),
  },
  { title: t("review.analysed"), key: "finishedAt", width: 120, render: (row) => fmt.date(row.run.finishedAt) },
]);

// The drawer: ?open=<testSubjectId> so a link can point at one result.
const openId = computed(() => route.query.open || "");
const openItem = ref(null);
const openLoading = ref(false);
const openError = ref("");
async function loadOpen() {
  if (!openId.value) {
    openItem.value = null;
    return;
  }
  openLoading.value = true;
  openError.value = "";
  try {
    openItem.value = await results.get(openId.value);
  } catch (e) {
    openItem.value = null;
    openError.value = e.message;
  } finally {
    openLoading.value = false;
  }
}
watch(openId, loadOpen, { immediate: true });
const drawerOpen = computed({
  get: () => !!openId.value,
  set: (value) => {
    if (!value) setQuery({ open: undefined });
  },
});
const rowProps = (row) => ({
  style: "cursor: pointer",
  "data-test": "review-row",
  "data-test-subject": row.testSubjectId,
  "data-status": row.status,
  onClick: (event) => {
    // The selection checkbox selects; it does not open.
    if (event.target.closest(".n-checkbox, .n-data-table-td--selection")) return;
    setQuery({ open: row.testSubjectId });
  },
});

// After a decision the drawer moves on to the next pending row of the page, so
// a queue can be worked through without closing it.
async function onChanged(updated) {
  openItem.value = { ...openItem.value, ...updated };
  const index = rows.value.findIndex((row) => row.testSubjectId === updated.testSubjectId);
  await load();
  if (status.value === "PENDING" && updated.status !== "PENDING") {
    const next = rows.value[Math.min(Math.max(index, 0), rows.value.length - 1)];
    setQuery({ open: next ? next.testSubjectId : undefined });
  } else {
    loadOpen();
  }
}

const statusOptions = computed(() => STATUSES.map((s) => selectOption("review-status-filter", s, t(`review.filter.${s}`))));
const experimentSelect = computed(() =>
  experiments.value.map((e) => selectOption("review-experiment-filter", e.id, `${e.code} · ${e.title}`)),
);
const pagination = computed(() => ({
  page: page.value,
  pageSize: pageSize.value,
  itemCount: total.value,
  showSizePicker: true,
  pageSizes: [25, 50, 100, 200],
  onUpdatePage: (p) => {
    page.value = p;
    load();
  },
  onUpdatePageSize: (size) => {
    pageSize.value = size;
    page.value = 1;
    load();
  },
}));
const drawerWidth = computed(() => (typeof window !== "undefined" && window.innerWidth < 760 ? "100%" : 760));
</script>

<template>
  <PageHead :title="$t('review.title')" :subtitle="$t('review.intro')" :count="total" />

  <div class="filters">
    <NSelect
      class="experiment"
      :value="experimentId"
      clearable
      filterable
      :filter="filterByText"
      :placeholder="$t('review.allExperiments')"
      :options="experimentSelect"
      data-test="review-experiment"
      @update:value="setQuery({ experimentId: $event || undefined, open: undefined })"
    />
    <NSelect class="status" :value="status" :options="statusOptions" data-test="review-status" @update:value="setQuery({ status: $event })" />
    <NInput v-model:value="search" class="search" clearable :placeholder="$t('review.search')" data-test="review-search" />
  </div>

  <div v-if="canReview && status === 'PENDING'" class="bulk" data-test="bulk-bar">
    <NButton size="small" secondary :disabled="!cleanIds.length" data-test="select-clean" @click="selectClean">
      {{ $t("review.selectClean", { count: cleanIds.length }) }}
    </NButton>
    <NPopconfirm @positive-click="bulkAccept">
      <template #trigger>
        <NButton size="small" type="primary" :disabled="!checked.length" :loading="bulkBusy" data-test="bulk-accept">
          {{ $t("review.bulkAccept", { count: checked.length }) }}
        </NButton>
      </template>
      {{ $t("review.bulkConfirm", { count: checked.length }) }}
    </NPopconfirm>
    <span class="muted">{{ $t("review.cleanHint") }}</span>
  </div>

  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 12px" />

  <NDataTable
    remote
    data-test="review-table"
    :columns="columns"
    :data="shown"
    :loading="loading"
    :row-key="(row) => row.testSubjectId"
    :row-props="rowProps"
    :scroll-x="1000"
    v-model:checked-row-keys="checked"
    :pagination="pagination"
  >
    <template #empty>
      <span class="muted" data-test="review-empty">{{ status === "PENDING" ? $t("review.emptyPending") : $t("review.empty") }}</span>
    </template>
  </NDataTable>

  <NDrawer v-model:show="drawerOpen" :width="drawerWidth" placement="right">
    <NDrawerContent :title="$t('review.drawerTitle')" closable :native-scrollbar="false">
      <NSpin :show="openLoading">
        <NAlert v-if="openError" type="error" :title="openError" />
        <ReviewDetail v-if="openItem" :item="openItem" :can-review="canReview" @changed="onChanged" />
      </NSpin>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.filters { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 12px; }
.experiment { width: 320px; max-width: 100%; }
.status { width: 200px; max-width: 100%; }
.search { width: 240px; max-width: 100%; }
.bulk { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
:deep(.animal-cell) { display: inline-flex; flex-direction: column; line-height: 1.3; white-space: nowrap; }
:deep(.metric-cell) { display: inline-flex; flex-direction: column; font-variant-numeric: tabular-nums; white-space: nowrap; }
:deep(.flag-cell) { display: inline-flex; gap: 4px; flex-wrap: wrap; align-items: center; }
</style>
