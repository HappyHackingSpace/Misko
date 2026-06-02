<script setup>
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";

const { t } = useI18n();
const items = ref([]);
const form = ref({ name: "", type: "POOL", description: "" });
const err = ref("");
const TYPES = ["POOL", "MAZE", "STICK", "PATH"];

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
  if (!confirm(t("scenarios.confirmDelete"))) return;
  await api(`/scenarios/${id}`, { method: "DELETE" });
  await load();
}
</script>

<template>
  <h1>{{ $t("scenarios.title") }}</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" :placeholder="$t('scenarios.namePlaceholder')" /></div>
      <div class="field"><label>{{ $t("scenarios.type") }}</label>
        <select v-model="form.type"><option v-for="ty in TYPES" :key="ty" :value="ty">{{ $t("scenarios.types." + ty) }}</option></select>
      </div>
      <div class="field" style="flex:2"><label>{{ $t("common.description") }}</label><input v-model="form.description" /></div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.name">{{ $t("common.add") }}</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>{{ $t("common.name") }}</th><th>{{ $t("scenarios.type") }}</th><th>{{ $t("common.description") }}</th><th></th></tr></thead>
      <tbody>
        <tr v-for="s in items" :key="s.id">
          <td>{{ s.name }}</td>
          <td><span class="pill" :class="s.type">{{ $t("scenarios.types." + s.type) }}</span></td>
          <td class="muted">{{ s.description }}</td>
          <td><button class="danger" @click="remove(s.id)">{{ $t("common.delete") }}</button></td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">{{ $t("scenarios.empty") }}</p>
  </div>
</template>
