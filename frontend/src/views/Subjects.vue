<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api.js";

const items = ref([]);
const form = ref({ code: "", sex: "M", groupName: "", notes: "" });
const err = ref("");

async function load() { items.value = await api("/subjects"); }
onMounted(load);

async function create() {
  err.value = "";
  try {
    await api("/subjects", { method: "POST", body: { ...form.value } });
    form.value = { code: "", sex: "M", groupName: "", notes: "" };
    await load();
  } catch (e) { err.value = e.message; }
}
async function remove(id) {
  if (!confirm("Denek silinsin mi?")) return;
  await api(`/subjects/${id}`, { method: "DELETE" });
  await load();
}
</script>

<template>
  <h1>Denekler (Fareler)</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>Kod</label><input v-model="form.code" placeholder="F-001" /></div>
      <div class="field"><label>Cinsiyet</label>
        <select v-model="form.sex"><option value="M">Erkek</option><option value="F">Dişi</option></select>
      </div>
      <div class="field"><label>Grup</label><input v-model="form.groupName" placeholder="kontrol" /></div>
      <div class="field" style="flex:2"><label>Not</label><input v-model="form.notes" /></div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.code">Ekle</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>Kod</th><th>Cinsiyet</th><th>Grup</th><th>Not</th><th></th></tr></thead>
      <tbody>
        <tr v-for="s in items" :key="s.id">
          <td>{{ s.code }}</td>
          <td>{{ s.sex === "F" ? "Dişi" : "Erkek" }}</td>
          <td class="muted">{{ s.groupName }}</td>
          <td class="muted">{{ s.notes }}</td>
          <td><button class="danger" @click="remove(s.id)">Sil</button></td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">Denek yok.</p>
  </div>
</template>
