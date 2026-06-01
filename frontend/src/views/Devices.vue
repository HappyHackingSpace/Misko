<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api.js";

const items = ref([]);
const form = ref({ name: "", platform: "android" });
const err = ref("");

async function load() { items.value = await api("/devices"); }
onMounted(load);

async function create() {
  err.value = "";
  try {
    await api("/devices", { method: "POST", body: { ...form.value } });
    form.value = { name: "", platform: "android" };
    await load();
  } catch (e) { err.value = e.message; }
}
async function remove(id) {
  if (!confirm("Cihaz silinsin mi?")) return;
  await api(`/devices/${id}`, { method: "DELETE" });
  await load();
}
</script>

<template>
  <h1>Test Cihazları</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>Ad</label><input v-model="form.name" placeholder="Telefon-1" /></div>
      <div class="field"><label>Platform</label>
        <select v-model="form.platform"><option value="android">Android</option><option value="ios">iOS</option></select>
      </div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.name">Ekle</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>Ad</th><th>Platform</th><th>Eklendi</th><th></th></tr></thead>
      <tbody>
        <tr v-for="d in items" :key="d.id">
          <td>{{ d.name }}</td>
          <td class="muted">{{ d.platform }}</td>
          <td class="muted">{{ new Date(d.createdAt).toLocaleDateString("tr-TR") }}</td>
          <td><button class="danger" @click="remove(d.id)">Sil</button></td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">Cihaz yok.</p>
  </div>
</template>
