<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api.js";

const items = ref([]);
const form = ref({ name: "", type: "POOL", description: "" });
const err = ref("");
const TYPES = [
  { v: "POOL", l: "Havuz" }, { v: "MAZE", l: "Labirent" },
  { v: "STICK", l: "Sopa" }, { v: "PATH", l: "Yol" },
];

async function load() { items.value = await api("/scenarios"); }
onMounted(load);

async function create() {
  err.value = "";
  try {
    await api("/scenarios", { method: "POST", body: { ...form.value } });
    form.value = { name: "", type: "POOL", description: "" };
    await load();
  } catch (e) { err.value = e.message; }
}
async function remove(id) {
  if (!confirm("Senaryo silinsin mi?")) return;
  await api(`/scenarios/${id}`, { method: "DELETE" });
  await load();
}
</script>

<template>
  <h1>Senaryolar</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>Ad</label><input v-model="form.name" placeholder="Havuz" /></div>
      <div class="field"><label>Tür</label>
        <select v-model="form.type"><option v-for="t in TYPES" :key="t.v" :value="t.v">{{ t.l }}</option></select>
      </div>
      <div class="field" style="flex:2"><label>Açıklama</label><input v-model="form.description" /></div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.name">Ekle</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>Ad</th><th>Tür</th><th>Açıklama</th><th></th></tr></thead>
      <tbody>
        <tr v-for="s in items" :key="s.id">
          <td>{{ s.name }}</td>
          <td><span class="pill" :class="s.type">{{ s.type }}</span></td>
          <td class="muted">{{ s.description }}</td>
          <td><button class="danger" @click="remove(s.id)">Sil</button></td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">Senaryo yok.</p>
  </div>
</template>
