<script setup>
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";

const { t, locale } = useI18n();
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
  if (!confirm(t("devices.confirmDelete"))) return;
  await api(`/devices/${id}`, { method: "DELETE" });
  await load();
}
</script>

<template>
  <h1>{{ $t("devices.title") }}</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" :placeholder="$t('devices.namePlaceholder')" /></div>
      <div class="field"><label>{{ $t("devices.platform") }}</label>
        <select v-model="form.platform"><option value="android">Android</option><option value="ios">iOS</option></select>
      </div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.name">{{ $t("common.add") }}</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>{{ $t("common.name") }}</th><th>{{ $t("devices.platform") }}</th><th>{{ $t("common.addedAt") }}</th><th></th></tr></thead>
      <tbody>
        <tr v-for="d in items" :key="d.id">
          <td>{{ d.name }}</td>
          <td class="muted">{{ d.platform }}</td>
          <td class="muted">{{ new Date(d.createdAt).toLocaleDateString(locale) }}</td>
          <td><button class="danger" @click="remove(d.id)">{{ $t("common.delete") }}</button></td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">{{ $t("devices.empty") }}</p>
  </div>
</template>
