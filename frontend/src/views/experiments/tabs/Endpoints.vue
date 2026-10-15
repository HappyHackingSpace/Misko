<script setup>
// What the study is judged on: a metric of a paradigm the protocols run,
// measured at a timepoint, optionally as change from a baseline timepoint.
// Exactly one endpoint is primary. Fixed once the study runs.
import { computed, h, inject, onMounted, ref, watch, watchEffect } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDataTable, NModal, NPopconfirm, NRadioButton, NRadioGroup, NSelect, NSpin, NTag, NTooltip, useMessage } from "naive-ui";
import { endpoints as endpointsApi, experiments, timepoints as timepointsApi } from "../../../api/experiments.js";
import { selectOption } from "../../../composables/selectOptions.js";
import { useAuth } from "../../../stores/auth.js";
import { DIRECTIONS, ENDPOINT_KINDS, useDesignFormat, useParadigms, useTabLink } from "../design/shared.js";

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const fmt = useDesignFormat();
const paradigms = useParadigms();
const goTab = useTabLink();
const { experiment, id } = inject("study");

const editable = computed(() => experiment.value?.status === "DRAFT" && auth.can("study:write"));
const rows = ref([]);
const timepoints = ref([]);
const options = ref([]);
const loading = ref(false);
const error = ref("");

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [e, tp, mo] = await Promise.all([endpointsApi.list(id.value), timepointsApi.list(id.value), experiments.metricOptions(id.value)]);
    rows.value = e.data || [];
    timepoints.value = tp.data || [];
    options.value = mo.data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

// Manifests give paradigm and metric names in the current language.
watchEffect(() => {
  paradigms.loadList();
  for (const o of options.value) paradigms.loadManifest(o.paradigmKey, o.paradigmVersion);
});

// Metric options merged per paradigm across the versions the protocols run.
const byParadigm = computed(() => {
  const map = {};
  for (const o of options.value) {
    map[o.paradigmKey] = map[o.paradigmKey] || new Set();
    o.metrics.forEach((m) => map[o.paradigmKey].add(m));
  }
  return map;
});
const metricInfo = (paradigmKey, metricKey) => {
  for (const o of options.value.filter((x) => x.paradigmKey === paradigmKey)) {
    const found = paradigms.manifest(o.paradigmKey, o.paradigmVersion)?.metrics.find((m) => m.key === metricKey);
    if (found) return found;
  }
  return paradigms.anyManifest(paradigmKey)?.metrics.find((m) => m.key === metricKey) || null;
};
const metricLabel = (paradigmKey, metricKey) => metricInfo(paradigmKey, metricKey)?.label || metricKey;
const timepointText = (tpId) => {
  const tp = timepoints.value.find((x) => x.id === tpId);
  return tp ? fmt.timepoint(tp) : "-";
};

const columns = computed(() => [
  {
    title: t("design.endpoints.kind"),
    key: "kind",
    width: 120,
    render: (r) => h(NTag, { size: "small", round: true, bordered: false, type: r.kind === "PRIMARY" ? "info" : "default" }, () => t(`design.endpointKinds.${r.kind}`)),
  },
  { title: t("design.protocols.paradigm"), key: "paradigmKey", render: (r) => paradigms.name(r.paradigmKey) },
  {
    title: t("design.endpoints.metric"),
    key: "metricKey",
    render: (r) => {
      const info = metricInfo(r.paradigmKey, r.metricKey);
      const label = h("span", { "data-test": "endpoint-metric-label" }, info?.label || r.metricKey);
      return info?.description ? h(NTooltip, null, { trigger: () => label, default: () => info.description }) : label;
    },
  },
  { title: t("design.endpoints.timepoint"), key: "timepointId", render: (r) => timepointText(r.timepointId) },
  { title: t("design.endpoints.baseline"), key: "baselineTimepointId", render: (r) => (r.baselineTimepointId ? timepointText(r.baselineTimepointId) : "-") },
  { title: t("design.endpoints.direction"), key: "direction", render: (r) => t(`design.directions.${r.direction}`) },
  ...(editable.value
    ? [
        {
          title: t("common.actions"),
          key: "actions",
          width: 160,
          render: (r) =>
            h("div", { class: "row-actions" }, [
              h(NButton, { size: "tiny", quaternary: true, onClick: () => openForm(r) }, () => t("common.edit")),
              h(
                NPopconfirm,
                { onPositiveClick: () => remove(r) },
                { trigger: () => h(NButton, { size: "tiny", quaternary: true, type: "error" }, () => t("common.delete")), default: () => t("design.endpoints.deleteConfirm") },
              ),
            ]),
        },
      ]
    : []),
]);

