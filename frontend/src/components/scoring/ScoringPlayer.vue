<script setup>
// Observer scoring of one animal on the test's current video: a player with
// frame stepping and playback speed, one button and shortcut per input event
// of the paradigm (points at the current time, intervals started and stopped,
// one button per label), the scored events on a timeline and in an editable
// list, and completing the sheet, which computes the result for review.
// Times are microseconds from the clip start of the video.
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  NAlert, NButton, NCheckbox, NCheckboxGroup, NDataTable, NInputNumber, NPopconfirm, NSelect, NSpace, NSpin, NTag, useMessage,
} from "naive-ui";
import { scoring } from "../../api/scoring.js";
import { recordings } from "../../api/media.js";
import ScoringTimeline from "./ScoringTimeline.vue";

const props = defineProps({
  testId: { type: String, required: true },
  subject: { type: Object, required: true },
  canRun: { type: Boolean, default: false },
});
// "changed" after completing or reopening, so the result is loaded again.
const emit = defineEmits(["changed"]);
const { t, te, locale } = useI18n();
const message = useMessage();

const sheet = ref(null);
const recording = ref(null);
const videoUrl = ref("");
const video = ref(null);
const loading = ref(true);
const busy = ref("");
const error = ref("");

const playing = ref(false);
const rate = ref(1);
const positionUs = ref(0); // media time of the video element, microseconds
const mediaDurationUs = ref(0);
const frameUs = ref(1_000_000 / 30); // estimated while playing
const running = ref({}); // lane key -> start in microseconds
const selected = ref("");
const editing = ref(null); // { id, start, end, label } in seconds
const scoredTypes = ref([]);

const draft = computed(() => sheet.value?.status === "DRAFT");
const writable = computed(() => props.canRun && draft.value);
const inputEvents = computed(() => sheet.value?.inputEvents || []);
const events = computed(() => [...(sheet.value?.events || [])].sort((a, b) => a.startUs - b.startUs));
const clipStartUs = computed(() => recording.value?.clipStartUs || 0);
const durationUs = computed(() => {
  if (sheet.value?.durationUs) return sheet.value.durationUs;
  if (recording.value?.clipEndUs) return recording.value.clipEndUs - clipStartUs.value;
  return Math.max(mediaDurationUs.value - clipStartUs.value, 0);
});
const currentUs = computed(() => Math.max(positionUs.value - clipStartUs.value, 0));

const labelName = (label) => (te(`scoring.labels.${label}`) ? t(`scoring.labels.${label}`) : label);
const definition = (type) => inputEvents.value.find((d) => d.type === type);
const laneKey = (type, label) => `${type}|${label || ""}`;

// One action per event type and label, each with a shortcut key 1 to 9, then 0.
const KEYS = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"];
const actions = computed(() =>
  inputEvents.value.flatMap((def) => (def.labels?.length ? def.labels : [""]).map((label) => ({ def, label })))
    .map((a, i) => ({
      ...a,
      key: laneKey(a.def.type, a.label),
      hotkey: KEYS[i] || "",
      title: a.label ? `${a.def.label} (${labelName(a.label)})` : a.def.label,
    })),
);
const lanes = computed(() => actions.value.map((a) => ({ key: a.key, title: a.title })));

// Why an event cannot be completed as it is, or "".
function problem(e) {
  const def = definition(e.type);
  if (e.startUs < 0) return t("scoring.hints.beforeStart");
  if (def?.kind === "INTERVAL" && !(e.endUs > e.startUs)) return t("scoring.hints.endBeforeStart");
  const end = def?.kind === "INTERVAL" ? e.endUs : e.startUs;
  if (durationUs.value && end > durationUs.value) return t("scoring.hints.afterEnd");
  return "";
}
const timelineEvents = computed(() =>
  events.value.map((e) => ({ id: e.id, lane: laneKey(e.type, e.label), kind: e.kind, startUs: e.startUs, endUs: e.endUs ?? e.startUs, invalid: !!problem(e) })),
);
const runningList = computed(() => Object.entries(running.value).map(([lane, startUs]) => ({ lane, startUs })));
// A type with scored events was necessarily scored, so it stays ticked.
const typesWithEvents = computed(() => new Set(events.value.map((e) => e.type)));
watch(typesWithEvents, (types) => {
  const missing = [...types].filter((ty) => !scoredTypes.value.includes(ty));
  if (missing.length) scoredTypes.value = [...scoredTypes.value, ...missing];
});
const invalidCount = computed(() => events.value.filter((e) => problem(e)).length);

