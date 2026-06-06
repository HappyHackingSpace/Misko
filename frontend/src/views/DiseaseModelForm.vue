<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const isNew = computed(() => !route.params.id);
const form = ref({ key: "", name: "", category: "", description: "" });
const err = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("diseaseModels.title"), to: "/disease-models" },
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
    const d = await api(`/disease-models/${route.params.id}`);
    form.value = { key: d.key, name: d.name, category: d.category || "", description: d.description || "" };
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
      await api("/disease-models", { method: "POST", body: { ...form.value } });
    } else {
      await api(`/disease-models/${route.params.id}`, { method: "PATCH", body: { ...form.value } });
    }
    router.push("/disease-models");
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!confirm(t("diseaseModels.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/disease-models/${route.params.id}`, { method: "DELETE" });
    router.push("/disease-models");
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
      <div class="field"><label>{{ $t("diseaseModels.key") }}</label><input v-model="form.key" placeholder="5xFAD" /></div>
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" /></div>
      <div class="field"><label>{{ $t("diseaseModels.category") }}</label><input v-model="form.category" placeholder="alzheimer" /></div>
    </div>
    <div class="row">
      <div class="field" style="flex:1"><label>{{ $t("common.notes") }}</label><input v-model="form.description" /></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/disease-models')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.key || !form.name" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
</style>
