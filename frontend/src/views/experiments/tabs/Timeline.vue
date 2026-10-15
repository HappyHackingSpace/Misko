<script setup>
// When animals are tested (timepoints, in study days from day 0) and what each
// test session runs (protocols: versioned lists of paradigm steps).
// Timepoints are fixed once the study runs; protocols get new versions until
// it completes.
import { computed, h, inject, onMounted, ref, watch, watchEffect } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDataTable, NEmpty, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NSpin, NTag, useMessage } from "naive-ui";
import { timepoints as timepointsApi } from "../../../api/experiments.js";
import { protocols as protocolsApi } from "../../../api/protocols.js";
import { useAuth } from "../../../stores/auth.js";
import { useParameters } from "../../../composables/useParameters.js";
import { useDesignFormat, useParadigms } from "../design/shared.js";
import ProtocolForm from "../design/ProtocolForm.vue";

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const fmt = useDesignFormat();
const paradigms = useParadigms();
const { format: formatParameter } = useParameters();
const { experiment, id } = inject("study");

const status = computed(() => experiment.value?.status);
const canWrite = computed(() => auth.can("study:write"));
const timepointsEditable = computed(() => status.value === "DRAFT" && canWrite.value);
const protocolsEditable = computed(() => status.value !== "COMPLETED" && canWrite.value);

const timepoints = ref([]);
const protocolList = ref([]);
// protocolId -> versions (oldest first)
const versions = ref({});
// protocolId -> chosen version number
const shown = ref({});
const loading = ref(false);
const error = ref("");

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [tp, pr] = await Promise.all([timepointsApi.list(id.value), protocolsApi.list(id.value)]);
    timepoints.value = tp.data || [];
    protocolList.value = pr.data || [];
    const all = await Promise.all(protocolList.value.map((p) => protocolsApi.versions(id.value, p.id)));
    const byId = {};
    protocolList.value.forEach((p, i) => (byId[p.id] = all[i].data || []));
    versions.value = byId;
    const chosen = {};
    for (const p of protocolList.value) chosen[p.id] = p.latestVersion;
    shown.value = chosen;
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

// Timepoints.
const tpOpen = ref(false);
const tpEditing = ref(null);
const tpForm = ref({ name: "", studyDay: 0, position: 1 });
const tpError = ref("");
const tpSaving = ref(false);
function addTimepoint() {
  tpEditing.value = null;
  const nextPosition = Math.max(0, ...timepoints.value.map((x) => x.position)) + 1;
  const lastDay = timepoints.value.length ? Math.max(...timepoints.value.map((x) => x.studyDay)) : -1;
  tpForm.value = { name: "", studyDay: lastDay + 1, position: nextPosition };
  tpError.value = "";
  tpOpen.value = true;
}
function editTimepoint(row) {
  tpEditing.value = row;
  tpForm.value = { name: row.name, studyDay: row.studyDay, position: row.position };
  tpError.value = "";
  tpOpen.value = true;
}
async function saveTimepoint() {
  tpError.value = "";
  tpSaving.value = true;
  const body = { name: tpForm.value.name.trim(), studyDay: tpForm.value.studyDay, position: tpForm.value.position };
  try {
    if (tpEditing.value) await timepointsApi.update(id.value, tpEditing.value.id, body);
    else await timepointsApi.create(id.value, body);
    tpOpen.value = false;
    message.success(t("design.timepoints.saved"));
    await load();
  } catch (e) {
    tpError.value = e.message;
  } finally {
    tpSaving.value = false;
  }
}
async function deleteTimepoint(row) {
  error.value = "";
  try {
    await timepointsApi.remove(id.value, row.id);
    message.success(t("design.timepoints.deleted"));
    await load();
  } catch (e) {
    error.value = e.message;
  }
}

const tpColumns = computed(() => [
  { title: t("design.timepoints.position"), key: "position", width: 80 },
  { title: t("common.name"), key: "name" },
  { title: t("design.timepoints.studyDay"), key: "studyDay", render: (row) => fmt.studyDay(row.studyDay) },
  ...(experiment.value?.dayZero
    ? [{ title: t("design.timepoints.date"), key: "date", render: (row) => fmt.date(addDays(experiment.value.dayZero, row.studyDay)) }]
    : []),
  ...(timepointsEditable.value
    ? [
        {
          title: t("common.actions"),
          key: "actions",
          width: 170,
          render: (row) =>
            h("div", { class: "row-actions" }, [
              h(NButton, { size: "tiny", quaternary: true, onClick: () => editTimepoint(row) }, () => t("common.edit")),
              h(
                NPopconfirm,
                { onPositiveClick: () => deleteTimepoint(row) },
                { trigger: () => h(NButton, { size: "tiny", quaternary: true, type: "error" }, () => t("common.delete")), default: () => t("design.timepoints.deleteConfirm") },
              ),
            ]),
        },
      ]
    : []),
]);

