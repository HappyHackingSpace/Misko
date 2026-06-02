<script setup>
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";

const { t } = useI18n();
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
  if (!confirm(t("subjects.confirmDelete"))) return;
  await api(`/subjects/${id}`, { method: "DELETE" });
  await load();
}
</script>

<template>
  <h1>{{ $t("subjects.title") }}</h1>
  <div class="card">
    <div class="row">
      <div class="field"><label>{{ $t("subjects.code") }}</label><input v-model="form.code" :placeholder="$t('subjects.codePlaceholder')" /></div>
      <div class="field"><label>{{ $t("subjects.sex") }}</label>
        <select v-model="form.sex"><option value="M">{{ $t("subjects.male") }}</option><option value="F">{{ $t("subjects.female") }}</option></select>
      </div>
      <div class="field"><label>{{ $t("subjects.group") }}</label><input v-model="form.groupName" :placeholder="$t('subjects.groupPlaceholder')" /></div>
      <div class="field" style="flex:2"><label>{{ $t("common.notes") }}</label><input v-model="form.notes" /></div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.code">{{ $t("common.add") }}</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>{{ $t("subjects.code") }}</th><th>{{ $t("subjects.sex") }}</th><th>{{ $t("subjects.group") }}</th><th>{{ $t("common.notes") }}</th><th></th></tr></thead>
      <tbody>
        <tr v-for="s in items" :key="s.id">
          <td>{{ s.code }}</td>
          <td>{{ s.sex === "F" ? $t("subjects.female") : $t("subjects.male") }}</td>
          <td class="muted">{{ s.groupName }}</td>
          <td class="muted">{{ s.notes }}</td>
          <td><button class="danger" @click="remove(s.id)">{{ $t("common.delete") }}</button></td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">{{ $t("subjects.empty") }}</p>
  </div>
</template>
