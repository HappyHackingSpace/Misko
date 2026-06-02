<script setup>
import { ref, onMounted, computed } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import AcceptanceEditor from "../components/AcceptanceEditor.vue";

const { t, locale } = useI18n();

const items = ref([]);
const scenarios = ref([]);
const subjects = ref([]);
const devices = ref([]);
const paradigms = ref([]);
const operators = ref([]);
const form = ref({ scenarioId: "", subjectId: "", deviceId: "", notes: "" });
const err = ref("");

// Acceptance criteria editor state (per test)
const editingId = ref(null);
const editBuffer = ref([]);
const editErr = ref("");

const canCreate = computed(() => form.value.scenarioId && form.value.subjectId);

async function load() {
  [items.value, scenarios.value, subjects.value, devices.value, paradigms.value, operators.value] =
    await Promise.all([
      api("/tests"), api("/scenarios"), api("/subjects"), api("/devices"),
      api(`/paradigms?lang=${locale.value}`), api("/paradigms/acceptance-operators"),
    ]);
}
onMounted(load);

async function create() {
  err.value = "";
  try {
    await api("/tests", { method: "POST", body: { ...form.value } });
    form.value = { scenarioId: "", subjectId: "", deviceId: "", notes: "" };
    await load();
  } catch (e) { err.value = e.message; }
}

async function setStatus(t, status) {
  const body = { status };
  if (status === "RUNNING") body.startedAt = new Date().toISOString();
  if (status === "DONE" || status === "FAILED") body.endedAt = new Date().toISOString();
  await api(`/tests/${t.id}`, { method: "PATCH", body });
  await load();
}
async function remove(id) {
  if (!confirm(t("tests.confirmDelete"))) return;
  await api(`/tests/${id}`, { method: "DELETE" });
  await load();
}

function parseCriteria(row) {
  if (!row.acceptanceCriteria) return [];
  try { return JSON.parse(row.acceptanceCriteria); } catch { return []; }
}
function toggleEdit(row) {
  if (editingId.value === row.id) { editingId.value = null; return; }
  editingId.value = row.id;
  editBuffer.value = parseCriteria(row);
  editErr.value = "";
}
async function saveCriteria(id) {
  editErr.value = "";
  try {
    await api(`/tests/${id}`, { method: "PATCH", body: { acceptanceCriteria: editBuffer.value } });
    editingId.value = null;
    await load();
  } catch (e) { editErr.value = e.message; }
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
function criteriaCount(row) {
  return parseCriteria(row).length;
}
</script>

<template>
  <h1>{{ $t("tests.title") }}</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>{{ $t("tests.scenario") }}</label>
        <select v-model="form.scenarioId">
          <option value="" disabled>{{ $t("common.select") }}</option>
          <option v-for="s in scenarios" :key="s.id" :value="s.id">{{ s.name }} ({{ s.type }})</option>
        </select>
      </div>
      <div class="field"><label>{{ $t("tests.subject") }}</label>
        <select v-model="form.subjectId">
          <option value="" disabled>{{ $t("common.select") }}</option>
          <option v-for="s in subjects" :key="s.id" :value="s.id">{{ s.code }}</option>
        </select>
      </div>
      <div class="field"><label>{{ $t("tests.device") }}</label>
        <select v-model="form.deviceId">
          <option value="">{{ $t("common.none") }}</option>
          <option v-for="d in devices" :key="d.id" :value="d.id">{{ d.name }}</option>
        </select>
      </div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!canCreate">{{ $t("tests.createTest") }}</button></div>
    </div>
    <p class="muted" v-if="!scenarios.length || !subjects.length">{{ $t("tests.needFirst") }}</p>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>{{ $t("tests.scenario") }}</th><th>{{ $t("tests.subject") }}</th><th>{{ $t("tests.operator") }}</th><th>{{ $t("tests.device") }}</th><th>{{ $t("tests.status") }}</th><th>{{ $t("tests.result") }}</th><th>{{ $t("common.actions") }}</th></tr></thead>
      <tbody>
        <template v-for="t in items" :key="t.id">
          <tr>
            <td>{{ t.scenario?.name }}</td>
            <td>{{ t.subject?.code }}</td>
            <td class="muted">{{ t.operator?.name }}</td>
            <td class="muted">{{ t.device?.name || "-" }}</td>
            <td :class="'status-' + t.status">{{ t.status }}</td>
            <td>
              <span class="pill" :class="passedClass(t)">{{ passedLabel(t) }}</span>
              <span class="muted" v-if="criteriaCount(t)"> · {{ criteriaCount(t) }} {{ $t("acceptance.criteriaShort") }}</span>
            </td>
            <td>
              <div class="row">
                <button v-if="t.status === 'PENDING'" @click="setStatus(t, 'RUNNING')">{{ $t("tests.start") }}</button>
                <button v-if="t.status === 'RUNNING'" @click="setStatus(t, 'DONE')">{{ $t("tests.finish") }}</button>
                <button v-if="t.status === 'RUNNING'" class="danger" @click="setStatus(t, 'FAILED')">{{ $t("tests.cancel") }}</button>
                <button @click="toggleEdit(t)">{{ $t("acceptance.edit") }}</button>
                <button class="danger" @click="remove(t.id)">{{ $t("common.delete") }}</button>
              </div>
            </td>
          </tr>
          <tr v-if="editingId === t.id">
            <td colspan="7">
              <div class="editor-wrap">
                <h4>{{ $t("acceptance.title") }}</h4>
                <AcceptanceEditor
                  v-model="editBuffer"
                  :paradigms="paradigms"
                  :operators="operators"
                />
                <p class="err" v-if="editErr">{{ editErr }}</p>
                <div class="row">
                  <button class="primary" @click="saveCriteria(t.id)">{{ $t("common.save") }}</button>
                  <button @click="editingId = null">{{ $t("common.cancel") }}</button>
                </div>
              </div>
            </td>
          </tr>
        </template>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">{{ $t("tests.empty") }}</p>
  </div>
</template>

<style scoped>
.pill.pass-yes { background: #1f7a3d; color: #fff; }
.pill.pass-no { background: #a3271f; color: #fff; }
.pill.pass-na { background: var(--active-bg); color: var(--muted); }
.editor-wrap { padding: 8px 4px; display: flex; flex-direction: column; gap: 10px; }
.editor-wrap h4 { margin: 0; }
</style>
