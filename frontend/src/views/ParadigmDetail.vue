<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { locale, te, t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const selected = ref(null);
const err = ref("");
const tab = ref("apparatus");

// The backend localizes free-text scientific labels via ?lang; the frontend
// only keeps a UI dictionary that translates the fixed enums (zone type/role,
// species). When an enum translation is missing it falls back to the raw key.
function tEnum(ns, value) {
  const key = `paradigms.${ns}.${value}`;
  return te(key) ? t(key) : value;
}
// Localize a parameter enum value (N/E/S/W, opaque/clear, fixed_speed, ...).
function tVal(value) {
  const key = `paramValues.${value}`;
  return te(key) ? t(key) : value;
}
function speciesLabel(list) {
  return (list || []).map((s) => tEnum("species", s)).join(", ");
}

// Tabs are derived from the loaded spec: apparatus and metrics always exist;
// zones and qc only show when they carry data.
const tabs = computed(() => {
  if (!selected.value) return [];
  const s = selected.value;
  const out = [{ key: "apparatus", label: t("paradigms.apparatusParams") }];
  if (s.zones.length) out.push({ key: "zones", label: t("paradigms.zones") });
  out.push({ key: "metrics", label: t("paradigms.metrics") });
  if (s.eventTypes && s.eventTypes.length) out.push({ key: "events", label: t("paradigms.events") });
  if (s.qc.length) out.push({ key: "qc", label: t("paradigms.qc") });
  return out;
});

// Keep the active tab valid if the current one disappears after a reload.
watch(tabs, (list) => {
  if (list.length && !list.some((x) => x.key === tab.value)) tab.value = list[0].key;
});

// Write the "Dashboard / Paradigms / <name>" trail into the breadcrumb store.
function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("paradigms.title"), to: "/paradigms" },
    { label: selected.value?.name || route.params.key },
  ]);
}

async function load() {
  err.value = "";
  selected.value = null;
  try {
    selected.value = await api(`/paradigms/${route.params.key}?lang=${locale.value}`);
    syncCrumb();
  } catch (e) {
    err.value = e.message;
  }
}

// A paradigm is a template; users create named environment instances from it.
function createEnvironment() {
  router.push(`/environments/new?paradigm=${route.params.key}`);
}

