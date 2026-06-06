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
const form = ref({ name: "", description: "" });
const environments = ref([]); // catalog of all environments
const selectedEnvIds = ref([]); // chosen environment ids
const err = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("scenarios.title"), to: "/scenarios" },
    { label: isNew.value ? t("common.new") : form.value.name || route.params.id },
  ]);
}

function toggleEnv(id) {
  const i = selectedEnvIds.value.indexOf(id);
  if (i === -1) selectedEnvIds.value.push(id);
  else selectedEnvIds.value.splice(i, 1);
}

async function load() {
  err.value = "";
  try {
    const envs = await api("/environments?all=true");
    environments.value = envs.data;
    if (!isNew.value) {
      const s = await api(`/scenarios/${route.params.id}`);
      form.value = { name: s.name, description: s.description || "" };
      selectedEnvIds.value = (s.environments || []).map((e) => e.id);
    }
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function save() {
  err.value = "";
  saving.value = true;
  try {
    const body = {
      name: form.value.name,
      description: form.value.description,
      environmentIds: selectedEnvIds.value,
    };
    if (isNew.value) {
      await api("/scenarios", { method: "POST", body });
    } else {
      await api(`/scenarios/${route.params.id}`, { method: "PATCH", body });
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
      <div class="field" style="flex:2"><label>{{ $t("common.description") }}</label><input v-model="form.description" /></div>
    </div>

    <h3 class="section">{{ $t("scenarios.environments") }}</h3>
    <p class="muted" v-if="!environments.length">{{ $t("scenarios.noEnvironments") }}</p>
    <div class="env-list" v-else>
      <label v-for="e in environments" :key="e.id" class="env-item">
        <input type="checkbox" :checked="selectedEnvIds.includes(e.id)" @change="toggleEnv(e.id)" />
        <span>{{ e.name }} <span class="muted">({{ e.paradigmKey }})</span></span>
      </label>
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
.section { font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); margin: 18px 0 8px; }
.env-list { display: flex; flex-direction: column; gap: 6px; }
.env-item { display: flex; align-items: center; gap: 8px; cursor: pointer; }
.env-item input { width: auto; }
</style>
