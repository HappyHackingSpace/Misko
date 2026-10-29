<script setup>
// One published paradigm version, read as a template: what it measures, the
// values an environment and a protocol step have to provide, and the metrics,
// zones, events and quality rules the engine produces. Names, explanations
// and technical definitions come from the API in the chosen language;
// formulas stay as written.
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api/client.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import { useParameters } from "../composables/useParameters.js";
import { metricUnit } from "../components/analysis/format.js";

const { t, te, locale } = useI18n();
const route = useRoute();
const auth = useAuth();
const crumb = useBreadcrumb();
const { unit, format, range } = useParameters();

// Turning a paradigm into a named apparatus needs apparatus:write.
const canCreateEnvironment = computed(() => auth.can("apparatus:write"));

const manifest = ref(null);
const error = ref("");
const tab = ref("parameters");
const open = ref({});

const SCOPES = ["APPARATUS", "ANALYSIS", "SESSION"];
const scopes = computed(() =>
  SCOPES.map((scope) => ({ scope, parameters: (manifest.value?.parameters || []).filter((p) => p.scope === scope) })).filter(
    (group) => group.parameters.length,
  ),
);

const tabs = computed(() => {
  if (!manifest.value) return [];
  const list = [{ key: "parameters", label: t("paradigms.parameters") }, { key: "metrics", label: t("paradigms.metrics") }];
  if (manifest.value.zones?.length) list.push({ key: "zones", label: t("paradigms.zones") });
  if (manifest.value.events?.length || manifest.value.inputEvents?.length) list.push({ key: "events", label: t("paradigms.events") });
  if (manifest.value.qc?.length) list.push({ key: "qc", label: t("paradigms.qc") });
  return list;
});
watch(tabs, (list) => {
  if (list.length && !list.some((entry) => entry.key === tab.value)) tab.value = list[0].key;
});

// Input events may reappear among the derived events; each type is listed once.
const allEvents = computed(() => {
  const seen = new Set();
  return [...(manifest.value?.inputEvents || []), ...(manifest.value?.events || [])].filter(
    (event) => !seen.has(event.type) && seen.add(event.type),
  );
});
const metricUnitText = (metric) => unit({ unit: metricUnit(metric.unit, metric.key) });
const qcLabel = (rule) => (te(`paradigms.qcRules.${rule.key}`) ? t(`paradigms.qcRules.${rule.key}`) : rule.key);
const category = (key) => t(`paradigms.categories.${String(key).toLowerCase()}`);
const toggle = (key) => (open.value = { ...open.value, [key]: !open.value[key] });