// Microseconds as seconds rounded to the millisecond, for editing.
const seconds = (us) => Math.round(us / 1000) / 1000;

// Times as m:ss.mmm with the decimal separator of the panel language.
function clock(us) {
  const total = Math.max(us || 0, 0) / 1_000_000;
  const minutes = Math.floor(total / 60);
  const seconds = (total - minutes * 60).toLocaleString(locale.value, { minimumIntegerDigits: 2, minimumFractionDigits: 3, maximumFractionDigits: 3 });
  return `${minutes}:${seconds}`;
}

async function loadSheet() {
  sheet.value = await scoring.get(props.testId, props.subject.id);
  const types = inputEvents.value.map((d) => d.type);
  scoredTypes.value = sheet.value.scoredTypes?.length ? [...sheet.value.scoredTypes] : types;
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [, list] = await Promise.all([loadSheet(), recordings.list(props.testId)]);
    const rows = list.data || [];
    recording.value = rows.find((r) => r.current) || rows.findLast((r) => r.video.status === "VERIFIED") || null;
    if (recording.value) videoUrl.value = (await recordings.readUrl(props.testId, recording.value.id)).url;
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

// Player.
let frameRequest = 0;
let lastMediaTime = null;
function trackFrames(_now, meta) {
  if (lastMediaTime !== null) {
    const delta = (meta.mediaTime - lastMediaTime) * 1_000_000;
    // The shortest step between presented frames is one frame; dropped frames only make steps longer.
    if (delta > 1000 && delta < frameUs.value * 0.98) frameUs.value = delta;
  }
  lastMediaTime = meta.mediaTime;
  positionUs.value = Math.round(meta.mediaTime * 1_000_000);
  if (video.value && !video.value.paused) frameRequest = video.value.requestVideoFrameCallback(trackFrames);
}
let animation = 0;
function tick() {
  if (!video.value) return;
  positionUs.value = Math.round(video.value.currentTime * 1_000_000);
  if (!video.value.paused) animation = requestAnimationFrame(tick);
}
function onPlay() {
  playing.value = true;
  lastMediaTime = null;
  if (video.value?.requestVideoFrameCallback) frameRequest = video.value.requestVideoFrameCallback(trackFrames);
  else animation = requestAnimationFrame(tick);
}
function onPause() {
  playing.value = false;
  if (video.value) positionUs.value = Math.round(video.value.currentTime * 1_000_000);
}
function onMetadata() {
  mediaDurationUs.value = Math.floor((video.value.duration || 0) * 1_000_000);
  video.value.playbackRate = rate.value;
  if (clipStartUs.value) video.value.currentTime = clipStartUs.value / 1_000_000;
}
function onTime() {
  if (video.value?.paused) positionUs.value = Math.round(video.value.currentTime * 1_000_000);
}
function toggle() {
  if (!video.value) return;
  if (video.value.paused) video.value.play();
  else video.value.pause();
}
// Seeks to a time from the clip start, clamped to the clip.
function seek(us) {
  if (!video.value) return;
  const clamped = Math.min(Math.max(us, 0), durationUs.value || us);
  video.value.currentTime = (clipStartUs.value + clamped) / 1_000_000;
  positionUs.value = clipStartUs.value + clamped;
}
function step(frames) {
  if (!video.value) return;
  video.value.pause();
  // Land in the middle of the target frame so rounding never shows the neighbour.
  const frame = Math.floor(currentUs.value / frameUs.value) + frames;
  seek(Math.max(frame, 0) * frameUs.value + (frame > 0 ? frameUs.value / 2 : 0));
}
watch(rate, (value) => {
  if (video.value) video.value.playbackRate = value;
});
const rates = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 2].map((value) => ({ value, label: `${value.toLocaleString(locale.value)}x` }));

