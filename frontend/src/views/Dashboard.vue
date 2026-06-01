<script setup>
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";

const { locale } = useI18n();
const counts = ref({ tests: 0, scenarios: 0, subjects: 0, devices: 0 });
const recent = ref([]);

onMounted(async () => {
  const [tests, scenarios, subjects, devices] = await Promise.all([
    api("/tests"), api("/scenarios"), api("/subjects"), api("/devices"),
  ]);
  counts.value = { tests: tests.length, scenarios: scenarios.length, subjects: subjects.length, devices: devices.length };
  recent.value = tests.slice(0, 6);
});
</script>

<template>
  <h1>{{ $t("dashboard.title") }}</h1>
  <div class="row">
    <div class="card" style="flex:1"><div class="muted">{{ $t("dashboard.tests") }}</div><div style="font-size:26px;font-weight:800">{{ counts.tests }}</div></div>
    <div class="card" style="flex:1"><div class="muted">{{ $t("dashboard.scenarios") }}</div><div style="font-size:26px;font-weight:800">{{ counts.scenarios }}</div></div>
    <div class="card" style="flex:1"><div class="muted">{{ $t("dashboard.subjects") }}</div><div style="font-size:26px;font-weight:800">{{ counts.subjects }}</div></div>
    <div class="card" style="flex:1"><div class="muted">{{ $t("dashboard.devices") }}</div><div style="font-size:26px;font-weight:800">{{ counts.devices }}</div></div>
  </div>

  <div class="card">
    <h1 style="font-size:16px">{{ $t("dashboard.recentTests") }}</h1>
    <table v-if="recent.length">
      <thead><tr><th>{{ $t("dashboard.scenario") }}</th><th>{{ $t("dashboard.subject") }}</th><th>{{ $t("dashboard.status") }}</th><th>{{ $t("dashboard.date") }}</th></tr></thead>
      <tbody>
        <tr v-for="t in recent" :key="t.id">
          <td>{{ t.scenario?.name }}</td>
          <td>{{ t.subject?.code }}</td>
          <td :class="'status-' + t.status">{{ t.status }}</td>
          <td class="muted">{{ new Date(t.createdAt).toLocaleString(locale) }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else class="muted">{{ $t("dashboard.empty") }}</p>
  </div>
</template>