// Form.
const open = ref(false);
const editing = ref(null);
const form = ref({});
const formError = ref("");
const saving = ref(false);
function openForm(row = null) {
  editing.value = row;
  formError.value = "";
  form.value = row
    ? { ...row, baselineTimepointId: row.baselineTimepointId || null }
    : {
        timepointId: null,
        baselineTimepointId: null,
        paradigmKey: Object.keys(byParadigm.value)[0] || null,
        metricKey: null,
        kind: rows.value.some((r) => r.kind === "PRIMARY") ? "SECONDARY" : "PRIMARY",
        direction: "HIGHER_IS_BETTER",
      };
  open.value = true;
}
const paradigmOptions = computed(() => Object.keys(byParadigm.value).map((k) => selectOption("endpoint-paradigm", k, paradigms.name(k))));
const metricOptions = computed(() =>
  [...(byParadigm.value[form.value.paradigmKey] || [])].map((m) => selectOption("endpoint-metric", m, metricLabel(form.value.paradigmKey, m))),
);
const timepointOptions = computed(() => timepoints.value.map((tp) => selectOption("endpoint-timepoint", tp.id, fmt.timepoint(tp))));
const baselineOptions = computed(() =>
  timepoints.value.filter((tp) => tp.id !== form.value.timepointId).map((tp) => selectOption("endpoint-baseline", tp.id, fmt.timepoint(tp))),
);
const chosenMetric = computed(() => (form.value.metricKey ? metricInfo(form.value.paradigmKey, form.value.metricKey) : null));
watch(
  () => form.value.paradigmKey,
  (key, old) => {
    if (old !== undefined && key !== old && !byParadigm.value[key]?.has(form.value.metricKey)) form.value.metricKey = null;
  },
);
watch(
  () => form.value.timepointId,
  (tp) => {
    if (tp && form.value.baselineTimepointId === tp) form.value.baselineTimepointId = null;
  },
);

