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
const form = ref({ code: "", sex: "M", groupName: "", notes: "" });
const err = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("subjects.title"), to: "/subjects" },
    { label: isNew.value ? t("common.new") : form.value.code || route.params.id },
  ]);
}

async function load() {
  err.value = "";
  if (isNew.value) {
    syncCrumb();
    return;
  }
  try {
    const s = await api(`/subjects/${route.params.id}`);
    form.value = { code: s.code, sex: s.sex, groupName: s.groupName || "", notes: s.notes || "" };
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
      await api("/subjects", { method: "POST", body: { ...form.value } });
    } else {
      await api(`/subjects/${route.params.id}`, { method: "PATCH", body: { ...form.value } });
    }
    router.push("/subjects");
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!confirm(t("subjects.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/subjects/${route.params.id}`, { method: "DELETE" });
    router.push("/subjects");
  } catch (e) {
    err.value = e.message;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <div class="card">
    <h2 style="margin-top:0">{{ isNew ? $t("common.new") : form.code }}</h2>
    <div class="row">
      <div class="field"><label>{{ $t("subjects.code") }}</label><input v-model="form.code" :placeholder="$t('subjects.codePlaceholder')" /></div>
      <div class="field"><label>{{ $t("subjects.sex") }}</label>
        <select v-model="form.sex"><option value="M">{{ $t("subjects.male") }}</option><option value="F">{{ $t("subjects.female") }}</option></select>
      </div>
      <div class="field"><label>{{ $t("subjects.group") }}</label><input v-model="form.groupName" :placeholder="$t('subjects.groupPlaceholder')" /></div>
      <div class="field" style="flex:2"><label>{{ $t("common.notes") }}</label><input v-model="form.notes" /></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/subjects')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.code" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
</style>
