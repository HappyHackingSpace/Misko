<script setup>
// The result of one recording: which run to read, the numbers that matter, the
// paired videos and the events. A recording can have several runs (a
// reanalysis, a failed attempt), so the run is chosen from a list instead of
// being laid out as a wall of buttons.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NCollapse, NCollapseItem, NDescriptions, NDescriptionsItem, NSelect, NTag } from "naive-ui";
import { eventColors } from "../../composables/eventColors.js";
import { filterByText, selectOption } from "../../composables/selectOptions.js";
import EventTimeline from "../EventTimeline.vue";
import VideoPairPanel from "../VideoPairPanel.vue";

const props = defineProps({
  // The runs of the chosen recording, newest first.
  runs: { type: Array, default: () => [] },
  runId: { type: String, default: "" },
  // The chosen run with its published result, or null.
  detail: { type: Object, default: null },
  tagType: { type: Function, required: true },
});
const emit = defineEmits(["select-run", "error"]);
const { t, te } = useI18n();

const succeeded = computed(() => props.detail?.status === "SUCCEEDED");
const metrics = computed(() => props.detail?.result?.metrics || []);
const events = computed(() => props.detail?.result?.events || []);

// Published metrics carry full floating-point precision, which reads as noise
// next to a unit; three decimal places is more than the tracking accuracy
// justifies. Trailing zeros are dropped so a whole number still reads as one.
function formatNumber(value) {
  if (typeof value !== "number" || !Number.isFinite(value)) return value;
  if (Number.isInteger(value)) return String(value);
  return value.toFixed(3).replace(/0+$/, "").replace(/\.$/, "");
}
function metricValue(metric) {
  if (metric.value === null) return t("analysis.missing");
  // A ratio is read as a share of the test, not as a bare fraction.
  if (metric.unit === "ratio") return `${formatNumber(Math.round(metric.value * 1000) / 10)}%`;
  return `${formatNumber(metric.value)} ${metric.unit}`;
}
const metricLabel = (key) => (te(`analysis.metricLabels.${key}`) ? t(`analysis.metricLabels.${key}`) : key);

// The numbers a reader looks for first. A paradigm that has none of them shows
// its first metrics instead, so a new paradigm needs no change here.
const PREFERRED = ["distance_cm", "mean_speed_cm_s", "center_time_ratio", "immobility_s"];
const headline = computed(() => {
  const byKey = Object.fromEntries(metrics.value.map((m) => [m.key, m]));
  const chosen = PREFERRED.map((key) => byKey[key]).filter(Boolean);
  for (const metric of metrics.value) {
    if (chosen.length >= 4) break;
    if (!chosen.includes(metric)) chosen.push(metric);
  }
  return chosen.slice(0, 4);
});

const runOptions = computed(() =>
  props.runs.map((run, index) => {
    const when = run.finishedAt ? ` · ${new Date(run.finishedAt).toLocaleString()}` : "";
    const text = `${t(`analysis.${run.trigger}`)} · ${t(`statuses.${run.status}`)}${when}`;
    return selectOption("run-select", run.id, text, index === 0 ? t("environments.latest") : "");
  }),
);
const currentRun = computed(() => props.runs.find((run) => run.id === props.runId) || null);

// The events, by type: filter chips say how many there are of each, and the
// strip shows where in the recording they happen.
const colors = computed(() => eventColors(events.value));
const types = computed(() => {
  const counts = {};
  for (const event of events.value) counts[event.type] = (counts[event.type] || 0) + 1;
  return Object.entries(counts).map(([type, count]) => ({ type, count }));
});
const hidden = ref(new Set());
const shownTypes = computed(() => types.value.map((x) => x.type).filter((type) => !hidden.value.has(type)));
function toggle(type) {
  const next = new Set(hidden.value);
  if (next.has(type)) next.delete(type);
  else next.add(type);
  hidden.value = next;
}
const eventLabel = (type) => (te(`events.${type}`) ? t(`events.${type}`) : type);

