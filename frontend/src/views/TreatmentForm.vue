<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const ROUTES = ["IP", "ORAL", "SC", "IV", "IN"];

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const isNew = computed(() => !route.params.id);
const form = ref({ key: "", name: "", defaultDose: "", unit: "", route: "", notes: "" });
const err = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("treatments.title"), to: "/treatments" },
    { label: isNew.value ? t("common.new") : form.value.key || route.params.id },
  ]);
}

async function load() {
  err.value = "";
  if (isNew.value) {
    syncCrumb();
    return;
  }
  try {
    const d = await api(`/treatments/${route.params.id}`);
    form.value = {
      key: d.key, name: d.name,
      defaultDose: d.defaultDose ?? "", unit: d.unit || "", route: d.route || "", notes: d.notes || "",
    };
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function save() {
  err.value = "";
  saving.value = true;
  try {
    if (isNew.value) {
      await api("/treatments", { method: "POST", body: { ...form.value } });
    } else {
      await api(`/treatments/${route.params.id}`, { method: "PATCH", body: { ...form.value } });
    }
    router.push("/treatments");
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!confirm(t("treatments.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/treatments/${route.params.id}`, { method: "DELETE" });
    router.push("/treatments");
  } catch (e) {
    err.value = e.message;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <div class="card">
    <h2 style="margin-top:0">{{ isNew ? $t("common.new") : form.key }}</h2>
    <div class="row">
      <div class="field"><label>{{ $t("treatments.key") }}</label><input v-model="form.key" placeholder="donepezil" /></div>
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" /></div>
    </div>
    <div class="row">
      <div class="field"><label>{{ $t("treatments.defaultDose") }}</label><input type="number" step="0.01" min="0" v-model="form.defaultDose" /></div>
      <div class="field"><label>{{ $t("treatments.unit") }}</label><input v-model="form.unit" placeholder="mg_kg" /></div>
      <div class="field"><label>{{ $t("treatments.route") }}</label>
        <select v-model="form.route">
          <option value="">{{ $t("treatments.none") }}</option>
          <option v-for="r in ROUTES" :key="r" :value="r">{{ r }}</option>
        </select>
      </div>
    </div>
    <div class="row">
      <div class="field" style="flex:1"><label>{{ $t("common.notes") }}</label><input v-model="form.notes" /></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/treatments')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.key || !form.name" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
</style>
