<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import MetricBarChart from "../components/MetricBarChart.vue";
import EventTimeline from "../components/EventTimeline.vue";
import TestComments from "../components/TestComments.vue";

const { t, te, locale } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const test = ref(null);
const err = ref("");
const metricDefs = ref({}); // paradigmKey -> metric defs
const eventTypes = ref({}); // paradigmKey -> event types
const zonesByEnv = ref({}); // envId -> zones
const draft = ref({}); // envId -> { type, t, payload }
const selectedMetric = ref({}); // envId -> metric key (parameter-based chart)
const activeTab = ref({}); // envId -> 'timeline' | 'charts'
const busy = ref("");

const tabOf = (envId) => activeTab.value[envId] || "timeline";
const setTab = (envId, tab) => { activeTab.value[envId] = tab; };
const drawer = ref({ open: false, env: null }); // event-add drawer

const environments = computed(() => test.value?.scenario?.environments || []);
const envEntry = (envId) => test.value?.result?.environments?.[envId] || { status: "PENDING", events: [], metrics: {} };
const statusLabel = (s) => t(`statuses.${s}`);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("tests.title"), to: "/tests" },
    { label: test.value ? `${test.value.scenario?.name} / ${test.value.subject?.code}` : route.params.id },
  ]);
}

function freshDraft(paradigmKey) {
  const first = (eventTypes.value[paradigmKey] || [])[0];
  return { type: first?.type || "", t: "", payload: {} };
}