function addDays(isoDate, days) {
  const [y, m, d] = isoDate.split("-").map(Number);
  const date = new Date(y, m - 1, d + days);
  const pad = (n) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

// Protocols.
const versionOf = (p) => (versions.value[p.id] || []).find((v) => v.number === shown.value[p.id]) || null;
const versionOptions = (p) =>
  (versions.value[p.id] || []).map((v) => ({ value: v.number, label: t("design.protocols.versionN", { n: v.number }) + (v.number === p.latestVersion ? ` (${t("design.protocols.latest")})` : "") }));

watchEffect(() => {
  paradigms.loadList();
  for (const list of Object.values(versions.value)) {
    for (const v of list) for (const s of v.steps) paradigms.loadManifest(s.paradigmKey, s.paradigmVersion);
  }
});

const trialTypeLabel = (step) => paradigms.manifest(step.paradigmKey, step.paradigmVersion)?.trialTypes.find((x) => x.key === step.trialType)?.label || step.trialType;
const sessionText = (step) => {
  const params = (paradigms.manifest(step.paradigmKey, step.paradigmVersion)?.parameters || []).filter((p) => p.scope === "SESSION");
  return params.map((p) => `${p.label}: ${formatParameter(p, step.session?.[p.key])}`).join(" · ");
};

const stepColumns = computed(() => [
  { title: "#", key: "position", width: 44 },
  {
    title: t("design.protocols.paradigm"),
    key: "paradigm",
    minWidth: 150,
    render: (s) => `${paradigms.name(s.paradigmKey)} · ${t("design.protocols.versionN", { n: s.paradigmVersion })}`,
  },
  { title: t("design.protocols.trialType"), key: "trialType", minWidth: 120, render: (s) => trialTypeLabel(s) },
  { title: t("design.protocols.trials"), key: "trials", width: 80 },
  { title: t("design.protocols.interval"), key: "interTrialIntervalS", minWidth: 110, render: (s) => t("design.protocols.secondsValue", { n: fmt.number(s.interTrialIntervalS) }) },
  { title: t("design.protocols.sessionSettings"), key: "session", minWidth: 240, render: (s) => sessionText(s) || "-" },
  { title: t("common.notes"), key: "notes", minWidth: 100, render: (s) => s.notes || "-" },
]);

const formOpen = ref(false);
const formMode = ref("create");
const formProtocol = ref(null);
const formSteps = ref([]);
function openProtocol(mode, p = null) {
  formMode.value = mode;
  formProtocol.value = p;
  const latest = p ? (versions.value[p.id] || []).find((v) => v.number === p.latestVersion) : null;
  formSteps.value = latest ? latest.steps : [];
  formOpen.value = true;
}
async function onProtocolSaved() {
  message.success(t("design.protocols.saved"));
  await load();
}

onMounted(load);
watch(status, load);
</script>

<template>
  <div class="timeline">
    <NAlert v-if="error" type="error" :title="error" data-test="timeline-error" />
    <NSpin :show="loading">
      <div class="stack">
        <NCard size="small" :title="$t('design.timepoints.title')">
          <template #header-extra>
            <NButton v-if="timepointsEditable" type="primary" size="small" data-test="timepoint-add" @click="addTimepoint">{{ $t("design.timepoints.add") }}</NButton>
          </template>
          <p class="muted intro">{{ status === "DRAFT" ? $t("design.timepoints.intro") : $t("design.frozen") }}</p>
          <div v-if="timepoints.length" class="strip" aria-hidden="true">
            <div v-for="tp in timepoints" :key="tp.id" class="mark">
              <span class="day">{{ fmt.studyDay(tp.studyDay) }}</span>
              <span class="pin"></span>
              <span class="tp-name">{{ tp.name }}</span>
            </div>
          </div>
          <NDataTable :columns="tpColumns" :data="timepoints" :row-key="(r) => r.id" size="small" :scroll-x="520" data-test="timepoints-table">
            <template #empty><span class="muted">{{ $t("design.timepoints.empty") }}</span></template>
          </NDataTable>
        </NCard>

        <NCard size="small" :title="$t('design.protocols.title')">
          <template #header-extra>
            <NButton v-if="protocolsEditable" type="primary" size="small" data-test="protocol-add" @click="openProtocol('create')">{{ $t("design.protocols.add") }}</NButton>
          </template>
          <p class="muted intro">{{ $t("design.protocols.intro") }}</p>
          <NEmpty v-if="!loading && !protocolList.length" :description="$t('design.protocols.empty')" />
          <div v-for="p in protocolList" :key="p.id" class="protocol" :data-test="`protocol-${p.name}`">
            <div class="protocol-head">
              <div class="protocol-title">
                <strong>{{ p.name }}</strong>
                <NTag size="small" round :bordered="false">{{ $t("design.protocols.versionCount", { n: p.latestVersion }) }}</NTag>
              </div>
              <NSelect
                v-if="(versions[p.id] || []).length > 1"
                v-model:value="shown[p.id]"
                size="small"
                class="version-select"
                :options="versionOptions(p)"
                data-test="protocol-version-select"
              />
              <div v-if="protocolsEditable" class="row-actions">
                <NButton size="small" data-test="protocol-new-version" @click="openProtocol('version', p)">{{ $t("design.protocols.newVersion") }}</NButton>
                <NButton size="small" quaternary @click="openProtocol('rename', p)">{{ $t("common.edit") }}</NButton>
              </div>
            </div>
            <p v-if="p.description" class="muted desc">{{ p.description }}</p>
            <template v-if="versionOf(p)">
              <p v-if="versionOf(p).notes" class="muted desc">{{ versionOf(p).notes }}</p>
              <NDataTable :columns="stepColumns" :data="versionOf(p).steps" :row-key="(s) => s.position" size="small" :scroll-x="900" data-test="protocol-steps" />
              <small class="muted">{{ $t("design.protocols.versionCreated", { date: fmt.dateTime(versionOf(p).createdAt) }) }}</small>
            </template>
          </div>
        </NCard>
      </div>
    </NSpin>

    <NModal v-model:show="tpOpen" preset="card" :title="tpEditing ? $t('design.timepoints.edit') : $t('design.timepoints.add')" style="width: 480px; max-width: 94vw">
      <form class="form" data-test="timepoint-form" @submit.prevent="saveTimepoint">
        <label class="fld wide">
          <span>{{ $t("common.name") }}</span>
          <NInput v-model:value="tpForm.name" :maxlength="120" :placeholder="$t('design.timepoints.namePlaceholder')" :input-props="{ 'data-test': 'timepoint-name' }" />
        </label>
        <label class="fld">
          <span>{{ $t("design.timepoints.studyDay") }}</span>
          <NInputNumber v-model:value="tpForm.studyDay" :min="-365" :max="3650" :precision="0" :input-props="{ 'data-test': 'timepoint-day' }" />
        </label>
        <label class="fld">
          <span>{{ $t("design.timepoints.position") }}</span>
          <NInputNumber v-model:value="tpForm.position" :min="1" :max="1000" :precision="0" :input-props="{ 'data-test': 'timepoint-position' }" />
        </label>
        <small class="muted wide">{{ $t("design.timepoints.dayHint") }}</small>
        <NAlert v-if="tpError" class="wide" type="error" :title="tpError" data-test="timepoint-error" />
      </form>
      <template #footer>
        <div class="dialog-actions">
          <NButton @click="tpOpen = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="tpSaving" :disabled="!tpForm.name.trim() || tpForm.studyDay === null || !tpForm.position" data-test="timepoint-save" @click="saveTimepoint">
            {{ $t("common.save") }}
          </NButton>
        </div>
      </template>
    </NModal>

    <ProtocolForm v-model:show="formOpen" :experiment-id="id" :mode="formMode" :protocol="formProtocol" :steps="formSteps" @saved="onProtocolSaved" />
  </div>
</template>

<style scoped>
.timeline, .stack { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.intro { margin: 0 0 12px; }
.strip { display: flex; gap: 0; overflow-x: auto; margin-bottom: 12px; padding-bottom: 4px; border-bottom: 2px solid var(--line); }
.mark { display: flex; flex-direction: column; align-items: center; min-width: 96px; flex: 1; gap: 2px; font-size: 12px; }
.day { color: var(--muted); }
.pin { width: 10px; height: 10px; border-radius: 50%; background: var(--accent); }
.tp-name { font-weight: 600; text-align: center; overflow-wrap: anywhere; }
.protocol { border-top: 1px solid var(--line); padding: 12px 0; display: flex; flex-direction: column; gap: 8px; min-width: 0; }
.protocol:first-of-type { border-top: 0; }
.protocol-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.protocol-title { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 160px; }
.version-select { width: 190px; }
.desc { margin: 0; white-space: pre-wrap; }
:deep(.row-actions) { display: flex; gap: 6px; flex-wrap: wrap; }
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 14px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld > span { font-size: 12px; color: var(--muted); }
.wide { grid-column: 1 / -1; }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
