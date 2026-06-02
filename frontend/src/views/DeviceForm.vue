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
const form = ref({ name: "", platform: "android" });
const err = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("devices.title"), to: "/devices" },
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
    const d = await api(`/devices/${route.params.id}`);
    form.value = { name: d.name, platform: d.platform };
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
      await api("/devices", { method: "POST", body: { ...form.value } });
    } else {
      await api(`/devices/${route.params.id}`, { method: "PATCH", body: { ...form.value } });
    }
    router.push("/devices");
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!confirm(t("devices.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/devices/${route.params.id}`, { method: "DELETE" });
    router.push("/devices");
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
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" :placeholder="$t('devices.namePlaceholder')" /></div>
      <div class="field"><label>{{ $t("devices.platform") }}</label>
        <select v-model="form.platform"><option value="android">Android</option><option value="ios">iOS</option></select>
      </div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/devices')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.name" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
</style>
