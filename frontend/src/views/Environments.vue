<script setup>
// Apparatus and their measurement revisions. Anyone signed in may read them;
// defining one needs apparatus:write, so the control appears only for a role
// the API would accept.
import { computed, onMounted, ref } from "vue";
import { api } from "../api/client.js";
import { useAuth } from "../stores/auth.js";

const auth = useAuth();
const canWrite = computed(() => auth.can("apparatus:write"));
const environments = ref([]);
const error = ref("");

onMounted(async () => {
  try {
    environments.value = (await api("/environments")).data || [];
  } catch (e) {
    error.value = e.message;
  }
});
</script>

<template>
  <div class="head">
    <h1>{{ $t("environments.title") }}</h1>
    <button v-if="canWrite" class="primary" data-test="environment-new" @click="$router.push('/environments/new')">
      {{ $t("environments.new") }}
    </button>
  </div>
  <p class="err" v-if="error">{{ error }}</p>

  <div class="card">
    <div class="table-scroll">
      <table class="rows">
        <thead>
          <tr>
            <th>{{ $t("common.name") }}</th>
            <th>{{ $t("environments.paradigm") }}</th>
            <th>{{ $t("environments.latestRevision") }}</th>
            <th>{{ $t("common.notes") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="environment in environments" :key="environment.id" data-test="environment">
            <td>{{ environment.name }}</td>
            <td class="muted">{{ environment.paradigmKey }}</td>
            <td class="muted">{{ environment.latestRevision }}</td>
            <td class="muted">{{ environment.notes }}</td>
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
</style>
