<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const router = useRouter();
const crumb = useBreadcrumb();

const scenarios = ref([]);
const subjects = ref([]);
const form = ref({ scenarioId: "", subjectId: "", notes: "" });
const err = ref("");
const saving = ref(false);

const canCreate = computed(() => form.value.scenarioId && form.value.subjectId);

async function load() {
  err.value = "";
  try {
    const [sc, su] = await Promise.all([
      api("/scenarios?all=true"),
      api("/subjects?all=true"),
    ]);
    scenarios.value = sc.data;
    subjects.value = su.data;
  } catch (e) {
    err.value = e.message;
  }
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("tests.title"), to: "/tests" },
    { label: t("common.new") },
  ]);
}

async function create() {
  err.value = "";
  saving.value = true;
  try {
    await api("/tests", { method: "POST", body: { ...form.value } });
    router.push("/tests");
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <div class="card">
    <h2 style="margin-top:0">{{ $t("common.new") }}</h2>
    <div class="row">
      <div class="field"><label>{{ $t("tests.scenario") }}</label>
        <select v-model="form.scenarioId">
          <option value="" disabled>{{ $t("common.select") }}</option>
          <option v-for="s in scenarios" :key="s.id" :value="s.id">{{ s.name }}</option>
        </select>
      </div>
      <div class="field"><label>{{ $t("tests.subject") }}</label>
        <select v-model="form.subjectId">
          <option value="" disabled>{{ $t("common.select") }}</option>
          <option v-for="s in subjects" :key="s.id" :value="s.id">{{ s.code }}</option>
        </select>
      </div>
      <div class="field" style="flex:2"><label>{{ $t("common.notes") }}</label><input v-model="form.notes" /></div>
    </div>
    <p class="muted" v-if="!scenarios.length || !subjects.length">{{ $t("tests.needFirst") }}</p>
    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/tests')">{{ $t("common.back") }}</button>
      <button class="primary" :disabled="saving || !canCreate" @click="create">{{ $t("tests.createTest") }}</button>
    </div>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; flex-wrap: wrap; }
</style>
