<script setup>
// The workflow of one test: upload a recording, watch its calibration and
// analysis status, then read the published result. Picking an event plays that
// interval in the paired video panel beside the timeline.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { analysis, calibration, recordings, tests } from "../api/endpoints.js";
import { crc32c, upload } from "../api/upload.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import EventTimeline from "../components/EventTimeline.vue";
import VideoPairPanel from "../components/VideoPairPanel.vue";

const { t } = useI18n();
const route = useRoute();
const auth = useAuth();
const crumb = useBreadcrumb();

const test = ref(null);
const trials = ref([]);
const recordingList = ref([]);
const calibrationStatus = ref({});
const runs = ref([]);
const detail = ref(null);
const selectedRunId = ref("");
const selectedEvent = ref(null);
const error = ref("");
const busy = ref("");
const progress = ref(0);
const clip = ref({ startS: 0, endS: "" });

const testId = computed(() => route.params.id);

// Starting, completing and recording a trial are running the test, which is
// test:run. Cancelling is test:write, because it retracts a planned record
// rather than carrying it out.
const canCancel = computed(() => auth.canWriteTests);
const planned = computed(() => test.value?.status === "PLANNED");
const inProgress = computed(() => test.value?.status === "IN_PROGRESS");
const newTrial = ref({ repetition: 1, startedAt: "", endedAt: "", notes: "" });
// Once the technician edits the start, the screen stops refreshing it.
const touchedStart = ref(false);
const cancelReason = ref("");

// The datetime fields speak local time; the API stores instants. Seconds are
// kept, because a test starts partway through a minute and a trial may not
// start before its test did: a field rounded to the minute would force the
// technician to postdate the trial and then wait to complete the test.
function localValue(date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 19);
}

// The start a new trial is offered: the current second, and never before the
// test's own start, which the API refuses. The field holds nothing finer than a
// second, and dropping the fraction rounds downwards, so within the second the
// test started in the test's own start is rounded upwards instead. A moment
// later the current second wins and the trial is recorded in the past, which is
// what lets the test be completed straight after.
function defaultTrialStart() {
  const second = 1000;
  const now = Math.floor(Date.now() / second) * second;
  const started = test.value?.startedAt ? Math.ceil(new Date(test.value.startedAt).getTime() / second) * second : 0;
  return localValue(new Date(Math.max(now, started)));
}

const instant = (value) => (value ? new Date(value).toISOString() : null);

const trialLabel = (trial) =>
  t("tests.trialLabel", { number: trial.number, repetition: trial.repetition, attempt: trial.attempt });

async function act(name, run) {
  busy.value = name;
  error.value = "";
  try {
    await run();
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = "";
  }
}

const startTest = () => act("start", () => tests.start(testId.value));
const completeTest = () => act("complete", () => tests.complete(testId.value));
const cancelTest = () => act("cancel", () => tests.cancel(testId.value, cancelReason.value.trim()));

const recordTrial = () =>
  act("trial", async () => {
    await tests.recordTrial(testId.value, {
      repetition: Number(newTrial.value.repetition),
      startedAt: instant(newTrial.value.startedAt) || new Date().toISOString(),
      // An open trial is one that has started and not yet ended.
      endedAt: instant(newTrial.value.endedAt),
      notes: newTrial.value.notes.trim(),
    });
    newTrial.value = { repetition: 1, startedAt: "", endedAt: "", notes: "" };
    touchedStart.value = false;
  });
const succeeded = computed(() => detail.value?.status === "SUCCEEDED");
const metrics = computed(() => detail.value?.result?.metrics || []);
const events = computed(() => detail.value?.result?.events || []);

async function load() {
  error.value = "";
  try {
    // The test carries its trials, so recording one needs no second call.
    test.value = await tests.get(testId.value);
    trials.value = test.value.trials || [];
    // The default start is the moment the screen last read the test, so it is
    // never earlier than the start the API just recorded. A value the
    // technician typed is left alone.
    if (!touchedStart.value) newTrial.value.startedAt = defaultTrialStart();
    recordingList.value = (await recordings.list(testId.value)).data || [];
    for (const recording of recordingList.value) {
      calibrationStatus.value[recording.id] = await calibration.status(testId.value, recording.id);
    }
    runs.value = (await analysis.runsOfTest(testId.value)).data || [];
    // Runs come newest first. Open the newest one that published a result, so a
    // later failed attempt does not hide the result the laboratory can read.
    if (!selectedRunId.value && runs.value.length) {
      const published = runs.value.find((run) => run.status === "SUCCEEDED");
      selectedRunId.value = (published || runs.value[0]).id;
    }
    await loadRun();
    crumb.set([{ label: t("tests.title") }, { label: test.value.paradigmKey }]);
  } catch (e) {
    error.value = e.message;
  }
}