// Scoring.
async function mutate(name, run) {
  busy.value = name;
  error.value = "";
  try {
    await run();
    await loadSheet();
    return true;
  } catch (e) {
    error.value = e.message;
    return false;
  } finally {
    busy.value = "";
  }
}

// Adds an event, or moves the existing one of a single event type.
function record(def, label, startUs, endUs) {
  const body = { type: def.type, startUs };
  if (label) body.label = label;
  if (def.kind === "INTERVAL") body.endUs = endUs;
  const existing = def.single ? events.value.find((e) => e.type === def.type) : null;
  return mutate("record", async () => {
    if (existing) {
      await scoring.change(props.testId, props.subject.id, existing.id, { ...body, endUs: def.kind === "INTERVAL" ? endUs : startUs });
      message.info(t("scoring.replaced", { event: def.label }));
    } else {
      await scoring.add(props.testId, props.subject.id, body);
    }
  });
}

async function trigger(action) {
  if (!writable.value || busy.value) return;
  const at = currentUs.value;
  if (action.def.kind !== "INTERVAL") {
    await record(action.def, action.label, at);
    return;
  }
  const started = running.value[action.key];
  if (started === undefined) {
    running.value = { ...running.value, [action.key]: at };
    return;
  }
  if (at <= started) {
    error.value = t("scoring.hints.endBeforeStart");
    return;
  }
  cancelRunning(action.key);
  await record(action.def, action.label, started, at);
}

function cancelRunning(key) {
  const rest = { ...running.value };
  delete rest[key];
  running.value = rest;
}

function select(id) {
  selected.value = id;
  const e = events.value.find((x) => x.id === id);
  if (e) seek(e.startUs);
}

function edit(e) {
  editing.value = { id: e.id, start: seconds(e.startUs), end: seconds(e.endUs ?? e.startUs), label: e.label || "" };
  selected.value = e.id;
}
const editProblem = computed(() => {
  const ed = editing.value;
  if (!ed) return "";
  const e = events.value.find((x) => x.id === ed.id);
  if (ed.start === null || (e?.kind === "INTERVAL" && ed.end === null)) return t("scoring.hints.missingTime");
  return problem({ type: e?.type, startUs: Math.round(ed.start * 1_000_000), endUs: Math.round(ed.end * 1_000_000) });
});
async function saveEdit() {
  const ed = editing.value;
  const e = events.value.find((x) => x.id === ed.id);
  const startUs = Math.round(ed.start * 1_000_000);
  const body = { type: e.type, startUs, endUs: e.kind === "INTERVAL" ? Math.round(ed.end * 1_000_000) : startUs };
  if (definition(e.type)?.labels?.length) body.label = ed.label;
  if (await mutate("edit", () => scoring.change(props.testId, props.subject.id, ed.id, body))) editing.value = null;
}
async function remove(e) {
  if (await mutate("remove", () => scoring.remove(props.testId, props.subject.id, e.id))) {
    if (selected.value === e.id) selected.value = "";
    if (editing.value?.id === e.id) editing.value = null;
  }
}

async function complete() {
  const body = { scoredTypes: scoredTypes.value };
  if (durationUs.value) body.durationUs = durationUs.value;
  busy.value = "complete";
  error.value = "";
  try {
    await scoring.complete(props.testId, props.subject.id, body);
    await loadSheet();
    message.success(t("scoring.completed"));
    emit("changed");
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = "";
  }
}
async function reopen() {
  busy.value = "reopen";
  error.value = "";
  try {
    await scoring.reopen(props.testId, props.subject.id);
    await loadSheet();
    message.success(t("scoring.reopened"));
    emit("changed");
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = "";
  }
}