const selectedEvent = ref(null);
// Another run is another set of events: nothing of the last one carries over.
watch(
  () => props.runId,
  () => {
    selectedEvent.value = null;
    hidden.value = new Set();
  },
);

const durationUs = computed(() => {
  const seconds = metrics.value.find((m) => m.key === "duration_s")?.value;
  const latest = events.value.reduce((max, e) => Math.max(max, e.endUs), 0);
  return Math.max(latest, typeof seconds === "number" ? seconds * 1_000_000 : 0) || 1;
});
// One lane per event type, so intervals of one kind cannot hide the points of another.
const lanes = computed(() =>
  types.value
    .filter((entry) => shownTypes.value.includes(entry.type))
    .map((entry) => ({
      type: entry.type,
      marks: events.value
        .filter((event) => event.type === entry.type)
        .map((event) => ({
          event,
          style: {
            left: `${(event.startUs / durationUs.value) * 100}%`,
            // A point has no length; it still needs something to click.
            width: `${Math.max(0.5, ((event.endUs - event.startUs) / durationUs.value) * 100)}%`,
            "--dot": colors.value[event.type],
          },
        })),
    })),
);

// Minutes and seconds along the recording, for reading where a mark sits.
const ticks = computed(() =>
  [0, 0.25, 0.5, 0.75, 1].map((share) => {
    const total = Math.round((durationUs.value * share) / 1_000_000);
    return { share, label: `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}` };
  }),
);
const isSelected = (event) => selectedEvent.value?.startUs === event.startUs && selectedEvent.value?.type === event.type;
</script>

<template>
  <div class="result" data-test="result" :data-run-id="runId">
    <div class="bar" v-if="runs.length">
      <span class="label">{{ $t("analysis.run") }}</span>
      <NSelect
        class="pick"
        :value="runId"
        data-test="run-select"
        :options="runOptions"
        :filter="filterByText"
        @update:value="emit('select-run', $event)"
      />
      <NTag v-if="currentRun" size="small" round :bordered="false" :type="tagType(currentRun.status)" data-test="run-status">
        {{ $t(`statuses.${currentRun.status}`) }}
      </NTag>
    </div>
    <p v-else class="muted" data-test="no-runs">{{ $t("analysis.noRuns") }}</p>

    <template v-if="detail">
      <p v-if="detail.status === 'QUEUED'" class="muted hint" data-test="run-hint">{{ $t("analysis.queuedHint") }}</p>
      <p v-else-if="detail.status === 'RUNNING'" class="muted hint" data-test="run-hint">{{ $t("analysis.runningHint") }}</p>
      <NAlert v-else-if="detail.status === 'FAILED'" type="error" data-test="run-failure" style="margin-top: 10px">
        {{ $t("analysis.failure") }}: {{ detail.failureReason }}
      </NAlert>

      <template v-if="succeeded">
        <div class="kpis" data-test="metrics-headline">
          <div class="kpi" v-for="metric in headline" :key="metric.key">
            <span class="kpi-label">{{ metricLabel(metric.key) }}</span>
            <span class="kpi-value" :data-test="`metric-${metric.key}`">{{ metricValue(metric) }}</span>
          </div>
        </div>

        <VideoPairPanel :run-id="runId" :play="selectedEvent" @error="emit('error', $event)" />

        <section class="events">
          <h4>{{ $t("analysis.events") }}</h4>
          <div class="chips" v-if="types.length">
            <button
              v-for="entry in types"
              :key="entry.type"
              type="button"
              class="chip"
              :class="{ off: hidden.has(entry.type) }"
              :style="{ '--dot': colors[entry.type] }"
              :aria-pressed="!hidden.has(entry.type)"
              :data-test="`event-filter-${entry.type}`"
              @click="toggle(entry.type)"
            >
              <span class="dot"></span>{{ eventLabel(entry.type) }} <b>{{ entry.count }}</b>
            </button>
          </div>
          <!-- Where in the recording the events happen. Each mark plays its event. -->
          <div class="lanes" v-if="events.length" data-test="event-strip">
            <div class="lane" v-for="lane in lanes" :key="lane.type">
              <span class="lane-label">{{ eventLabel(lane.type) }}</span>
              <div class="track">
                <button
                  v-for="(mark, i) in lane.marks"
                  :key="`${mark.event.type}-${mark.event.startUs}-${i}`"
                  type="button"
                  class="mark"
                  :class="{ selected: isSelected(mark.event) }"
                  :style="mark.style"
                  :title="eventLabel(mark.event.type)"
                  data-test="event-mark"
                  @click="selectedEvent = mark.event"
                ></button>
              </div>
            </div>
            <div class="lane axis">
              <span class="lane-label"></span>
              <div class="track ticks">
                <span v-for="tick in ticks" :key="tick.share" class="tick" :style="{ left: `${tick.share * 100}%` }" :data-edge="tick.share === 1 ? 'end' : tick.share === 0 ? 'start' : 'mid'">{{ tick.label }}</span>
              </div>
            </div>
          </div>
          <EventTimeline :events="events" :selected="selectedEvent" :types="shownTypes" @play="selectedEvent = $event" />
        </section>

        <NCollapse class="all-metrics">
          <NCollapseItem :title="`${$t('analysis.allMetrics')} (${metrics.length})`" name="metrics">
            <NDescriptions :column="1" label-placement="left" size="small" bordered data-test="metrics">
              <NDescriptionsItem v-for="metric in metrics" :key="metric.key" :label="metric.key">
                <span :data-test="`metric-row-${metric.key}`">{{ metricValue(metric) }}</span>
              </NDescriptionsItem>
            </NDescriptions>
          </NCollapseItem>
        </NCollapse>
      </template>
    </template>
  </div>