async function loadRun() {
  selectedEvent.value = null;
  detail.value = selectedRunId.value ? await analysis.run(selectedRunId.value) : null;
}

async function selectRun(id) {
  selectedRunId.value = id;
  await loadRun();
}

// Uploads the file straight to storage, then asks the API to verify it.
async function uploadRecording(event) {
  const file = event.target.files?.[0];
  if (!file) return;
  busy.value = "upload";
  error.value = "";
  progress.value = 0;
  try {
    const body = {
      fileName: file.name,
      contentType: file.type || "video/mp4",
      sizeBytes: file.size,
      crc32c: await crc32c(file),
      clipStartUs: Math.round(Number(clip.value.startS || 0) * 1_000_000),
      clipEndUs: clip.value.endS === "" ? null : Math.round(Number(clip.value.endS) * 1_000_000),
    };
    const started = await recordings.start(testId.value, body);
    await upload(started.upload, file, { onProgress: (value) => (progress.value = value) });
    busy.value = "finalize";
    await recordings.finalize(testId.value, started.recording.id);
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = "";
    event.target.value = "";
  }
}

async function reanalyze(recordingId) {
  busy.value = recordingId;
  error.value = "";
  try {
    await analysis.reanalyze(testId.value, recordingId);
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = "";
  }
}

