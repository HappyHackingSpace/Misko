<script setup>
import { ref, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import AcceptanceEditor from "../components/AcceptanceEditor.vue";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t, locale } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const test = ref(null);
const paradigms = ref([]);
const operators = ref([]);
const editBuffer = ref([]);
const err = ref("");
const editErr = ref("");

function parseCriteria(row) {
  if (!row?.acceptanceCriteria) return [];
  try { return JSON.parse(row.acceptanceCriteria); } catch { return []; }
}

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
    editBuffer.value = parseCriteria(test.value);
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function loadDropdowns() {
  try {
    const [pa, op] = await Promise.all([
      api(`/paradigms?all=true&lang=${locale.value}`),
      api("/paradigms/acceptance-operators"),
    ]);
    paradigms.value = pa.data;
    operators.value = op;
  } catch (e) {
    err.value = e.message;
  }
}

async function setStatus(status) {
  err.value = "";
  try {
    const body = { status };
    if (status === "RUNNING") body.startedAt = new Date().toISOString();
    if (status === "DONE" || status === "FAILED") body.endedAt = new Date().toISOString();
    await api(`/tests/${route.params.id}`, { method: "PATCH", body });
    await load();
  } catch (e) {
    err.value = e.message;
  }
}

async function saveCriteria() {
  editErr.value = "";
  try {
    await api(`/tests/${route.params.id}`, { method: "PATCH", body: { acceptanceCriteria: editBuffer.value } });
    await load();
  } catch (e) { editErr.value = e.message; }
}

async function remove() {
  if (!confirm(t("tests.confirmDelete"))) return;
  err.value = "";
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

onMounted(async () => {
  await Promise.all([load(), loadDropdowns()]);
});
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
          <button v-if="test.status === 'PENDING'" @click="setStatus('RUNNING')">{{ $t("tests.start") }}</button>
          <button v-if="test.status === 'RUNNING'" @click="setStatus('DONE')">{{ $t("tests.finish") }}</button>
          <button v-if="test.status === 'RUNNING'" class="danger" @click="setStatus('FAILED')">{{ $t("tests.cancel") }}</button>
          <button class="danger" @click="remove">{{ $t("common.delete") }}</button>
        </div>
      </div>
      <table class="kv">
        <tbody>
          <tr><th>{{ $t("tests.scenario") }}</th><td>{{ test.scenario?.name }}</td></tr>
          <tr><th>{{ $t("tests.subject") }}</th><td>{{ test.subject?.code }}</td></tr>
          <tr><th>{{ $t("tests.operator") }}</th><td>{{ test.operator?.name }}</td></tr>
          <tr><th>{{ $t("tests.device") }}</th><td>{{ test.device?.name || "-" }}</td></tr>
          <tr><th>{{ $t("tests.status") }}</th><td><span :class="'status-' + test.status">{{ test.status }}</span></td></tr>
          <tr><th>{{ $t("tests.result") }}</th><td><span class="pill" :class="passedClass(test)">{{ passedLabel(test) }}</span></td></tr>
          <tr v-if="test.notes"><th>{{ $t("common.notes") }}</th><td class="muted">{{ test.notes }}</td></tr>
        </tbody>
      </table>
    </div>

    <div class="card">
      <h4>{{ $t("acceptance.title") }}</h4>
      <AcceptanceEditor v-model="editBuffer" :paradigms="paradigms" :operators="operators" />
      <p class="err" v-if="editErr">{{ editErr }}</p>
      <div class="actions">
        <button class="primary" @click="saveCriteria">{{ $t("common.save") }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.detail-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.head-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.kv th { text-align: left; padding-right: 16px; white-space: nowrap; vertical-align: top; }
.kv td { width: 100%; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
.pill.pass-yes { background: #1f7a3d; color: #fff; }
.pill.pass-no { background: #a3271f; color: #fff; }
.pill.pass-na { background: var(--active-bg); color: var(--muted); }
h4 { margin: 0 0 10px; }
</style>