// Keyboard: space plays, arrows step a frame (with shift one second), digits score.
function onKey(event) {
  if (event.ctrlKey || event.metaKey || event.altKey) return;
  const target = event.target;
  if (target?.closest?.("input, textarea, select, [contenteditable='true']")) return;
  if (event.key === " ") {
    event.preventDefault();
    toggle();
  } else if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
    event.preventDefault();
    const sign = event.key === "ArrowLeft" ? -1 : 1;
    if (event.shiftKey) seek(currentUs.value + sign * 1_000_000);
    else step(sign);
  } else {
    const action = actions.value.find((a) => a.hotkey && a.hotkey === event.key);
    if (action) {
      event.preventDefault();
      trigger(action);
    }
  }
}

const columns = computed(() => [
  {
    title: t("scoring.list.event"),
    key: "type",
    minWidth: 120,
    render: (e) => h("span", definition(e.type)?.label || e.type),
  },
  {
    title: t("scoring.list.label"),
    key: "label",
    width: 120,
    render: (e) => {
      const labels = definition(e.type)?.labels || [];
      if (editing.value?.id === e.id && labels.length) {
        return h(NSelect, {
          size: "small",
          value: editing.value.label,
          options: labels.map((l) => ({ value: l, label: labelName(l) })),
          "onUpdate:value": (v) => (editing.value.label = v),
        });
      }
      return e.label ? labelName(e.label) : "";
    },
  },
  timeColumn("start", t("scoring.list.start")),
  timeColumn("end", t("scoring.list.end")),
  {
    title: "",
    key: "actions",
    width: 170,
    render: (e) => {
      const hint = problem(e);
      const parts = [];
      const shown = editing.value?.id === e.id ? editProblem.value : hint;
      if (shown) parts.push(h("span", { class: "hint", "data-test": "event-hint" }, shown));
      if (!writable.value) return h("div", { class: "row-actions" }, parts);
      if (editing.value?.id === e.id) {
        parts.push(
          h(NButton, { size: "tiny", onClick: () => (editing.value = null) }, () => t("common.cancel")),
          h(NButton, { size: "tiny", type: "primary", disabled: !!editProblem.value, loading: busy.value === "edit", "data-test": "event-save", onClick: saveEdit }, () => t("scoring.list.save")),
        );
      } else {
        parts.push(
          h(NButton, { size: "tiny", secondary: true, "data-test": "event-edit", onClick: () => edit(e) }, () => t("scoring.list.edit")),
          h(NPopconfirm, { onPositiveClick: () => remove(e) }, {
            trigger: () => h(NButton, { size: "tiny", secondary: true, type: "error", "data-test": "event-delete" }, () => t("scoring.list.delete")),
            default: () => t("scoring.list.deleteConfirm"),
          }),
        );
      }
      return h("div", { class: "row-actions" }, parts);
    },
  },
]);

function timeColumn(which, title) {
  return {
    title,
    key: which,
    width: 150,
    render: (e) => {
      const isEnd = which === "end";
      if (isEnd && e.kind !== "INTERVAL") return "";
      const us = isEnd ? e.endUs : e.startUs;
      if (editing.value?.id === e.id) {
        return h("div", { class: "time-edit" }, [
          h(NInputNumber, {
            size: "small",
            value: editing.value[which],
            min: 0,
            step: frameUs.value / 1_000_000,
            showButton: false,
            style: "width: 90px",
            "data-test": `event-${which}`,
            "onUpdate:value": (v) => (editing.value[which] = v),
          }),
          h(NButton, { size: "tiny", quaternary: true, title: t("scoring.list.nowHint"), onClick: () => (editing.value[which] = seconds(currentUs.value)) }, () => t("scoring.list.now")),
        ]);
      }
      return h("a", { class: "time", onClick: () => seek(us), "data-test": `event-${which}-time` }, clock(us));
    },
  };
}
const rowProps = (e) => ({ class: e.id === selected.value ? "selected-row" : "", "data-test": "scored-event", "data-type": e.type });

