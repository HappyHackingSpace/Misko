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

const TYPES = ["POOL", "MAZE", "STICK", "PATH"];
const isNew = computed(() => !route.params.id);
const form = ref({ name: "", type: "POOL", description: "" });
const err = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("scenarios.title"), to: "/scenarios" },
    { label: isNew.value ? t("common.new") : form.value.name || route.params.id },
  ]);
}

async function load() {
  err.value = "";
  if (isNew.value) {
    syncCrumb();
    return;
  }
  try {
    const s = await api(`/scenarios/${route.params.id}`);
    form.value = { name: s.name, type: s.type, description: s.description || "" };
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
      await api("/scenarios", { method: "POST", body: { ...form.value } });
    } else {
      await api(`/scenarios/${route.params.id}`, { method: "PATCH", body: { ...form.value } });
    }
    router.push("/scenarios");
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!confirm(t("scenarios.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/scenarios/${route.params.id}`, { method: "DELETE" });
    router.push("/scenarios");
  } catch (e) {
    err.value = e.message;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <div class="card">
    <h2 style="margin-top:0">{{ isNew ? $t("common.new") : form.name }}</h2>
    <div class="row">
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" :placeholder="$t('scenarios.namePlaceholder')" /></div>
      <div class="field"><label>{{ $t("scenarios.type") }}</label>
        <select v-model="form.type"><option v-for="ty in TYPES" :key="ty" :value="ty">{{ $t("scenarios.types." + ty) }}</option></select>
      </div>
      <div class="field" style="flex:2"><label>{{ $t("common.description") }}</label><input v-model="form.description" /></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/scenarios')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.name" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
</style>