// Re-fetch backend-localized labels when the language changes (breadcrumb too).
watch(locale, load);
// Reload if the :key changes while staying on the same component instance.
watch(() => route.params.key, load);

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <p class="err" v-if="err">{{ err }}</p>

  <div class="detail" v-if="selected">
    <!-- Identity + metadata -->
    <div class="card">
      <div class="detail-head">
        <h2 style="margin-top:0">{{ selected.name }} <span class="muted">({{ selected.key }})</span></h2>
        <div class="head-actions">
          <button @click="router.push('/paradigms')">{{ $t("common.back") }}</button>
          <button class="primary" @click="createEnvironment">{{ $t("paradigms.createEnvironment") }}</button>
        </div>
      </div>
      <p class="muted">
        {{ $t("paradigms.category") }}: {{ $t("paradigms.categories." + selected.category) }}
        · {{ $t("paradigms.schemaVersion") }} {{ selected.schemaVersion }}
        · {{ speciesLabel(selected.species) }}
      </p>
      <div class="chips">
        <span class="pill" v-for="tt in selected.trialTypes" :key="tt.key">{{ tt.label }}</span>
      </div>
    </div>

    <!-- Tab strip for the rest of the contract -->
    <div class="tabs">
      <button
        v-for="tb in tabs"
        :key="tb.key"
        class="tab"
        :class="{ active: tab === tb.key }"
        @click="tab = tb.key"
      >
        {{ tb.label }}
      </button>
    </div>

    <!-- Apparatus parameters (read-only template; values are chosen per environment) -->
    <div class="card" v-show="tab === 'apparatus'">
      <div class="table-scroll">
        <table>
          <thead><tr><th>{{ $t("paradigms.field") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.range") }}</th><th>{{ $t("paradigms.default") }}</th></tr></thead>
          <tbody>
            <tr v-for="f in selected.apparatusParameters" :key="f.key">
              <td>{{ f.label }} <span class="muted">{{ f.key }}</span></td>
              <td><span class="pill">{{ f.unit }}</span></td>
              <td class="muted">{{ f.min != null ? f.min + " - " + f.max : (f.options ? f.options.map(tVal).join(" / ") : "-") }}</td>
              <td>{{ f.default != null ? tVal(f.default) : "-" }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Zones -->
    <div class="card" v-show="tab === 'zones'" v-if="selected.zones.length">
      <div class="table-scroll">
        <table>
          <thead><tr><th>{{ $t("common.name") }}</th><th>{{ $t("paradigms.zoneType") }}</th><th>{{ $t("paradigms.zoneRole") }}</th><th>{{ $t("paradigms.required") }}</th></tr></thead>
          <tbody>
            <tr v-for="z in selected.zones" :key="z.key">
              <td>{{ z.label }} <span class="muted">{{ z.key }}</span></td>
              <td class="muted">{{ tEnum("zoneTypes", z.type) }}</td>
              <td><span class="pill">{{ tEnum("zoneRoles", z.role) }}</span></td>
              <td>{{ z.required ? "✓" : "" }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Metrics -->
    <div class="card" v-show="tab === 'metrics'">
      <div class="table-scroll">
        <table>
          <thead><tr><th>{{ $t("paradigms.metric") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.required") }}</th><th>{{ $t("common.description") }}</th></tr></thead>
          <tbody>
            <tr v-for="m in selected.metrics" :key="m.key">
              <td>{{ m.label }} <span class="muted">{{ m.key }}<template v-if="m.templated">.*</template></span></td>
              <td><span class="pill">{{ m.unit }}</span></td>
              <td>{{ m.required ? "✓" : "" }}</td>
              <td class="muted">{{ m.definition }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Event types + CV detection contract -->
    <div class="card" v-show="tab === 'events'" v-if="selected.eventTypes && selected.eventTypes.length">
      <p class="muted" style="margin-top:0">{{ $t("paradigms.eventsIntro") }}</p>
      <div class="table-scroll">
        <table>
          <thead><tr><th>{{ $t("paradigms.event") }}</th><th>{{ $t("paradigms.payload") }}</th><th>{{ $t("paradigms.detection") }}</th></tr></thead>
          <tbody>
            <tr v-for="et in selected.eventTypes" :key="et.type">
              <td>{{ et.label }} <span class="muted">{{ et.type }}</span></td>
              <td class="muted">{{ Object.keys(et.payload || {}).join(", ") || "-" }}</td>
              <td>
                <span class="pill" :class="et.detect?.kind === 'custom' ? 'pill-warn' : ''">{{ et.detect?.kind || "manual" }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- QC requirements -->
    <div class="card" v-show="tab === 'qc'" v-if="selected.qc.length">
      <ul class="rules">
        <li v-for="q in selected.qc" :key="q.key">
          <code>{{ q.key }} {{ q.operator }} {{ q.value }}</code>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.detail-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.detail-head h2 { margin: 0; }
.head-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.tabs { display: flex; flex-wrap: wrap; gap: 6px; border-bottom: 1px solid var(--line); }
.tab {
  background: transparent; border: 1px solid transparent; border-bottom: none;
  border-radius: 10px 10px 0 0; padding: 8px 14px; cursor: pointer; color: var(--muted);
  margin-bottom: -1px;
}
.tab:hover { color: var(--text); }
.tab.active {
  color: var(--text); background: var(--panel);
  border-color: var(--line); border-bottom-color: var(--panel);
}
.rules { margin: 0; padding-left: 18px; }
.rules li { margin: 4px 0; }
.rules code { background: var(--active-bg); padding: 2px 6px; border-radius: 6px; }
.pill-warn { color: #ffb454; border-color: #ffb454; }
h3 { margin: 0 0 8px; }
</style>
