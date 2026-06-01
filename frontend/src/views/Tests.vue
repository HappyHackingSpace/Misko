<script setup>
import { ref, onMounted, computed } from "vue";
import { api } from "../api.js";

const items = ref([]);
const scenarios = ref([]);
const subjects = ref([]);
const devices = ref([]);
const form = ref({ scenarioId: "", subjectId: "", deviceId: "", notes: "" });
const err = ref("");

const canCreate = computed(() => form.value.scenarioId && form.value.subjectId);

async function load() {
  [items.value, scenarios.value, subjects.value, devices.value] = await Promise.all([
    api("/tests"), api("/scenarios"), api("/subjects"), api("/devices"),
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
  if (!confirm("Test silinsin mi?")) return;
  await api(`/tests/${id}`, { method: "DELETE" });
  await load();
}
</script>

<template>
  <h1>Testler</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>Senaryo</label>
        <select v-model="form.scenarioId">
          <option value="" disabled>Seç…</option>
          <option v-for="s in scenarios" :key="s.id" :value="s.id">{{ s.name }} ({{ s.type }})</option>
        </select>
      </div>
      <div class="field"><label>Denek</label>
        <select v-model="form.subjectId">
          <option value="" disabled>Seç…</option>
          <option v-for="s in subjects" :key="s.id" :value="s.id">{{ s.code }}</option>
        </select>
      </div>
      <div class="field"><label>Cihaz</label>
        <select v-model="form.deviceId">
          <option value="">(yok)</option>
          <option v-for="d in devices" :key="d.id" :value="d.id">{{ d.name }}</option>
        </select>
      </div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!canCreate">Test oluştur</button></div>
    </div>
    <p class="muted" v-if="!scenarios.length || !subjects.length">Önce en az bir senaryo ve bir denek ekle.</p>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>Senaryo</th><th>Denek</th><th>Operatör</th><th>Cihaz</th><th>Durum</th><th>İşlem</th></tr></thead>
      <tbody>
        <tr v-for="t in items" :key="t.id">
          <td>{{ t.scenario?.name }}</td>
          <td>{{ t.subject?.code }}</td>
          <td class="muted">{{ t.operator?.name }}</td>
          <td class="muted">{{ t.device?.name || "—" }}</td>
          <td :class="'status-' + t.status">{{ t.status }}</td>
          <td>
            <div class="row">
              <button v-if="t.status === 'PENDING'" @click="setStatus(t, 'RUNNING')">Başlat</button>
              <button v-if="t.status === 'RUNNING'" @click="setStatus(t, 'DONE')">Bitir</button>
              <button v-if="t.status === 'RUNNING'" class="danger" @click="setStatus(t, 'FAILED')">İptal</button>
              <button class="danger" @click="remove(t.id)">Sil</button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">Test yok.</p>
  </div>
</template>
