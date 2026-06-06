<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import MetricsForm from "../components/MetricsForm.vue";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t, locale } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const test = ref(null);
const err = ref("");
const metricDefs = ref({}); // paradigmKey -> metric defs
const zonesByEnv = ref({}); // envId -> zones
const buffers = ref({}); // envId -> metrics being entered
const busy = ref("");

const environments = computed(() => test.value?.scenario?.environments || []);
const envEntry = (envId) => test.value?.result?.environments?.[envId] || { status: "PENDING" };

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("tests.title"), to: "/tests" },
    { label: test.value ? `${test.value.scenario?.name} / ${test.value.subject?.code}` : route.params.id },
  ]);
}

async function load() {
  err.value = "";
  try {
    test.value = await api(`/tests/${route.params.id}`);
    await Promise.all(environments.value.map(async (env) => {
      if (!metricDefs.value[env.paradigmKey]) {
        metricDefs.value[env.paradigmKey] = await api(`/paradigms/metrics?paradigm=${env.paradigmKey}&lang=${locale.value}`);
      }
      if (!zonesByEnv.value[env.id]) {
        const full = await api(`/environments/${env.id}`);
        zonesByEnv.value[env.id] = full.config?.zones || [];
      }
      buffers.value[env.id] = { ...(envEntry(env.id).metrics || {}) };
    }));
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function submitEnv(envId, body) {
  busy.value = envId;
  err.value = "";
  try {
    await api(`/tests/${route.params.id}/environments/${envId}`, { method: "PATCH", body });
    await load();
  } catch (e) {
    err.value = e.message;
  } finally {
    busy.value = "";
  }
}

const startEnv = (envId) => submitEnv(envId, { status: "RUNNING" });
const finishEnv = (envId) => submitEnv(envId, { status: "DONE", metrics: buffers.value[envId] || {} });
const editEnv = (envId) => submitEnv(envId, { status: "RUNNING" });

async function remove() {
  if (!confirm(t("tests.confirmDelete"))) return;
  try {
    await api(`/tests/${route.params.id}`, { method: "DELETE" });
    router.push("/tests");
  } catch (e) {
    err.value = e.message;
  }
}

function passedLabel(row) {
  if (row.passed === true) return t("tests.passYes");
  if (row.passed === false) return t("tests.passNo");
  return t("tests.passNa");
}
function passedClass(row) {
  if (row.passed === true) return "pass-yes";
  if (row.passed === false) return "pass-no";
  return "pass-na";
}

// Read-only summary of an environment's entered metrics.
function summaryFor(env) {
  const m = envEntry(env.id).metrics || {};
  const defs = metricDefs.value[env.paradigmKey] || [];
  const out = [];
  for (const d of defs) {
    const v = m[d.key];
    if (v == null) continue;
    if (d.templated && typeof v === "object") {
      for (const [zk, zv] of Object.entries(v)) out.push({ label: `${d.label} · ${zk}`, value: zv, unit: d.unit });
    } else {
      out.push({ label: d.label, value: v, unit: d.unit });
    }
  }
  return out;
}

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
          <tr><th>{{ $t("tests.result") }}</th><td><span class="pill" :class="passedClass(test)">{{ passedLabel(test) }}</span></td></tr>
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

      <!-- PENDING: start -->
      <div v-if="envEntry(env.id).status === 'PENDING'">
        <p class="muted">{{ $t("results.notStarted") }}</p>
        <button class="primary" :disabled="busy === env.id" @click="startEnv(env.id)">{{ $t("tests.start") }}</button>
      </div>

      <!-- RUNNING: enter metrics -->
      <div v-else-if="envEntry(env.id).status === 'RUNNING'">
        <MetricsForm
          v-model="buffers[env.id]"
          :metrics="metricDefs[env.paradigmKey] || []"
          :zones="zonesByEnv[env.id] || []"
        />
        <div class="actions">
          <button class="primary" :disabled="busy === env.id" @click="finishEnv(env.id)">{{ $t("tests.finish") }}</button>
        </div>
      </div>

      <!-- DONE: read-only summary -->
      <div v-else>
        <table class="kv">
          <tbody>
            <tr v-for="(row, i) in summaryFor(env)" :key="i"><th>{{ row.label }}</th><td>{{ row.value }} <span class="muted">{{ row.unit }}</span></td></tr>
            <tr v-if="!summaryFor(env).length"><td class="muted">{{ $t("tests.noResult") }}</td></tr>
          </tbody>
        </table>
        <div class="actions">
          <button :disabled="busy === env.id" @click="editEnv(env.id)">{{ $t("results.edit") }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.detail-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.head-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.env-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.env-head h4 { margin: 0; }
.kv th { text-align: left; padding-right: 16px; white-space: nowrap; vertical-align: top; }
.kv td { width: 100%; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
.pill.pass-yes { background: #1f7a3d; color: #fff; }
.pill.pass-no { background: #a3271f; color: #fff; }
.pill.pass-na { background: var(--active-bg); color: var(--muted); }
</style>