async function load() {
  error.value = "";
  try {
    // GET /paradigms/{key} already returns the latest manifest in full, so it
    // is only fetched again when a specific version is asked for.
    const latest = await api(`/paradigms/${route.params.key}`);
    const asked = Number(route.query.version);
    manifest.value = asked && asked !== latest.version ? await api(`/paradigms/${route.params.key}/versions/${asked}`) : latest;
    crumb.set([{ label: t("paradigms.title"), to: "/paradigms" }, { label: manifest.value.name }]);
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(load);
watch(locale, load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <p class="err" v-if="error">{{ error }}</p>
  <div class="detail" v-if="manifest">
    <div class="card">
      <h2 style="margin-top:0">{{ manifest.name }}</h2>
      <p>{{ manifest.description }}</p>
      <p class="muted">
        {{ category(manifest.category) }} ·
        {{ $t("paradigms.trialTypes") }}: {{ manifest.trialTypes.map((tt) => tt.label).join(", ") }} ·
        v{{ manifest.version }}
      </p>
      <!-- A paradigm is a template. This is where it becomes a real apparatus
           the laboratory owns, with the physical values of its own setup. -->
      <RouterLink
        v-if="canCreateEnvironment"
        class="link"
        :to="`/environments/new?paradigm=${manifest.key}`"
        data-test="create-environment"
      >
        {{ $t("environments.createFromParadigm") }}
      </RouterLink>
    </div>

    <div class="card">
      <div class="tabs">
        <button v-for="entry in tabs" :key="entry.key" class="tab" :class="{ active: tab === entry.key }" @click="tab = entry.key">
          {{ entry.label }}
        </button>
      </div>

      <div v-show="tab === 'parameters'">
        <section v-for="group in scopes" :key="group.scope" class="scope" :data-test="`scope-${group.scope}`">
          <h4>{{ $t(`paradigms.scopes.${group.scope}`) }}</h4>
          <p class="muted hint">{{ $t(`paradigms.scopeHints.${group.scope}`) }}</p>
          <div class="table-scroll">
            <table class="rows">
              <thead>
                <tr><th>{{ $t("common.name") }}</th><th>{{ $t("paradigms.range") }}</th><th>{{ $t("paradigms.default") }}</th></tr>
              </thead>
              <tbody>
                <tr v-for="parameter in group.parameters" :key="parameter.key">
                  <td>
                    <strong>{{ parameter.label }}</strong>
                    <div class="muted small">{{ parameter.help }}</div>
                  </td>
                  <td class="muted nowrap">{{ range(parameter) }}</td>
                  <td class="muted nowrap">{{ format(parameter, parameter.default) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
        <p class="muted" v-if="!scopes.length">{{ $t("paradigms.noParameters") }}</p>
      </div>

      <div class="table-scroll" v-show="tab === 'metrics'">
        <table class="rows">
          <thead><tr><th>{{ $t("paradigms.metric") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.description") }}</th></tr></thead>
          <tbody>
            <template v-for="metric in manifest.metrics" :key="metric.key">
              <tr>
                <td><strong>{{ metric.label }}</strong></td>
                <td class="muted nowrap">{{ metricUnitText(metric) }}</td>
                <td>
                  {{ metric.description }}
                  <a href="#" class="link small toggle" @click.prevent="toggle(metric.key)">{{ $t("paradigms.technical") }}</a>
                </td>
              </tr>
              <tr v-if="open[metric.key]" class="technical">
                <td colspan="3">
                  <div class="small"><span class="muted">{{ metric.key }}</span></div>
                  <div class="small">{{ metric.definition }}</div>
                  <div class="small"><span class="muted">{{ $t("paradigms.formula") }}:</span> <code>{{ metric.formula }}</code></div>
                  <div class="small"><span class="muted">{{ $t("paradigms.missingWhen") }}:</span> {{ metric.missingWhen }}</div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <div class="table-scroll" v-show="tab === 'zones'">
        <table class="rows">
          <thead><tr><th>{{ $t("paradigms.zone") }}</th><th>{{ $t("paradigms.geometry") }}</th></tr></thead>
          <tbody>
            <tr v-for="zone in manifest.zones" :key="zone.key">
              <td><strong>{{ zone.label }}</strong></td>
              <td class="muted small">{{ zone.geometry }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="table-scroll" v-show="tab === 'events'">
        <table class="rows">
          <thead><tr><th>{{ $t("paradigms.event") }}</th><th>{{ $t("paradigms.definition") }}</th></tr></thead>
          <tbody>
            <tr v-for="event in allEvents" :key="event.type">
              <td><strong>{{ event.label }}</strong></td>
              <td class="muted small">{{ event.definition }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="table-scroll" v-show="tab === 'qc'">
        <table class="rows">
          <thead><tr><th>{{ $t("paradigms.rule") }}</th><th>{{ $t("paradigms.limit") }}</th></tr></thead>
          <tbody>
            <tr v-for="rule in manifest.qc" :key="rule.key">
              <td>{{ qcLabel(rule) }}</td>
              <td class="muted nowrap">{{ rule.operator }} {{ format({ unit: rule.unit }, rule.value) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--line); margin-bottom: 16px; flex-wrap: wrap; }
.tab { background: transparent; border: none; border-bottom: 2px solid transparent; border-radius: 0; padding: 8px 14px; color: var(--muted); font-size: 13px; margin-bottom: -1px; }
.tab.active { color: var(--txt); border-bottom-color: var(--accent, #4cc2ff); font-weight: 600; }
.scope + .scope { margin-top: 20px; }
.scope h4 { margin: 0; font-size: 13px; }
.hint { margin: 2px 0 8px; font-size: 12px; }
.rows { width: 100%; border-collapse: collapse; }
.rows th { text-align: left; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; padding: 6px 8px; }
.rows td { padding: 8px; border-top: 1px solid var(--line); vertical-align: top; }
.technical td { border-top: none; padding-top: 0; }
.small { font-size: 12px; line-height: 1.5; }
.nowrap { white-space: nowrap; }
.toggle { margin-left: 6px; }
</style>
