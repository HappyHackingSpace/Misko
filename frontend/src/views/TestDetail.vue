<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t, locale } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const test = ref(null);
const err = ref("");
const metricDefs = ref({}); // paradigmKey -> metric defs
const eventTypes = ref({}); // paradigmKey -> event types
const zonesByEnv = ref({}); // envId -> zones
const draft = ref({}); // envId -> { type, t, payload }
const busy = ref("");

const environments = computed(() => test.value?.scenario?.environments || []);
const envEntry = (envId) => test.value?.result?.environments?.[envId] || { status: "PENDING", events: [], metrics: {} };

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
    draft.value[env.id] = freshDraft(env.paradigmKey);
    await load();
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
const payloadStr = (p) => Object.entries(p || {}).map(([k, v]) => `${k}=${v}`).join(", ");

// Live derived-metric summary for an environment.
function summaryFor(env) {
  const m = envEntry(env.id).metrics || {};
  const defs = metricDefs.value[env.paradigmKey] || [];
  const out = [];
  for (const d of defs) {
    const v = m[d.key];
    if (v == null) continue;
    if (d.templated && typeof v === "object") {
      for (const [zk, zv] of Object.entries(v)) out.push({ label: `${d.label} · ${zk}`, value: round(zv), unit: d.unit });
    } else {
      out.push({ label: d.label, value: typeof v === "number" ? round(v) : v, unit: d.unit });
    }
  }
  return out;
}
const round = (n) => (typeof n === "number" ? Math.round(n * 100) / 100 : n);

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
      <table class="kv">
        <tbody>
          <tr><th>{{ $t("tests.scenario") }}</th><td>{{ test.scenario?.name }}</td></tr>
          <tr><th>{{ $t("tests.subject") }}</th><td>{{ test.subject?.code }}</td></tr>
          <tr><th>{{ $t("tests.operator") }}</th><td>{{ test.operator?.name }}</td></tr>
          <tr><th>{{ $t("tests.status") }}</th><td><span :class="'status-' + test.status">{{ test.status }}</span></td></tr>
          <tr v-if="test.notes"><th>{{ $t("common.notes") }}</th><td class="muted">{{ test.notes }}</td></tr>
        </tbody>
      </table>
    </div>

    <!-- One run section per scenario environment -->
    <div class="card" v-for="env in environments" :key="env.id">
      <div class="env-head">
        <h4>{{ env.name }} <span class="muted">({{ env.paradigmKey }})</span></h4>
        <span class="pill" :class="'status-' + envEntry(env.id).status">{{ envEntry(env.id).status }}</span>
      </div>

      <!-- PENDING -->
      <div v-if="envEntry(env.id).status === 'PENDING'">
        <p class="muted">{{ $t("results.notStarted") }}</p>
        <button class="primary" :disabled="busy === env.id" @click="startEnv(env.id)">{{ $t("tests.start") }}</button>
      </div>

      <!-- RUNNING: log events; metrics derive live -->
      <template v-else>
        <div class="cols">
          <div class="col">
            <h5>{{ $t("results.events") }}</h5>
            <div v-if="envEntry(env.id).status === 'RUNNING'" class="event-add">
              <select v-model="draft[env.id].type">
                <option v-for="et in eventTypes[env.paradigmKey] || []" :key="et.type" :value="et.type">{{ et.label }}</option>
              </select>
              <template v-for="(kind, field) in eventTypeDef(env.paradigmKey, draft[env.id].type)?.payload || {}" :key="field">
                <select v-if="field === 'zone'" v-model="draft[env.id].payload.zone">
                  <option value="" disabled>{{ $t("results.zone") }}</option>
                  <option v-for="z in zonesByEnv[env.id] || []" :key="z.key" :value="z.key">{{ z.label || z.key }}</option>
                </select>
                <input v-else :type="kind === 'number' ? 'number' : 'text'" v-model="draft[env.id].payload[field]" :placeholder="field" />
              </template>
              <input type="number" step="0.1" min="0" v-model="draft[env.id].t" :placeholder="$t('results.tSeconds')" class="t-input" />
              <button class="primary small" :disabled="busy === env.id || !draft[env.id].type" @click="addEvent(env)">{{ $t("results.addEvent") }}</button>
            </div>

            <table class="events" v-if="envEntry(env.id).events?.length">
              <tbody>
                <tr v-for="(ev, i) in envEntry(env.id).events" :key="i">
                  <td>{{ eventLabel(env, ev.type) }}</td>
                  <td class="muted">t={{ ev.t }}s</td>
                  <td class="muted">{{ payloadStr(ev.payload) }}</td>
                  <td v-if="envEntry(env.id).status === 'RUNNING'"><button class="danger small" @click="removeEvent(env.id, i)">×</button></td>
                </tr>
              </tbody>
            </table>
            <p v-else class="muted">{{ $t("results.noEvents") }}</p>
          </div>

          <div class="col">
            <h5>{{ $t("results.liveMetrics") }}</h5>
            <table class="kv" v-if="summaryFor(env).length">
              <tbody>
                <tr v-for="(row, i) in summaryFor(env)" :key="i"><th>{{ row.label }}</th><td>{{ row.value }} <span class="muted">{{ row.unit }}</span></td></tr>
              </tbody>
            </table>
            <p v-else class="muted">{{ $t("tests.noResult") }}</p>
          </div>
        </div>

        <div class="actions">
          <button v-if="envEntry(env.id).status === 'RUNNING'" class="primary" :disabled="busy === env.id" @click="finishEnv(env.id)">{{ $t("tests.finish") }}</button>
          <button v-else :disabled="busy === env.id" @click="editEnv(env.id)">{{ $t("results.edit") }}</button>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.detail-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.head-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.env-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.env-head h4 { margin: 0; }
.cols { display: flex; gap: 24px; flex-wrap: wrap; }
.col { flex: 1; min-width: 260px; }
.col h5 { margin: 0 0 8px; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.event-add { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; margin-bottom: 10px; }
.event-add select, .event-add input { width: auto; flex: 0 0 auto; }
.event-add .t-input { width: 90px; }
.events { width: 100%; border-collapse: collapse; }
.events td { padding: 5px 8px; border-bottom: 1px solid var(--line); }
.kv th { text-align: left; padding-right: 16px; white-space: nowrap; vertical-align: top; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
.small { padding: 4px 10px; font-size: .85rem; }
</style>
