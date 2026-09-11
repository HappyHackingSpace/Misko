<script setup>
// The published paradigm catalog. The API returns the whole list at once, and
// says which versions a worker can actually analyze.
import { onMounted, ref } from "vue";
import { api } from "../api/client.js";

const paradigms = ref([]);
const error = ref("");

onMounted(async () => {
  try {
    paradigms.value = (await api("/paradigms")).data || [];
  } catch (e) {
    error.value = e.message;
  }
});
</script>

<template>
  <div class="head">
    <h1>{{ $t("paradigms.title") }}</h1>
  </div>
  <p class="err" v-if="error">{{ error }}</p>

  <div class="card">
    <div class="table-scroll">
      <table class="rows">
        <thead>
          <tr>
            <th>{{ $t("paradigms.key") }}</th>
            <th>{{ $t("common.name") }}</th>
            <th>{{ $t("paradigms.versions") }}</th>
            <th>{{ $t("paradigms.automated") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="paradigm in paradigms" :key="paradigm.key">
            <td>
              <RouterLink class="link" :to="`/paradigms/${paradigm.key}`">{{ paradigm.key }}</RouterLink>
            </td>
            <td>{{ paradigm.name }}</td>
            <td class="muted">{{ paradigm.versions.join(", ") }}</td>
            <td class="muted">{{ paradigm.automatedAnalysis ? $t("common.yes") : $t("common.no") }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.rows { width: 100%; border-collapse: collapse; }
.rows th { text-align: left; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; padding: 6px 8px; }
.rows td { padding: 6px 8px; border-top: 1px solid var(--line); }
.link { color: var(--accent); font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