</template>

<style scoped>
.result { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
.bar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.label { font-size: 12px; color: var(--muted); }
.pick { width: 360px; max-width: 100%; }
.hint { margin: 0; }
.kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 10px; }
.kpi { background: var(--panel2); border-radius: 10px; padding: 10px 12px; display: flex; flex-direction: column; gap: 2px; }
.kpi-label { font-size: 12px; color: var(--muted); }
.kpi-value { font-size: 20px; font-weight: 700; font-variant-numeric: tabular-nums; }
.events h4 { margin: 0 0 8px; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }
.chips { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 8px; }
.chip {
  display: inline-flex; align-items: center; gap: 6px; padding: 3px 10px; border-radius: 99px; font-size: 12px; font-weight: 600;
  background: var(--panel2); border: 1px solid var(--line);
}
.chip.off { opacity: .45; }
.chip b { color: var(--muted); font-weight: 600; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--dot); }
.lanes { display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; padding: 8px 10px; background: var(--panel2); border: 1px solid var(--line); border-radius: 10px; }
.lane { display: grid; grid-template-columns: 110px minmax(0, 1fr); align-items: center; gap: 10px; }
.lane-label { font-size: 11.5px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.track { position: relative; height: 16px; background: color-mix(in srgb, var(--line) 35%, transparent); border-radius: 4px; }
.mark {
  position: absolute; top: 2px; height: 12px; min-width: 4px; padding: 0; border: 0; border-radius: 3px;
  background: var(--dot); opacity: .9;
}
.mark:hover, .mark.selected { opacity: 1; outline: 2px solid var(--dot); outline-offset: 1px; z-index: 1; }
.axis .track { background: transparent; height: 14px; }
.tick { position: absolute; top: 0; font-size: 10.5px; color: var(--muted); font-variant-numeric: tabular-nums; transform: translateX(-50%); }
.tick[data-edge="start"] { transform: none; }
.tick[data-edge="end"] { transform: translateX(-100%); }
.all-metrics { margin-top: 2px; }
</style>