async function save() {
  formError.value = "";
  saving.value = true;
  const f = form.value;
  const body = { timepointId: f.timepointId, paradigmKey: f.paradigmKey, metricKey: f.metricKey, kind: f.kind, direction: f.direction };
  try {
    if (editing.value) {
      await endpointsApi.update(id.value, editing.value.id, { ...body, baselineTimepointId: f.baselineTimepointId || "" });
    } else {
      if (f.baselineTimepointId) body.baselineTimepointId = f.baselineTimepointId;
      await endpointsApi.create(id.value, body);
    }
    open.value = false;
    message.success(t("design.endpoints.saved"));
    await load();
  } catch (e) {
    formError.value = e.message;
  } finally {
    saving.value = false;
  }
}
async function remove(row) {
  error.value = "";
  try {
    await endpointsApi.remove(id.value, row.id);
    message.success(t("design.endpoints.deleted"));
    await load();
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(load);
watch(() => experiment.value?.status, load);
</script>

<template>
  <div class="endpoints">
    <NAlert v-if="experiment && experiment.status !== 'DRAFT'" type="info" :title="$t('design.frozen')" />
    <NAlert v-if="error" type="error" :title="error" data-test="endpoints-error" />
    <NCard size="small" :title="$t('design.endpoints.title')">
      <template #header-extra>
        <NButton v-if="editable" type="primary" size="small" :disabled="!options.length || !timepoints.length" data-test="endpoint-add" @click="openForm()">
          {{ $t("design.endpoints.add") }}
        </NButton>
      </template>
      <p class="muted intro">{{ $t("design.endpoints.intro") }}</p>
      <NAlert v-if="!loading && editable && (!options.length || !timepoints.length)" type="warning" class="intro">
        {{ !timepoints.length ? $t("design.endpoints.needTimepoints") : $t("design.endpoints.needProtocols") }}
        <a href="#" class="link" @click.prevent="goTab('timeline')">{{ $t("experiments.tabs.timeline") }}</a>
      </NAlert>
      <NSpin :show="loading">
        <NDataTable :columns="columns" :data="rows" :row-key="(r) => r.id" size="small" :scroll-x="820" data-test="endpoints-table">
          <template #empty><span class="muted">{{ $t("design.endpoints.empty") }}</span></template>
        </NDataTable>
      </NSpin>
    </NCard>

    <NModal v-model:show="open" preset="card" :title="editing ? $t('design.endpoints.edit') : $t('design.endpoints.add')" style="width: 560px; max-width: 94vw">
      <div class="form" data-test="endpoint-form">
        <div class="fld">
          <span>{{ $t("design.protocols.paradigm") }}</span>
          <NSelect v-model:value="form.paradigmKey" :options="paradigmOptions" data-test="endpoint-paradigm" />
        </div>
        <div class="fld">
          <span>{{ $t("design.endpoints.metric") }}</span>
          <NSelect v-model:value="form.metricKey" :options="metricOptions" filterable data-test="endpoint-metric" />
        </div>
        <small v-if="chosenMetric" class="muted wide">{{ chosenMetric.description }}</small>
        <div class="fld">
          <span>{{ $t("design.endpoints.timepoint") }}</span>
          <NSelect v-model:value="form.timepointId" :options="timepointOptions" data-test="endpoint-timepoint" />
        </div>
        <div class="fld">
          <span>{{ $t("design.endpoints.baseline") }}</span>
          <NSelect v-model:value="form.baselineTimepointId" :options="baselineOptions" clearable :placeholder="$t('design.endpoints.noBaseline')" data-test="endpoint-baseline" />
        </div>
        <div class="fld wide">
          <span>{{ $t("design.endpoints.kind") }}</span>
          <NRadioGroup v-model:value="form.kind" data-test="endpoint-kind">
            <NRadioButton v-for="k in ENDPOINT_KINDS" :key="k" :value="k" :data-test="`endpoint-kind-${k}`">{{ $t(`design.endpointKinds.${k}`) }}</NRadioButton>
          </NRadioGroup>
        </div>
        <div class="fld wide">
          <span>{{ $t("design.endpoints.direction") }}</span>
          <NRadioGroup v-model:value="form.direction" data-test="endpoint-direction">
            <NRadioButton v-for="d in DIRECTIONS" :key="d" :value="d" :data-test="`endpoint-direction-${d}`">{{ $t(`design.directions.${d}`) }}</NRadioButton>
          </NRadioGroup>
          <small class="muted">{{ $t("design.endpoints.directionHint") }}</small>
        </div>
        <NAlert v-if="formError" class="wide" type="error" :title="formError" data-test="endpoint-error" />
      </div>
      <template #footer>
        <div class="dialog-actions">
          <NButton @click="open = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" :disabled="!form.paradigmKey || !form.metricKey || !form.timepointId" data-test="endpoint-save" @click="save">
            {{ $t("common.save") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.endpoints { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.intro { margin: 0 0 12px; }
:deep(.row-actions) { display: flex; gap: 6px; }
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.fld > span { font-size: 12px; color: var(--muted); }
.wide { grid-column: 1 / -1; }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
