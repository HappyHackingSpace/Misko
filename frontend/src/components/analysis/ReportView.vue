<script setup>
// The study report of one experiment: per endpoint, the group descriptives at
// its timepoint with what is still pending, the role-based comparisons in
// words, dose response and the time course. Only accepted results of animals
// that are not excluded count; the banner on top says when the numbers are
// still provisional so a half-reviewed report is not read as the final one.
import { computed, h, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NButton, NDataTable, NDropdown, NEmpty, NSpin, NTabPane, NTabs, NTag, useMessage } from "naive-ui";
import { report as reportApi } from "../../api/analysis.js";
import GroupChart from "./GroupChart.vue";
import { metricUnit, saveBlob, useNumbers, useParadigmText } from "./format.js";

const props = defineProps({
  experimentId: { type: String, required: true },
});
const { t } = useI18n();
const message = useMessage();
const fmt = useNumbers();
const text = useParadigmText();

const data = ref(null);
const loading = ref(false);
const error = ref("");
const exporting = ref(false);
const endpointTab = ref("");

async function load() {
  loading.value = true;
  error.value = "";
  try {
    data.value = await reportApi.get(props.experimentId);
    for (const s of data.value.sections) {
      for (const v of s.versions) text.need(s.endpoint.paradigmKey, v.paradigmVersion);
      if (!s.versions.length) text.need(s.endpoint.paradigmKey, 1);
    }
    if (!data.value.sections.some((s) => s.endpoint.id === endpointTab.value)) endpointTab.value = data.value.sections[0]?.endpoint.id || "";
  } catch (e) {
    data.value = null;
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}
watch(() => props.experimentId, load, { immediate: true });
defineExpose({ reload: load });

const groupsById = computed(() => Object.fromEntries((data.value?.groups || []).map((g) => [g.id, g])));
const timepointsById = computed(() => Object.fromEntries((data.value?.timepoints || []).map((tp) => [tp.id, tp])));
const groupName = (id) => {
  const g = groupsById.value[id];
  return g ? `${g.code} ${g.name}` : "?";
};
const treatment = (g) => (g?.treatment ? `${g.treatment.substanceName} ${fmt.dose(g.treatment.doseMicro, g.treatment.unit)}` : "");
const unitOf = (section) => metricUnit(section.versions[0]?.unit, section.endpoint.metricKey) || "";
const metricName = (section) => text.metricLabel(section.endpoint.paradigmKey, section.endpoint.metricKey);
const tabLabel = (section) =>
  `${t(`studyReport.kind.${section.endpoint.kind}`)} · ${metricName(section)} · ${timepointsById.value[section.endpoint.timepointId]?.name || ""}`;

// What is still open for the endpoint on screen, at its own timepoint.
const currentSection = computed(() => data.value?.sections.find((s) => s.endpoint.id === endpointTab.value) || data.value?.sections[0] || null);
const open = computed(() => {
  const totals = { pending: 0, awaitingAnalysis: 0 };
  for (const g of currentSection.value?.groups || []) {
    totals.pending += g.pending;
    totals.awaitingAnalysis += g.awaitingAnalysis;
  }
  return totals;
});
const state = computed(() => {
  if (!data.value) return "";
  const outstanding = open.value.pending + open.value.awaitingAnalysis;
  if (outstanding) return "provisional";
  return data.value.experiment.status === "COMPLETED" ? "final" : "interim";
});

function statsColumns(section) {
  const unit = unitOf(section);
  const count = (key, tone) => ({
    title: t(`studyReport.counts.${key}`),
    key,
    align: "right",
    render: (row) => (row[key] ? h("span", { class: `count tone-${tone}` }, fmt.number(row[key])) : h("span", { class: "muted" }, "0")),
  });
  const columns = [
    {
      title: t("studyReport.group"),
      key: "group",
      width: 200,
      fixed: "left",
      render: (row) => {
        const g = groupsById.value[row.groupId];
        return h("span", { class: "group-cell" }, [
          h("b", null, `${g?.code} · ${g?.name}`),
          h("span", { class: "muted" }, [t(`studyReport.roles.${g?.role}`), treatment(g) ? ` · ${treatment(g)}` : ""]),
        ]);
      },
    },
    { title: "n", key: "n", align: "right" },
    { title: t("studyReport.mean"), key: "mean", align: "right", render: (row) => fmt.metric(row.mean, unit) },
    { title: "SD", key: "sd", align: "right", render: (row) => fmt.metric(row.sd, unit) },
    { title: "SEM", key: "sem", align: "right", render: (row) => fmt.metric(row.sem, unit) },
  ];
  if (section.endpoint.baselineTimepointId) {
    columns.push({
      title: t("studyReport.change"),
      key: "change",
      align: "right",
      render: (row) =>
        row.change && row.change.n
          ? `${fmt.signedMetric(row.change.mean, unit)}${row.change.sem != null ? ` ± ${fmt.metric(row.change.sem, unit)}` : ""} (n=${row.change.n})`
          : "–",
    });
  }
  columns.push(
    count("tested", "plain"),
    count("pending", "warning"),
    count("awaitingAnalysis", "warning"),
    count("rejected", "error"),
    count("missing", "warning"),
    count("excluded", "error"),
  );
  return columns;
}

const bars = (section) =>
  section.groups.map((row) => {
    const g = groupsById.value[row.groupId];
    return { id: row.groupId, code: g?.code, name: g?.name, role: g?.role, mean: row.mean, sem: row.sem, n: row.n };
  });

// One sentence per comparison, in the endpoint's direction.
function compareText(section, c) {
  const unit = unitOf(section);
  const base = t("studyReport.compare.sentence", {
    group: groupName(c.groupId),
    reference: groupName(c.referenceGroupId),
    difference: fmt.signedMetric(c.difference, unit),
    percent: fmt.percent(c.percentDifference),
  });
  if (c.effect === null) return t("studyReport.compare.notYet", { group: groupName(c.groupId), reference: groupName(c.referenceGroupId) });
  let verdict;
  if (c.kind === "MODEL_VALIDITY") verdict = c.effect < 0 ? t("studyReport.compare.deficit") : t("studyReport.compare.noDeficit");
  else verdict = c.effect > 0 ? t("studyReport.compare.improves") : t("studyReport.compare.noImprovement");
  return `${base} ${verdict}`;
}
const rescueWidth = (value) => `${Math.max(0, Math.min(100, value ?? 0))}%`;
const KINDS = ["MODEL_VALIDITY", "ASSAY_SENSITIVITY", "EFFICACY"];
const comparisonsBy = (section, kind) => section.comparisons.filter((c) => c.kind === kind);

function doseTrend(points) {
  const values = points.map((p) => p.rescuePercent).filter((v) => typeof v === "number");
  if (values.length < 2) return t("studyReport.dose.tooFew");
  const up = values.every((v, i) => i === 0 || v >= values[i - 1]);
  const down = values.every((v, i) => i === 0 || v <= values[i - 1]);
  if (up) return t("studyReport.dose.rises");
  if (down) return t("studyReport.dose.falls");
  return t("studyReport.dose.noTrend");
}

const courseTimepoints = (section) => section.timeCourse.map((c) => timepointsById.value[c.timepointId]).filter(Boolean);
function courseRows(section) {
  return section.groups.map((row) => {
    const cells = {};
    for (const point of section.timeCourse) {
      cells[point.timepointId] = point.groups.find((g) => g.groupId === row.groupId) || null;
    }
    return { groupId: row.groupId, cells };
  });
}
function courseCell(section, stats) {
  if (!stats) return "–";
  const unit = unitOf(section);
  const value = stats.n ? `${fmt.metric(stats.mean, unit)}${stats.sem != null ? ` ± ${fmt.metric(stats.sem, unit)}` : ""} (n=${stats.n})` : "–";
  const open = stats.pending + stats.awaitingAnalysis;
  return open ? `${value} · ${t("studyReport.openCount", { count: open })}` : value;
}

const exportOptions = computed(() => [
  { key: "animals", label: t("studyReport.export.animals") },
  { key: "groups", label: t("studyReport.export.groups") },
  { key: "animals-excel", label: t("studyReport.export.animalsExcel") },
  { key: "groups-excel", label: t("studyReport.export.groupsExcel") },
]);
async function onExport(key) {
  const [level, excel] = key.split("-");
  exporting.value = true;
  try {
    const { blob, fileName } = await reportApi.export(props.experimentId, { level, excel: !!excel });
    saveBlob(blob, `${data.value?.experiment.code || "report"}-${fileName.replace(/^misko-/, "")}`);
    message.success(t("studyReport.export.done"));
  } catch (e) {
    message.error(e.message);
  } finally {
    exporting.value = false;
  }
}

// "3 results wait for review and 2 animals wait for analysis", leaving out a zero part.
const provisionalParts = computed(() => {
  const parts = [];
  if (open.value.pending) parts.push(t("studyReport.state.provisionalPending", { n: open.value.pending }));
  if (open.value.awaitingAnalysis) parts.push(t("studyReport.state.provisionalAwaiting", { n: open.value.awaitingAnalysis }));
  const text = parts.join(t("studyReport.state.provisionalJoin"));
  return text.charAt(0).toUpperCase() + text.slice(1);
});
</script>

<template>
  <div class="report" data-test="study-report">
    <NAlert v-if="error" type="error" :title="error" data-test="report-error" />
    <NSpin :show="loading && !data">
      <template v-if="data">
        <div class="report-head">
          <NAlert
            v-if="state === 'provisional'"
            type="warning"
            :title="$t('studyReport.state.provisionalTitle')"
            data-test="report-provisional"
            class="banner"
          >
            {{ $t("studyReport.state.provisional", { parts: provisionalParts }) }}
            <RouterLink class="link" :to="{ path: '/review', query: { experimentId: data.experiment.id, status: 'PENDING' } }" data-test="report-review-link">
              {{ $t("studyReport.state.goReview") }}
            </RouterLink>
          </NAlert>
          <NAlert v-else-if="state === 'interim'" type="info" :title="$t('studyReport.state.interimTitle')" data-test="report-interim" class="banner">
            {{ $t("studyReport.state.interim") }}
          </NAlert>
          <NAlert v-else-if="state === 'final'" type="success" :title="$t('studyReport.state.finalTitle')" data-test="report-final" class="banner">
            {{ $t("studyReport.state.final") }}
          </NAlert>
          <NDropdown trigger="click" :options="exportOptions" @select="onExport">
            <NButton :loading="exporting" secondary data-test="report-export">{{ $t("studyReport.export.button") }}</NButton>
          </NDropdown>
        </div>
        <NAlert v-if="data.experiment.blinded" type="default" :show-icon="false" class="blind-note" data-test="report-blinded">
          {{ $t("studyReport.blinded") }}
        </NAlert>

        <NEmpty v-if="!data.sections.length" :description="$t('studyReport.noEndpoints')" data-test="report-empty" />

        <NTabs v-else v-model:value="endpointTab" type="line" animated>
          <NTabPane v-for="section in data.sections" :key="section.endpoint.id" :name="section.endpoint.id" :tab="tabLabel(section)">
            <div class="section" :data-test="`report-section-${section.endpoint.kind}`">
              <p class="muted endpoint-line">
                {{ $t(`studyReport.direction.${section.endpoint.direction}`) }}
                <template v-if="section.endpoint.baselineTimepointId">
                  · {{ $t("studyReport.baseline", { name: timepointsById[section.endpoint.baselineTimepointId]?.name }) }}
                </template>
                · {{ text.paradigmName(section.endpoint.paradigmKey) }}
              </p>

              <NAlert v-if="!section.comparable" type="error" :title="$t('studyReport.notComparableTitle')" data-test="report-not-comparable">
                {{ $t("studyReport.notComparable") }}
                <ul>
                  <li v-for="v in section.versions" :key="`${v.paradigmVersion}-${v.metricEngineVersion}-${v.unit}`">
                    {{ $t("studyReport.version", { paradigm: v.paradigmVersion, engine: v.metricEngineVersion, unit: fmt.unitText(v.unit) || v.unit }) }}
                  </li>
                </ul>
              </NAlert>

              <h3>{{ $t("studyReport.groupsTitle", { timepoint: timepointsById[section.endpoint.timepointId]?.name }) }}</h3>
              <NDataTable
                size="small"
                :columns="statsColumns(section)"
                :data="section.groups"
                :row-key="(row) => row.groupId"
                :scroll-x="1180"
                :row-props="(row) => ({ 'data-test': 'report-group', 'data-group': groupsById[row.groupId]?.code })"
                data-test="report-groups"
              />

              <div class="split">
                <GroupChart
                  v-if="section.comparable"
                  :bars="bars(section)"
                  :format="(v) => fmt.metric(v, unitOf(section))"
                  :label="$t('studyReport.chartLabel', { metric: metricName(section) })"
                />
                <div class="comparisons" data-test="report-comparisons">
                  <h3>{{ $t("studyReport.comparisonsTitle") }}</h3>
                  <p v-if="!section.comparisons.length" class="muted">{{ $t("studyReport.noComparisons") }}</p>
                  <template v-for="kind in KINDS" :key="kind">
                    <div v-if="comparisonsBy(section, kind).length" class="kind" :data-test="`comparison-${kind}`">
                      <h4>{{ $t(`studyReport.comparisonKinds.${kind}`) }}</h4>
                      <p class="muted kind-hint">{{ $t(`studyReport.comparisonHints.${kind}`) }}</p>
                      <div v-for="c in comparisonsBy(section, kind)" :key="`${c.groupId}-${c.referenceGroupId}`" class="comparison">
                        <p>{{ compareText(section, c) }}</p>
                        <div v-if="kind !== 'MODEL_VALIDITY' && c.rescuePercent !== null" class="rescue" data-test="rescue">
                          <span class="rescue-label">{{ $t("studyReport.rescue", { value: fmt.number(c.rescuePercent) }) }}</span>
                          <span class="meter"><span :style="{ width: rescueWidth(c.rescuePercent) }"></span></span>
                        </div>
                      </div>
                    </div>
                  </template>
                </div>
              </div>

              <div v-if="section.doseResponse.length" class="dose" data-test="report-dose">
                <h3>{{ $t("studyReport.comparisonKinds.DOSE_RESPONSE") }}</h3>
                <div v-for="d in section.doseResponse" :key="d.substanceId" class="dose-block">
                  <p>
                    <b>{{ d.substanceName }}</b>: {{ doseTrend(d.points) }}
                  </p>
                  <div class="table-scroll">
                    <table>
                      <thead>
                        <tr>
                          <th>{{ $t("studyReport.dose.dose") }}</th>
                          <th>{{ $t("studyReport.group") }}</th>
                          <th class="num">n</th>
                          <th class="num">{{ $t("studyReport.mean") }}</th>
                          <th class="num">{{ $t("studyReport.dose.rescue") }}</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="p in d.points" :key="p.groupId">
                          <td>{{ fmt.dose(p.doseMicro, d.unit) }}</td>
                          <td>{{ groupName(p.groupId) }}</td>
                          <td class="num">{{ p.n }}</td>
                          <td class="num">{{ fmt.metric(p.mean, unitOf(section)) }}</td>
                          <td class="num">{{ p.rescuePercent === null ? "–" : `${fmt.number(p.rescuePercent)}%` }}</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>

              <div v-if="section.timeCourse.length > 1" class="course" data-test="report-course">
                <h3>{{ $t("studyReport.timeCourse") }}</h3>
                <div class="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>{{ $t("studyReport.group") }}</th>
                        <th v-for="tp in courseTimepoints(section)" :key="tp.id">
                          {{ tp.name }} <span class="muted">({{ $t("studyReport.day", { day: tp.studyDay }) }})</span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="row in courseRows(section)" :key="row.groupId">
                        <td>{{ groupName(row.groupId) }}</td>
                        <td v-for="tp in courseTimepoints(section)" :key="tp.id" class="num">{{ courseCell(section, row.cells[tp.id]) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <p class="muted footnote">{{ $t("studyReport.footnote") }}</p>
              <NTag v-for="v in section.versions" :key="`${v.paradigmVersion}-${v.metricEngineVersion}`" size="small" round :bordered="false" class="version">
                {{ $t("studyReport.version", { paradigm: v.paradigmVersion, engine: v.metricEngineVersion, unit: fmt.unitText(v.unit) || v.unit }) }}
              </NTag>
            </div>
          </NTabPane>
        </NTabs>
      </template>
    </NSpin>
  </div>
</template>

<style scoped>
.report { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.report-head { display: flex; gap: 12px; align-items: flex-start; flex-wrap: wrap; }
.banner { flex: 1; min-width: 260px; }
.blind-note { font-size: 13px; }
.section { display: flex; flex-direction: column; gap: 12px; padding-top: 6px; min-width: 0; }
.endpoint-line { margin: 0; }
h3 { margin: 6px 0 0; font-size: 14px; }
h4 { margin: 0; font-size: 13px; }
:deep(.group-cell) { display: inline-flex; flex-direction: column; line-height: 1.3; }
:deep(.count) { font-weight: 700; }
:deep(.tone-warning) { color: var(--warn); }
:deep(.tone-error) { color: var(--bad); }
.split { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 20px; align-items: start; }
@media (max-width: 900px) { .split { grid-template-columns: minmax(0, 1fr); } }
.comparisons { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.kind { display: flex; flex-direction: column; gap: 6px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel2); }
.kind-hint { margin: 0; font-size: 12px; }
.comparison p { margin: 0; font-size: 13.5px; }
.rescue { display: flex; align-items: center; gap: 10px; margin-top: 4px; }
.rescue-label { font-size: 12.5px; font-weight: 600; white-space: nowrap; }
.meter { flex: 1; height: 8px; border-radius: 99px; background: color-mix(in srgb, var(--line) 60%, transparent); overflow: hidden; }
.meter span { display: block; height: 100%; background: var(--accent); }
.dose-block p { margin: 0 0 6px; font-size: 13.5px; }
.num { text-align: right; font-variant-numeric: tabular-nums; }
.footnote { margin: 4px 0 0; font-size: 12px; }
.version { margin-right: 6px; }
</style>
