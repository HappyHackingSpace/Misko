<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api.js";

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
  <h1>Panel</h1>
  <div class="row">
    <div class="card" style="flex:1"><div class="muted">Test</div><div style="font-size:26px;font-weight:800">{{ counts.tests }}</div></div>
    <div class="card" style="flex:1"><div class="muted">Senaryo</div><div style="font-size:26px;font-weight:800">{{ counts.scenarios }}</div></div>
    <div class="card" style="flex:1"><div class="muted">Denek</div><div style="font-size:26px;font-weight:800">{{ counts.subjects }}</div></div>
    <div class="card" style="flex:1"><div class="muted">Cihaz</div><div style="font-size:26px;font-weight:800">{{ counts.devices }}</div></div>
  </div>

  <div class="card">
    <h1 style="font-size:16px">Son testler</h1>
    <table v-if="recent.length">
      <thead><tr><th>Senaryo</th><th>Denek</th><th>Durum</th><th>Tarih</th></tr></thead>
      <tbody>
        <tr v-for="t in recent" :key="t.id">
          <td>{{ t.scenario?.name }}</td>
          <td>{{ t.subject?.code }}</td>
          <td :class="'status-' + t.status">{{ t.status }}</td>
          <td class="muted">{{ new Date(t.createdAt).toLocaleString("tr-TR") }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else class="muted">Henüz test yok.</p>
  </div>
</template>