const metricValue = (metric) => (metric.value === null ? t("analysis.missing") : `${metric.value} ${metric.unit}`);

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <p class="err" v-if="error" data-test="error">{{ error }}</p>

  <div class="detail" v-if="test">
    <div class="card">
      <h2 style="margin-top:0">
        {{ test.paradigmKey }} <span class="muted">v{{ test.paradigmVersion }}</span>
      </h2>
      <p class="muted" data-test="test-status" :data-status="test.status">
        {{ $t("tests.status") }}: {{ $t(`statuses.${test.status}`) }}
      </p>
      <p class="muted" v-if="test.cancelReason" data-test="cancel-reason">
        {{ $t("tests.cancelled", { reason: test.cancelReason }) }}
      </p>

      <!-- A planned test starts; one in progress completes. Either can be
           retracted, and a finished one offers nothing. -->
      <div class="actions" v-if="planned || inProgress">
        <button v-if="planned && auth.canRun" :disabled="!!busy" data-test="start-test" @click="startTest">
          {{ $t("tests.start") }}
        </button>
        <button
          v-if="inProgress && auth.canRun"
          :disabled="!!busy || !trials.length"
          :title="trials.length ? '' : $t('tests.needsTrial')"
          data-test="complete-test"
          @click="completeTest"
        >
          {{ $t("tests.complete") }}
        </button>
        <template v-if="canCancel">
          <input
            v-model="cancelReason"
            class="reason"
            :placeholder="$t('tests.cancelReason')"
            data-test="cancel-reason-input"
          />
          <button class="small" :disabled="!!busy || !cancelReason.trim()" data-test="cancel-test" @click="cancelTest">
            {{ $t("tests.cancel") }}
          </button>
        </template>
      </div>
      <p class="muted" v-if="inProgress && !trials.length" data-test="needs-trial">{{ $t("tests.needsTrial") }}</p>
    </div>

    <div class="card">
      <h4>{{ $t("tests.trials") }}</h4>
      <p class="muted" data-test="trial-count">
        {{ $t("tests.plannedTrials", { done: trials.length, planned: test.plannedTrials }) }}
      </p>
      <ul class="trials" v-if="trials.length">
        <li v-for="trial in trials" :key="trial.id" data-test="trial" :data-repetition="trial.repetition">
          {{ trialLabel(trial) }}
          <span class="muted">{{ new Date(trial.startedAt).toLocaleString() }}</span>
        </li>
      </ul>
      <p v-else class="muted">{{ $t("tests.noTrials") }}</p>

      <form v-if="inProgress && auth.canRun" class="trial-form" data-test="trial-form" @submit.prevent="recordTrial">
        <label class="fld narrow">
          <span>{{ $t("tests.repetition") }}</span>
          <input
            type="number"
            min="1"
            :max="test.plannedTrials"
            v-model="newTrial.repetition"
            data-test="trial-repetition"
            required
          />
        </label>
        <label class="fld">
          <span>{{ $t("tests.trialStart") }}</span>
          <input
            type="datetime-local"
            step="1"
            v-model="newTrial.startedAt"
            data-test="trial-start"
            required
            @input="touchedStart = true"
          />
        </label>
        <label class="fld">
          <span>{{ $t("tests.trialEnd") }}</span>
          <input type="datetime-local" step="1" v-model="newTrial.endedAt" data-test="trial-end" />
        </label>
        <button type="submit" :disabled="!!busy" data-test="trial-save">{{ $t("tests.recordTrial") }}</button>
      </form>
    </div>

    <div class="card">
      <h4>{{ $t("tests.recordings") }}</h4>
      <ul class="recordings" v-if="recordingList.length" data-test="recordings">
        <li
          v-for="recording in recordingList"
          :key="recording.id"
          data-test="recording"
          :data-video-status="recording.video.status"
        >
          <span>{{ recording.video.fileName || recording.video.id.slice(0, 8) }}</span>
          <span class="muted">{{ $t(`statuses.${recording.video.status}`) }}</span>
          <span class="muted">{{ $t(`statuses.${calibrationStatus[recording.id]?.status || "PENDING"}`) }}</span>
          <button
            v-if="auth.canRun"
            class="small"
            :disabled="busy === recording.id"
            data-test="reanalyze"
            @click="reanalyze(recording.id)"
          >
            {{ $t("analysis.reanalyze") }}
          </button>
        </li>
      </ul>
      <p v-else class="muted">{{ $t("tests.noRecordings") }}</p>

      <div class="upload" v-if="auth.canRun">
        <label class="fld">
          <span>{{ $t("tests.clipStart") }}</span>
          <input type="number" min="0" step="0.1" v-model="clip.startS" />
        </label>
        <label class="fld">
          <span>{{ $t("tests.clipEnd") }}</span>
          <input type="number" min="0" step="0.1" v-model="clip.endS" />
        </label>
        <label class="fld">
          <span>{{ $t("tests.upload") }}</span>
          <input type="file" accept="video/*" data-test="upload-input" :disabled="!!busy" @change="uploadRecording" />
        </label>
        <p class="muted" v-if="busy === 'upload'" data-test="upload-progress">
          {{ $t("tests.uploading") }} {{ Math.round(progress * 100) }}%
        </p>
        <p class="muted" v-else-if="busy === 'finalize'">{{ $t("tests.finalizing") }}</p>
      </div>
    </div>

    <div class="card">
      <h4>{{ $t("analysis.runs") }}</h4>
      <p v-if="!runs.length" class="muted" data-test="no-runs">{{ $t("analysis.noRuns") }}</p>
      <div v-else class="runs" data-test="runs">
        <button
          v-for="run in runs"
          :key="run.id"
          class="run"
          :class="{ active: run.id === selectedRunId }"
          data-test="run"
          :data-run-id="run.id"
          :data-selected="run.id === selectedRunId"
          @click="selectRun(run.id)"
        >
          <span>{{ $t(`analysis.${run.trigger}`) }}</span>
          <span class="muted">{{ $t(`statuses.${run.status}`) }}</span>
          <span class="muted" v-if="run.finishedAt">{{ new Date(run.finishedAt).toLocaleString() }}</span>
        </button>
      </div>

      <template v-if="detail">
        <p v-if="detail.status === 'QUEUED'" class="muted" data-test="run-hint">{{ $t("analysis.queuedHint") }}</p>
        <p v-else-if="detail.status === 'RUNNING'" class="muted" data-test="run-hint">{{ $t("analysis.runningHint") }}</p>
        <p v-else-if="detail.status === 'FAILED'" class="err" data-test="run-failure">
          {{ $t("analysis.failure") }}: {{ detail.failureReason }}
        </p>

        <div v-if="succeeded" class="result">
          <div class="col">
            <h5>{{ $t("analysis.metrics") }}</h5>
            <table class="rows" data-test="metrics">
              <tbody>
                <tr v-for="metric in metrics" :key="metric.key">
                  <td>{{ metric.key }}</td>
                  <td class="muted" :data-test="`metric-${metric.key}`">{{ metricValue(metric) }}</td>
                </tr>
              </tbody>
            </table>
            <h5>{{ $t("analysis.events") }}</h5>
            <EventTimeline :events="events" :selected="selectedEvent" @play="selectedEvent = $event" />
          </div>
          <div class="col">
            <VideoPairPanel :run-id="selectedRunId" :play="selectedEvent" @error="error = $event" />
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.recordings { list-style: none; margin: 0 0 12px; padding: 0; display: flex; flex-direction: column; gap: 6px; }
.recordings li { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.upload { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
.actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-top: 10px; }
.reason { min-width: 220px; }
.trials { list-style: none; margin: 0 0 12px; padding: 0; display: flex; flex-direction: column; gap: 6px; }
.trial-form { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 14px; }
.narrow { max-width: 110px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.runs { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
.run {
  display: flex; gap: 8px; align-items: center;
  background: var(--panel2); border: 1px solid var(--line); border-radius: 9px; padding: 6px 10px; cursor: pointer;
}
.run.active { border-color: var(--accent); }
.result { display: flex; gap: 20px; flex-wrap: wrap; }
.col { flex: 1; min-width: 280px; }
.col h5 { margin: 0 0 8px; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.rows { width: 100%; border-collapse: collapse; margin-bottom: 16px; }
.rows td { padding: 4px 8px; border-bottom: 1px solid var(--line); }
.small { padding: 4px 10px; font-size: .85rem; }
</style>
