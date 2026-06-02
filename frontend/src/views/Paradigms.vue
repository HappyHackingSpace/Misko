<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api.js";

const items = ref([]);
const selected = ref(null);
const err = ref("");

async function load() {
  err.value = "";
  try {
    items.value = await api("/paradigms");
    if (items.value.length) await select(items.value[0].key);
  } catch (e) {
    err.value = e.message;
  }
}

async function select(key) {
  err.value = "";
  try {
    selected.value = await api(`/paradigms/${key}`);
  } catch (e) {
    err.value = e.message;
  }
}

onMounted(load);
</script>

<template>
  <h1>{{ $t("paradigms.title") }}</h1>
  <p class="muted" style="margin-top:0">{{ $t("paradigms.intro") }}</p>
  <p class="err" v-if="err">{{ err }}</p>

  <div class="cols">
    <!-- Sol: paradigma listesi -->
    <div class="list">
      <button
        v-for="p in items"
        :key="p.key"
        class="listitem"
        :class="{ active: selected && selected.key === p.key }"
        @click="select(p.key)"
      >
        <span class="who-name">{{ p.name }}</span>
        <span class="who-meta">{{ $t("paradigms.categories." + p.category) }}</span>
        <span class="pill">{{ p.metricCount }} {{ $t("paradigms.metricsShort") }}</span>
      </button>
    </div>

    <!-- Sag: secili paradigma detayi -->
    <div class="detail" v-if="selected">
      <div class="card">
        <h2 style="margin-top:0">{{ selected.name }} <span class="muted">({{ selected.key }})</span></h2>
        <p class="muted">
          {{ $t("paradigms.category") }}: {{ $t("paradigms.categories." + selected.category) }}
          · {{ $t("paradigms.schemaVersion") }} {{ selected.schemaVersion }}
          · {{ selected.species.join(", ") }}
        </p>
        <div class="chips">
          <span class="pill" v-for="tt in selected.trialTypes" :key="tt.key">{{ tt.label }}</span>
        </div>
      </div>

      <!-- Apparatus parametreleri -->
      <div class="card">
        <h3>{{ $t("paradigms.apparatusParams") }}</h3>
        <table>
          <thead><tr><th>{{ $t("paradigms.field") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.range") }}</th><th>{{ $t("paradigms.default") }}</th></tr></thead>
          <tbody>
            <tr v-for="f in selected.apparatusParameters" :key="f.key">
              <td>{{ f.label }} <span class="muted">{{ f.key }}</span></td>
              <td><span class="pill">{{ f.unit }}</span></td>
              <td class="muted">{{ f.min != null ? f.min + " - " + f.max : (f.options ? f.options.join(" / ") : "-") }}</td>
              <td>{{ f.default ?? "-" }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Zone'lar -->
      <div class="card" v-if="selected.zones.length">
        <h3>{{ $t("paradigms.zones") }}</h3>
        <table>
          <thead><tr><th>{{ $t("common.name") }}</th><th>{{ $t("paradigms.zoneType") }}</th><th>{{ $t("paradigms.zoneRole") }}</th><th>{{ $t("paradigms.required") }}</th></tr></thead>
          <tbody>
            <tr v-for="z in selected.zones" :key="z.key">
              <td>{{ z.label }} <span class="muted">{{ z.key }}</span></td>
              <td class="muted">{{ z.type }}</td>
              <td><span class="pill">{{ z.role }}</span></td>
              <td>{{ z.required ? "✓" : "" }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Metrikler -->
      <div class="card">
        <h3>{{ $t("paradigms.metrics") }} ({{ selected.metrics.length }})</h3>
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

      <!-- Kabul kriterleri -->
      <div class="card" v-if="selected.acceptance.length">
        <h3>{{ $t("paradigms.acceptance") }}</h3>
        <ul class="rules">
          <li v-for="a in selected.acceptance" :key="a.key">
            <code>{{ a.metricKey }} {{ a.operator }} {{ a.value }}</code>
            <span class="muted" v-if="a.appliesToTrialTypes"> · {{ a.appliesToTrialTypes.join(", ") }}</span>
          </li>
        </ul>
      </div>

      <!-- QC gereksinimleri -->
      <div class="card" v-if="selected.qc.length">
        <h3>{{ $t("paradigms.qc") }}</h3>
        <ul class="rules">
          <li v-for="q in selected.qc" :key="q.key">
            <code>{{ q.key }} {{ q.operator }} {{ q.value }}</code>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cols { display: flex; gap: 16px; align-items: flex-start; }
.list { display: flex; flex-direction: column; gap: 8px; min-width: 220px; }
.listitem {
  display: flex; flex-direction: column; align-items: flex-start; gap: 2px;
  text-align: left; background: var(--panel); border: 1px solid var(--line);
  border-radius: 12px; padding: 10px 12px; cursor: pointer;
}
.listitem.active { border-color: var(--accent); background: var(--active-bg); }
.who-name { font-weight: 700; }
.who-meta { color: var(--muted); font-size: 12px; }
.detail { flex: 1; display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.rules { margin: 0; padding-left: 18px; }
.rules li { margin: 4px 0; }
.rules code { background: var(--active-bg); padding: 2px 6px; border-radius: 6px; }
h3 { margin: 0 0 8px; }
</style>
