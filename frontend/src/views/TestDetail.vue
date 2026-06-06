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

const test = ref(null);
const err = ref("");

const environments = computed(() => test.value?.scenario?.environments || []);
const resultJson = computed(() =>
  test.value?.result ? JSON.stringify(test.value.result, null, 2) : "");

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("tests.title"), to: "/tests" },
    { label: test.value ? `${test.value.scenario?.name} / ${test.value.subject?.code}` : route.params.id },
  ]);
}

async function load() {
  err.value = "";
  try {
    test.value = await api(`/tests/${route.params.id}`);
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function setStatus(status) {
  err.value = "";
  try {
    const body = { status };
    if (status === "RUNNING") body.startedAt = new Date().toISOString();
    if (status === "DONE" || status === "FAILED") body.endedAt = new Date().toISOString();
    await api(`/tests/${route.params.id}`, { method: "PATCH", body });
    await load();
  } catch (e) {
    err.value = e.message;
  }
}

async function remove() {
  if (!confirm(t("tests.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/tests/${route.params.id}`, { method: "DELETE" });
    router.push("/tests");
  } catch (e) {
    err.value = e.message;
  }
}

function passedLabel(row) {
  if (row.passed === true) return t("tests.passYes");
  if (row.passed === false) return t("tests.passNo");
  return t("tests.passNa");
}
function passedClass(row) {
  if (row.passed === true) return "pass-yes";
  if (row.passed === false) return "pass-no";
  return "pass-na";
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <p class="err" v-if="err">{{ err }}</p>

  <div class="detail" v-if="test">
    <div class="card">
      <div class="detail-head">
        <h2 style="margin-top:0">{{ test.scenario?.name }} <span class="muted">/ {{ test.subject?.code }}</span></h2>
        <div class="head-actions">
          <button @click="router.push('/tests')">{{ $t("common.back") }}</button>
          <button v-if="test.status === 'PENDING'" @click="setStatus('RUNNING')">{{ $t("tests.start") }}</button>
          <button v-if="test.status === 'RUNNING'" @click="setStatus('DONE')">{{ $t("tests.finish") }}</button>
          <button v-if="test.status === 'RUNNING'" class="danger" @click="setStatus('FAILED')">{{ $t("tests.cancel") }}</button>
          <button class="danger" @click="remove">{{ $t("common.delete") }}</button>
        </div>
      </div>
      <table class="kv">
        <tbody>
          <tr><th>{{ $t("tests.scenario") }}</th><td>{{ test.scenario?.name }}</td></tr>
          <tr>
            <th>{{ $t("scenarios.environments") }}</th>
            <td>
              <span v-if="!environments.length" class="muted">-</span>
              <span v-for="e in environments" :key="e.id" class="env-chip">{{ e.name }} <span class="muted">({{ e.paradigmKey }})</span></span>
            </td>
          </tr>
          <tr><th>{{ $t("tests.subject") }}</th><td>{{ test.subject?.code }}</td></tr>
          <tr><th>{{ $t("tests.operator") }}</th><td>{{ test.operator?.name }}</td></tr>
          <tr><th>{{ $t("tests.status") }}</th><td><span :class="'status-' + test.status">{{ test.status }}</span></td></tr>
          <tr><th>{{ $t("tests.result") }}</th><td><span class="pill" :class="passedClass(test)">{{ passedLabel(test) }}</span></td></tr>
          <tr v-if="test.notes"><th>{{ $t("common.notes") }}</th><td class="muted">{{ test.notes }}</td></tr>
        </tbody>
      </table>
    </div>

    <div class="card">
      <h4>{{ $t("tests.metrics") }}</h4>
      <pre v-if="resultJson" class="result">{{ resultJson }}</pre>
      <p v-else class="muted">{{ $t("tests.noResult") }}</p>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.detail-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.head-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.kv th { text-align: left; padding-right: 16px; white-space: nowrap; vertical-align: top; }
.kv td { width: 100%; }
.env-chip { display: inline-block; margin-right: 10px; }
.pill.pass-yes { background: #1f7a3d; color: #fff; }
.pill.pass-no { background: #a3271f; color: #fff; }
.pill.pass-na { background: var(--active-bg); color: var(--muted); }
.result { background: var(--active-bg); border: 1px solid var(--line); border-radius: 8px; padding: 12px; overflow: auto; font-size: 12px; }
h4 { margin: 0 0 10px; }
</style>