watch(locale, () => loadSheet().catch((e) => (error.value = e.message)));
watch(() => props.subject.id, () => {
  running.value = {};
  editing.value = null;
  load();
});
onMounted(() => {
  load();
  window.addEventListener("keydown", onKey);
});
onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKey);
  cancelAnimationFrame(animation);
  if (frameRequest && video.value?.cancelVideoFrameCallback) video.value.cancelVideoFrameCallback(frameRequest);
});
defineExpose({ hasRunning: () => runningList.value.length > 0 });
</script>

<template>
  <div class="scoring" data-test="scoring-player" :data-status="sheet?.status || ''">
    <NSpin :show="loading">
      <div class="head">
        <NTag v-if="sheet" size="small" round :bordered="false" :type="draft ? 'warning' : 'success'" data-test="scoring-status">
          {{ $t(`scoring.statuses.${sheet.status}`) }}
        </NTag>
        <span v-if="sheet" class="muted small">{{ $t("scoring.version", { n: sheet.version }) }}</span>
      </div>

      <p v-if="!loading && !recording" class="muted" data-test="scoring-no-video">{{ $t("scoring.noVideo") }}</p>

      <template v-if="recording && videoUrl">
        <video
          ref="video"
          class="player"
          :src="videoUrl"
          preload="auto"
          playsinline
          data-test="scoring-video"
          @loadedmetadata="onMetadata"
          @play="onPlay"
          @pause="onPause"
          @ended="onPause"
          @seeked="onTime"
          @timeupdate="onTime"
          @click="toggle"
        ></video>

        <div class="controls">
          <NButton size="small" secondary :title="$t('scoring.player.frameBack')" data-test="frame-back" @click="step(-1)">‹ {{ $t("scoring.player.frame") }}</NButton>
          <NButton size="small" type="primary" data-test="play" @click="toggle">{{ playing ? $t("scoring.player.pause") : $t("scoring.player.play") }}</NButton>
          <NButton size="small" secondary :title="$t('scoring.player.frameForward')" data-test="frame-forward" @click="step(1)">{{ $t("scoring.player.frame") }} ›</NButton>
          <label class="speed">
            <span class="muted small">{{ $t("scoring.player.speed") }}</span>
            <NSelect v-model:value="rate" size="small" :options="rates" style="width: 90px" data-test="speed" />
          </label>
          <span class="clock" data-test="clock">{{ clock(currentUs) }} / {{ clock(durationUs) }}</span>
        </div>
        <p class="muted small keys">{{ $t("scoring.player.keys") }}</p>
      </template>

      <NAlert v-if="error" type="error" :title="error" data-test="scoring-error" style="margin: 8px 0" />

      <template v-if="sheet">
        <div v-if="writable" class="actions" data-test="scoring-actions">
          <NButton
            v-for="a in actions"
            :key="a.key"
            size="small"
            :type="running[a.key] !== undefined ? 'warning' : 'default'"
            :disabled="!recording || !!busy"
            :title="a.def.definition"
            data-test="score-button"
            :data-type="a.def.type"
            :data-label="a.label"
            @click="trigger(a)"
          >
            <kbd v-if="a.hotkey">{{ a.hotkey }}</kbd>
            <template v-if="a.def.kind === 'INTERVAL'">
              {{ running[a.key] !== undefined ? $t("scoring.stop", { event: a.title }) : $t("scoring.start", { event: a.title }) }}
            </template>
            <template v-else>{{ a.title }}</template>
          </NButton>
        </div>
        <div v-for="r in runningList" :key="r.lane" class="running" data-test="running-interval">
          <span>{{ $t("scoring.running", { event: lanes.find((l) => l.key === r.lane)?.title, from: clock(r.startUs), length: clock(Math.max(currentUs - r.startUs, 0)) }) }}</span>
          <NButton size="tiny" quaternary @click="cancelRunning(r.lane)">{{ $t("common.cancel") }}</NButton>
        </div>

        <ScoringTimeline
          v-if="lanes.length"
          :lanes="lanes"
          :events="timelineEvents"
          :running="runningList"
          :duration-us="durationUs"
          :current-us="currentUs"
          :selected="selected"
          @seek="seek"
          @select="select"
        />

        <h4>{{ $t("scoring.list.title", { count: events.length }) }}</h4>
        <NDataTable
          size="small"
          :columns="columns"
          :data="events"
          :row-key="(e) => e.id"
          :row-props="rowProps"
          :scroll-x="640"
          data-test="scored-events"
        >
          <template #empty><span class="muted">{{ $t("scoring.list.empty") }}</span></template>
        </NDataTable>

        <div class="finish" data-test="scoring-finish">
          <template v-if="writable">
            <strong>{{ $t("scoring.scoredTypes") }}</strong>
            <p class="muted small">{{ $t("scoring.scoredTypesHint") }}</p>
            <NCheckboxGroup v-model:value="scoredTypes">
              <NSpace>
                <NCheckbox v-for="d in inputEvents" :key="d.type" :value="d.type" :label="d.label" :disabled="typesWithEvents.has(d.type)" data-test="scored-type" />
              </NSpace>
            </NCheckboxGroup>
            <p v-if="runningList.length" class="hint small">{{ $t("scoring.hints.running") }}</p>
            <p v-else-if="invalidCount" class="hint small">{{ $t("scoring.hints.invalid", { count: invalidCount }) }}</p>
            <NPopconfirm :disabled="!scoredTypes.length || runningList.length > 0 || invalidCount > 0" @positive-click="complete">
              <template #trigger>
                <NButton
                  type="primary"
                  :loading="busy === 'complete'"
                  :disabled="!!busy || !scoredTypes.length || runningList.length > 0 || invalidCount > 0"
                  data-test="scoring-complete"
                >
                  {{ $t("scoring.complete") }}
                </NButton>
              </template>
              {{ $t("scoring.completeConfirm") }}
            </NPopconfirm>
          </template>
          <template v-else-if="sheet.status === 'COMPLETED'">
            <p class="muted small" data-test="scoring-completed-hint">
              {{ $t("scoring.completedHint", { types: (sheet.scoredTypes || []).map((ty) => definition(ty)?.label || ty).join(", ") }) }}
            </p>
            <NPopconfirm v-if="canRun" @positive-click="reopen">
              <template #trigger>
                <NButton secondary :loading="busy === 'reopen'" :disabled="!!busy" data-test="scoring-reopen">{{ $t("scoring.reopen") }}</NButton>
              </template>
              {{ $t("scoring.reopenConfirm") }}
            </NPopconfirm>
          </template>
        </div>
      </template>
    </NSpin>
  </div>
</template>

<style scoped>
.scoring { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.head { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
.player { width: 100%; max-height: 52vh; border-radius: 10px; background: var(--panel2); display: block; }
.controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 8px; }
.speed { display: flex; align-items: center; gap: 6px; }
.clock { font-variant-numeric: tabular-nums; margin-left: auto; }
.keys { margin: 4px 0 8px; }
.small { font-size: 12px; margin: 0; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; margin: 8px 0; }
kbd { font-family: inherit; font-size: 11px; border: 1px solid var(--line); border-radius: 4px; padding: 0 4px; margin-right: 6px; color: var(--muted); }
.running { display: flex; align-items: center; gap: 8px; color: var(--warn); font-size: 13px; font-variant-numeric: tabular-nums; }
h4 { margin: 16px 0 8px; font-size: 14px; }
.finish { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--line); }
.hint { color: var(--warn); }
:deep(.hint) { color: var(--warn); font-size: 12px; }
:deep(.row-actions) { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
:deep(.time-edit) { display: flex; align-items: center; gap: 4px; }
:deep(.time) { cursor: pointer; font-variant-numeric: tabular-nums; color: var(--txt); text-decoration: underline dotted; }
:deep(.selected-row td) { background: var(--active-bg) !important; }
</style>
