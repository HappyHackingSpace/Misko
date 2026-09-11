<script setup>
// One published paradigm version: the parameters it takes, the zones it knows,
// and the metrics, events and quality rules the engine produces. Definitions
// come from the API and are shown as written, because they are the scientific
// contract.
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api/client.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const route = useRoute();
const crumb = useBreadcrumb();

const summary = ref(null);
const manifest = ref(null);
const version = ref(null);
const error = ref("");
const tab = ref("parameters");

const tabs = computed(() => {
  if (!manifest.value) return [];
  const list = [{ key: "parameters", label: t("paradigms.apparatusParams") }];
  if (manifest.value.zones?.length) list.push({ key: "zones", label: t("paradigms.zones") });
  list.push({ key: "metrics", label: t("paradigms.metrics") });
  if (manifest.value.events?.length) list.push({ key: "events", label: t("paradigms.events") });
  if (manifest.value.qc?.length) list.push({ key: "qc", label: t("paradigms.qc") });
  return list;
});

watch(tabs, (list) => {
  if (list.length && !list.some((entry) => entry.key === tab.value)) tab.value = list[0].key;
});

const parameters = computed(() => [
  ...(manifest.value?.apparatusParameters || []).map((p) => ({ ...p, scope: t("paradigms.apparatus") })),
  ...(manifest.value?.sessionParameters || []).map((p) => ({ ...p, scope: t("paradigms.session") })),
]);

async function load() {
  error.value = "";
  try {
    summary.value = await api(`/paradigms/${route.params.key}`);
    version.value = Number(route.query.version) || summary.value.latestVersion;
    manifest.value = await api(`/paradigms/${route.params.key}/versions/${version.value}`);
    crumb.set([
      { label: t("paradigms.title"), to: "/paradigms" },
      { label: manifest.value.name },
    ]);
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <p class="err" v-if="error">{{ error }}</p>

  <div class="detail" v-if="manifest">
    <div class="card">
      <h2 style="margin-top:0">
        {{ manifest.name }} <span class="muted">{{ manifest.key }} v{{ manifest.version }}</span>
      </h2>
      <p class="muted">
        {{ $t("paradigms.metricEngine") }} {{ manifest.metricEngineVersion }} ·
        {{ $t("paradigms.automated") }}: {{ manifest.automatedAnalysis ? $t("common.yes") : $t("common.no") }}
      </p>
    </div>

    <div class="card">
      <div class="tabs">
        <button v-for="entry in tabs" :key="entry.key" class="tab" :class="{ active: tab === entry.key }" @click="tab = entry.key">
          {{ entry.label }}
        </button>
      </div>

      <div class="table-scroll" v-show="tab === 'parameters'">
        <table class="rows">
          <thead>
            <tr><th>{{ $t("common.name") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.range") }}</th><th>{{ $t("paradigms.default") }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="parameter in parameters" :key="parameter.key">
              <td>{{ parameter.label }} <span class="muted">{{ parameter.scope }}</span></td>
              <td class="muted">{{ parameter.unit }}</td>
              <td class="muted">{{ parameter.min }} - {{ parameter.max }}</td>
              <td class="muted">{{ parameter.default }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="table-scroll" v-show="tab === 'zones'">
        <table class="rows">
          <thead><tr><th>{{ $t("paradigms.zone") }}</th><th>{{ $t("paradigms.geometry") }}</th></tr></thead>
          <tbody>
            <tr v-for="zone in manifest.zones" :key="zone.key">
              <td>{{ zone.key }} <span class="muted">{{ zone.role }}</span></td>
              <td class="muted">{{ zone.geometry }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="table-scroll" v-show="tab === 'metrics'">
        <table class="rows">
          <thead><tr><th>{{ $t("common.name") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.definition") }}</th></tr></thead>
          <tbody>
            <tr v-for="metric in manifest.metrics" :key="metric.key">
              <td>{{ metric.label }} <span class="muted">{{ metric.key }}</span></td>
              <td class="muted">{{ metric.unit }}</td>
              <td class="muted">{{ metric.definition }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="table-scroll" v-show="tab === 'events'">
        <table class="rows">
          <thead><tr><th>{{ $t("paradigms.event") }}</th><th>{{ $t("paradigms.definition") }}</th></tr></thead>
          <tbody>
            <tr v-for="event in manifest.events" :key="event.type">
              <td>{{ event.type }} <span class="muted">{{ event.kind }}</span></td>
              <td class="muted">{{ event.definition }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="table-scroll" v-show="tab === 'qc'">
        <table class="rows">
          <thead><tr><th>{{ $t("paradigms.rule") }}</th><th>{{ $t("paradigms.limit") }}</th></tr></thead>
          <tbody>
            <tr v-for="rule in manifest.qc" :key="rule.key">
              <td>{{ rule.key }}</td>
              <td class="muted">{{ rule.operator }} {{ rule.value }} {{ rule.unit }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--line); margin-bottom: 16px; }
.tab { background: transparent; border: none; border-bottom: 2px solid transparent; border-radius: 0; padding: 8px 14px; color: var(--muted); font-size: 13px; margin-bottom: -1px; }
.tab.active { color: var(--txt); border-bottom-color: var(--accent, #4cc2ff); font-weight: 600; }
.rows { width: 100%; border-collapse: collapse; }
.rows th { text-align: left; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; padding: 6px 8px; }
.rows td { padding: 6px 8px; border-top: 1px solid var(--line); vertical-align: top; }
</style>