async function load() {
  err.value = "";
  try {
    test.value = await api(`/tests/${route.params.id}`);
    await Promise.all(environments.value.map(async (env) => {
      if (!metricDefs.value[env.paradigmKey]) {
        const spec = await api(`/paradigms/${env.paradigmKey}?lang=${locale.value}`);
        metricDefs.value[env.paradigmKey] = spec.metrics || [];
        eventTypes.value[env.paradigmKey] = spec.eventTypes || [];
      }
      if (!zonesByEnv.value[env.id]) {
        const full = await api(`/environments/${env.id}`);
        zonesByEnv.value[env.id] = full.config?.zones || [];
      }
      if (!draft.value[env.id]) draft.value[env.id] = freshDraft(env.paradigmKey);
      // default parameter-based chart selection once metrics exist
      if (!selectedMetric.value[env.id]) {
        const opts = metricOptions(env);
        if (opts.length) selectedMetric.value[env.id] = opts[0].key;
      }
    }));
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function patchEnv(envId, body) {
  busy.value = envId;
  err.value = "";
  try {
    await api(`/tests/${route.params.id}/environments/${envId}`, { method: "PATCH", body });
    await load();
  } catch (e) { err.value = e.message; } finally { busy.value = ""; }
}
const startEnv = (envId) => patchEnv(envId, { status: "RUNNING" });
const finishEnv = (envId) => patchEnv(envId, { status: "DONE" });
const editEnv = (envId) => patchEnv(envId, { status: "RUNNING" });

const eventTypeDef = (paradigmKey, type) => (eventTypes.value[paradigmKey] || []).find((e) => e.type === type);

function openDrawer(env) {
  if (!draft.value[env.id]) draft.value[env.id] = freshDraft(env.paradigmKey);
  drawer.value = { open: true, env };
}
function closeDrawer() { drawer.value = { open: false, env: null }; }

async function addEvent(env) {
  const d = draft.value[env.id];
  if (!d?.type) return;
  busy.value = env.id;
  err.value = "";
  try {
    const payload = {};
    const def = eventTypeDef(env.paradigmKey, d.type);
    for (const [field, kind] of Object.entries(def?.payload || {})) {
      const v = d.payload[field];
      if (v === "" || v == null) continue;
      payload[field] = kind === "number" ? Number(v) : v;
    }
    await api(`/tests/${route.params.id}/environments/${env.id}/events`, {
      method: "POST",
      body: { type: d.type, t: d.t === "" ? 0 : Number(d.t), payload },
    });
    // keep the drawer open for rapid entry; reset only the value fields
    draft.value[env.id] = { ...freshDraft(env.paradigmKey), type: d.type };
    await load(); // auto-updates event list, metrics and charts
  } catch (e) { err.value = e.message; } finally { busy.value = ""; }
}

async function removeEvent(envId, index) {
  busy.value = envId;
  err.value = "";
  try {
    await api(`/tests/${route.params.id}/environments/${envId}/events/${index}`, { method: "DELETE" });
    await load();
  } catch (e) { err.value = e.message; } finally { busy.value = ""; }
}

async function remove() {
  if (!confirm(t("tests.confirmDelete"))) return;
  try {
    await api(`/tests/${route.params.id}`, { method: "DELETE" });
    router.push("/tests");
  } catch (e) { err.value = e.message; }
}

const eventLabel = (env, type) => eventTypeDef(env.paradigmKey, type)?.label || type;
const round = (n) => (typeof n === "number" ? Math.round(n * 100) / 100 : n);
// Map a zone key (e.g. "wall_annulus") to its localized label (e.g. "Duvar halkası").
const zoneLabel = (env, zk) => (zonesByEnv.value[env.id] || []).find((z) => z.key === zk)?.label || zk;
// Localize a payload field name (zone/seconds/object/...); raw fallback.
const tField = (f) => (te(`payloadFields.${f}`) ? t(`payloadFields.${f}`) : f);
// Render an event payload with localized field names and zone labels.
const payloadStr = (env, p) => Object.entries(p || {})
  .map(([k, v]) => `${tField(k)}=${k === "zone" ? zoneLabel(env, v) : v}`)
  .join(", ");

// Live derived-metric summary (table).
function summaryFor(env) {
  const m = envEntry(env.id).metrics || {};
  const defs = metricDefs.value[env.paradigmKey] || [];
  const out = [];
  for (const d of defs) {
    const v = m[d.key];
    if (v == null) continue;
    if (d.templated && typeof v === "object") {
      for (const [zk, zv] of Object.entries(v)) out.push({ label: `${d.label} · ${zoneLabel(env, zk)}`, value: round(zv), unit: d.unit });
    } else {
      out.push({ label: d.label, value: typeof v === "number" ? round(v) : v, unit: d.unit });
    }
  }
  return out;
}

// --- Timeline ---
// Logged events as timeline items (EventTimeline orders them by t).
function timelineItems(env) {
  return (envEntry(env.id).events || []).map((ev) => ({
    t: ev.t,
    type: ev.type,
    label: eventLabel(env, ev.type),
    detail: payloadStr(env, ev.payload),
  }));
}

// --- Charts ---
// Metrics that currently carry chartable data (for the parameter picker).
function metricOptions(env) {
  const m = envEntry(env.id).metrics || {};
  return (metricDefs.value[env.paradigmKey] || []).filter((d) => {
    const v = m[d.key];
    return typeof v === "number" || (v && typeof v === "object" && Object.keys(v).length > 0);
  });
}
function paramRows(env) {
  const m = envEntry(env.id).metrics || {};
  const key = selectedMetric.value[env.id];
  const d = (metricDefs.value[env.paradigmKey] || []).find((x) => x.key === key);
  if (!d) return [];
  const v = m[d.key];
  if (typeof v === "number") return [{ label: d.label, value: round(v) }];
  if (v && typeof v === "object") return Object.entries(v).filter(([, zv]) => typeof zv === "number").map(([zk, zv]) => ({ label: zoneLabel(env, zk), value: round(zv) }));
  return [];
}
const paramLabels = (env) => paramRows(env).map((r) => r.label);
const paramValues = (env) => paramRows(env).map((r) => r.value);

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <p class="err" v-if="err">{{ err }}</p>

  <div class="detail" v-if="test">
    <div class="card">
      <div class="detail-head">
        <h2 style="margin-top:0">{{ test.scenario?.name }} <span class="muted">/ {{ test.subject?.code }}</span></h2>
        <div class="head-actions">
          <button @click="router.push('/tests')">{{ $t("common.back") }}</button>
          <button class="danger" @click="remove">{{ $t("common.delete") }}</button>
        </div>
      </div>
      <div class="table-scroll">
        <table class="kv">
          <tbody>
            <tr><th>{{ $t("tests.scenario") }}</th><td>{{ test.scenario?.name }}</td></tr>
            <tr><th>{{ $t("tests.subject") }}</th><td>{{ test.subject?.code }}</td></tr>
            <tr><th>{{ $t("tests.operator") }}</th><td>{{ test.operator?.name }}</td></tr>
            <tr><th>{{ $t("tests.status") }}</th><td><span :class="'status-' + test.status">{{ statusLabel(test.status) }}</span></td></tr>
            <tr v-if="test.notes"><th>{{ $t("common.notes") }}</th><td class="muted">{{ test.notes }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- One run section per scenario environment -->
    <div class="card" v-for="env in environments" :key="env.id">
      <div class="env-head">
        <h4>{{ env.name }} <span class="muted">({{ env.paradigmKey }})</span></h4>
        <div class="env-head-right">
          <button v-if="envEntry(env.id).status === 'RUNNING'" class="primary small" @click="openDrawer(env)">+ {{ $t("results.addEvent") }}</button>
          <span class="pill" :class="'status-' + envEntry(env.id).status">{{ statusLabel(envEntry(env.id).status) }}</span>
        </div>
      </div>

      <!-- PENDING -->
      <div v-if="envEntry(env.id).status === 'PENDING'">
        <p class="muted">{{ $t("results.notStarted") }}</p>
        <button class="primary" :disabled="busy === env.id" @click="startEnv(env.id)">{{ $t("tests.start") }}</button>
      </div>

      <!-- RUNNING / DONE -->
      <template v-else>
        <!-- Metric counter bar (parameter-based metrics, live) -->
        <div class="counter-bar" v-if="summaryFor(env).length">
          <div class="counter" v-for="(row, i) in summaryFor(env)" :key="i">
            <span class="c-val">{{ row.value }}<span class="c-unit" v-if="row.unit"> {{ row.unit }}</span></span>
            <span class="c-label">{{ row.label }}</span>
          </div>
        </div>
        <p v-else class="muted">{{ $t("tests.noResult") }}</p>

        <!-- Tabs: timeline vs charts -->
        <div class="tabs">
          <button class="tab" :class="{ active: tabOf(env.id) === 'timeline' }" @click="setTab(env.id, 'timeline')">{{ $t("results.timeline") }}</button>
          <button class="tab" :class="{ active: tabOf(env.id) === 'charts' }" @click="setTab(env.id, 'charts')">{{ $t("results.charts") }}</button>
        </div>

        <!-- Timeline tab: event-based timeline + editable event list -->
        <div v-show="tabOf(env.id) === 'timeline'" class="tab-panel">
          <div class="cols">
            <div class="col">
              <h5>{{ $t("results.timeline") }}</h5>
              <EventTimeline v-if="envEntry(env.id).events?.length" :items="timelineItems(env)" />
              <p v-else class="muted">{{ $t("results.noEvents") }}</p>
            </div>
            <div class="col">
              <h5>{{ $t("results.events") }}</h5>
              <div class="table-scroll" v-if="envEntry(env.id).events?.length">
                <table class="events">
                  <tbody>
                    <tr v-for="(ev, i) in envEntry(env.id).events" :key="i">
                      <td>{{ eventLabel(env, ev.type) }}</td>
                      <td class="muted">t={{ ev.t }}s</td>
                      <td class="muted">{{ payloadStr(env, ev.payload) }}</td>
                      <td v-if="envEntry(env.id).status === 'RUNNING'"><button class="danger small" @click="removeEvent(env.id, i)">×</button></td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <p v-else class="muted">{{ $t("results.noEvents") }}</p>
            </div>
          </div>
        </div>

        <!-- Charts tab: parameter-based bar chart -->
        <div v-show="tabOf(env.id) === 'charts'" class="tab-panel">
          <div class="charts" v-if="metricOptions(env).length">
            <p class="chart-title">
              {{ $t("results.chartByParam") }}
              <select v-model="selectedMetric[env.id]" class="metric-select">
                <option v-for="d in metricOptions(env)" :key="d.key" :value="d.key">{{ d.label }}</option>
              </select>
            </p>
            <MetricBarChart :labels="paramLabels(env)" :values="paramValues(env)" :label="selectedMetric[env.id]" />
          </div>
          <p v-else class="muted chart-empty">{{ $t("results.noChartData") }}</p>
        </div>

        <div class="actions">
          <button v-if="envEntry(env.id).status === 'RUNNING'" class="primary" :disabled="busy === env.id" @click="finishEnv(env.id)">{{ $t("tests.finish") }}</button>
          <button v-else :disabled="busy === env.id" @click="editEnv(env.id)">{{ $t("results.edit") }}</button>
        </div>
      </template>
    </div>

    <!-- Discussion thread: open to every authenticated user. -->
    <TestComments :test-id="route.params.id" />
  </div>

  <!-- Event-add drawer (right) -->
  <div v-if="drawer.open" class="drawer-overlay" @click="closeDrawer"></div>
  <aside class="drawer" :class="{ open: drawer.open }" v-if="drawer.env">
    <div class="drawer-head">
      <h4>{{ $t("results.addEvent") }} <span class="muted">· {{ drawer.env.name }}</span></h4>
      <button class="icon" @click="closeDrawer">×</button>
    </div>
    <div class="drawer-body">
      <label class="fld">
        <span>{{ $t("results.events") }}</span>
        <select v-model="draft[drawer.env.id].type">
          <option v-for="et in eventTypes[drawer.env.paradigmKey] || []" :key="et.type" :value="et.type">{{ et.label }}</option>
        </select>
      </label>
      <template v-for="(kind, field) in eventTypeDef(drawer.env.paradigmKey, draft[drawer.env.id].type)?.payload || {}" :key="field">
        <label class="fld">
          <span>{{ field === 'zone' ? $t('results.zone') : tField(field) }}</span>
          <select v-if="field === 'zone'" v-model="draft[drawer.env.id].payload.zone">
            <option value="" disabled>{{ $t("results.zone") }}</option>
            <option v-for="z in zonesByEnv[drawer.env.id] || []" :key="z.key" :value="z.key">{{ z.label || z.key }}</option>
          </select>
          <input v-else :type="kind === 'number' ? 'number' : 'text'" v-model="draft[drawer.env.id].payload[field]" :placeholder="tField(field)" />
        </label>
      </template>
      <label class="fld">
        <span>{{ $t("results.tSeconds") }}</span>
        <input type="number" step="0.1" min="0" v-model="draft[drawer.env.id].t" :placeholder="$t('results.tSeconds')" />
      </label>
      <button class="primary block" :disabled="busy === drawer.env.id || !draft[drawer.env.id].type" @click="addEvent(drawer.env)">{{ $t("results.addEventShort") }}</button>
      <p class="muted hint">{{ envEntry(drawer.env.id).events?.length || 0 }} {{ $t("results.events").toLowerCase() }}</p>
    </div>
  </aside>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.detail-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.head-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.env-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; flex-wrap: wrap; }
.env-head h4 { margin: 0; }
.env-head-right { display: flex; align-items: center; gap: 10px; }
.cols { display: flex; gap: 24px; flex-wrap: wrap; }
.col { flex: 1; min-width: 260px; }
.col h5 { margin: 0 0 8px; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.chart-title { margin: 0 0 6px; font-size: 13px; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }

/* Metric counter bar */
.counter-bar { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 16px; }
.counter {
  display: flex; flex-direction: column; gap: 2px;
  min-width: 120px; flex: 1 1 120px;
  background: var(--panel2); border: 1px solid var(--line); border-radius: 11px;
  padding: 10px 12px;
}
.c-val { font-size: 20px; font-weight: 700; line-height: 1.1; font-variant-numeric: tabular-nums; }
.c-unit { font-size: 12px; font-weight: 500; color: var(--muted); }
.c-label { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }

/* Tabs */
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--line); margin-bottom: 16px; }
.tab {
  background: transparent; border: none; border-bottom: 2px solid transparent;
  border-radius: 0; padding: 8px 14px; color: var(--muted); font-size: 13px;
  margin-bottom: -1px;
}
.tab.active { color: var(--txt); border-bottom-color: var(--accent, #4cc2ff); font-weight: 600; }
.tab-panel { min-width: 0; }
.chart-empty { margin-top: 14px; }
.metric-select { width: auto; flex: 0 0 auto; }
.events { width: 100%; border-collapse: collapse; }
.events td { padding: 5px 8px; border-bottom: 1px solid var(--line); }
.kv th { text-align: left; padding-right: 16px; white-space: nowrap; vertical-align: top; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; flex-wrap: wrap; }
.small { padding: 4px 10px; font-size: .85rem; }

/* Drawer */
.drawer-overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.35); z-index: 40; }
.drawer {
  position: fixed; top: 0; right: 0; height: 100vh; width: 340px; max-width: 90vw;
  background: var(--panel); border-left: 1px solid var(--line); z-index: 41;
  display: flex; flex-direction: column; box-shadow: -8px 0 24px rgba(0,0,0,0.2);
}
.drawer-head { display: flex; align-items: center; justify-content: space-between; padding: 14px 16px; border-bottom: 1px solid var(--line); }
.drawer-head h4 { margin: 0; }
.drawer-body { padding: 16px; display: flex; flex-direction: column; gap: 12px; overflow-y: auto; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.fld select, .fld input { width: 100%; }
.block { width: 100%; }
.icon { background: transparent; border: none; font-size: 22px; line-height: 1; cursor: pointer; color: var(--muted); }
.hint { margin: 4px 0 0; font-size: 12px; }
</style>
