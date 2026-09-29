<script setup>
// The workflow of one test: upload a recording, watch its calibration and
// analysis status, then read the published result. Picking an event plays that
// interval in the paired video panel beside the timeline.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDescriptions, NDescriptionsItem, NProgress, NTag } from "naive-ui";
import { analysis, calibration, recordings, tests } from "../api/endpoints.js";
import { crc32c, upload } from "../api/upload.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";
import EventTimeline from "../components/EventTimeline.vue";
import TestComments from "../components/TestComments.vue";
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

// A status carries meaning through its color everywhere on this page: the
// test's own lifecycle, a video's verification, a calibration's validity, an
// analysis run's outcome. One map covers all of them.
const STATUS_TAG = {
  PLANNED: "info",
  IN_PROGRESS: "warning",
  COMPLETED: "success",
  CANCELLED: "error",
  PENDING: "default",
  VERIFIED: "success",
  REJECTED: "error",
  QUEUED: "info",
  RUNNING: "warning",
  SUCCEEDED: "success",
  FAILED: "error",
  NOT_REQUIRED: "default",
  WAITING_FOR_CALIBRATION: "warning",
  CALIBRATED: "success",
  VALID: "success",
};
const tagType = (status) => STATUS_TAG[status] || "default";

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

// Calibration turns pixels into centimetres, so it is what makes a recording
// measurable. Entering one runs the test, which is test:run.
const PLANES = ["ARENA_FLOOR", "WATER_SURFACE", "APPARATUS_TOP"];
const emptyPoint = () => ({ pixelX: "", pixelY: "", worldX: "", worldY: "" });
// The API asks for at least four fit points and three check points.
const blankCalibration = () => ({
  cameraId: "",
  frameWidth: "",
  frameHeight: "",
  crop: { x: 0, y: 0, width: "", height: "" },
  referenceFrameS: 0,
  measurementPlane: "ARENA_FLOOR",
  fitPoints: [emptyPoint(), emptyPoint(), emptyPoint(), emptyPoint()],
  checkPoints: [emptyPoint(), emptyPoint(), emptyPoint()],
});
const calibrating = ref("");
const newCalibration = ref(blankCalibration());

const calibrationOf = (recordingId) => calibrationStatus.value[recordingId]?.calibration || null;
const calibratable = (recording) =>
  recording.video.status === "VERIFIED" && calibrationStatus.value[recording.id]?.status !== "NOT_REQUIRED";

const centimetres = (value) => `${Number(value).toFixed(2)} cm`;

function openCalibration(recording) {
  const current = calibrationOf(recording.id);
  newCalibration.value = blankCalibration();
  // A correction has to keep the camera and the frame of the calibration it
  // replaces, so those are carried over rather than asked for again.
  if (current) {
    newCalibration.value.cameraId = current.cameraId;
    newCalibration.value.frameWidth = current.frameWidth;
    newCalibration.value.frameHeight = current.frameHeight;
    newCalibration.value.crop = { ...current.crop };
    newCalibration.value.measurementPlane = current.measurementPlane;
  }
  calibrating.value = recording.id;
}

const point = (p) => ({
  pixelX: Number(p.pixelX),
  pixelY: Number(p.pixelY),
  worldX: Number(p.worldX),
  worldY: Number(p.worldY),
});

async function saveCalibration(recordingId) {
  const current = calibrationOf(recordingId);
  await act("calibration", async () => {
    await calibration.create(testId.value, recordingId, {
      cameraId: newCalibration.value.cameraId.trim(),
      frameWidth: Number(newCalibration.value.frameWidth),
      frameHeight: Number(newCalibration.value.frameHeight),
      crop: {
        x: Number(newCalibration.value.crop.x),
        y: Number(newCalibration.value.crop.y),
        width: Number(newCalibration.value.crop.width),
        height: Number(newCalibration.value.crop.height),
      },
      // The field speaks seconds; the API counts microseconds from the start.
      referenceFrameUs: Math.round(Number(newCalibration.value.referenceFrameS || 0) * 1_000_000),
      measurementPlane: newCalibration.value.measurementPlane,
      fitPoints: newCalibration.value.fitPoints.map(point),
      checkPoints: newCalibration.value.checkPoints.map(point),
      // Every calibration after the first has to name the one it replaces.
      supersedesId: current?.id || "",
    });
    calibrating.value = "";
  });
}

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
    crumb.set([{ label: t("tests.title"), to: "/tests" }, { label: test.value.paradigmKey }]);
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

// Published metrics carry full floating-point precision, which reads as noise
// next to a unit; three decimal places is more than the underlying tracking
// accuracy justifies. Trailing zeros are dropped so a whole number of seconds
// or centimetres still reads as one.
function formatNumber(value) {
  if (typeof value !== "number" || !Number.isFinite(value)) return value;
  if (Number.isInteger(value)) return String(value);
  return value.toFixed(3).replace(/0+$/, "").replace(/\.$/, "");
}
const metricValue = (metric) => (metric.value === null ? t("analysis.missing") : `${formatNumber(metric.value)} ${metric.unit}`);

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />

  <div class="detail" v-if="test">
    <PageHead :title="`${test.paradigmKey} · v${test.paradigmVersion}`">
      <template #actions>
        <NTag :type="tagType(test.status)" round data-test="test-status" :data-status="test.status">
          {{ $t(`statuses.${test.status}`) }}
        </NTag>
        <template v-if="planned || inProgress">
          <NButton v-if="planned && auth.canRun" type="primary" :loading="busy === 'start'" :disabled="!!busy" data-test="start-test" @click="startTest">
            <template #icon>
              <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" stroke="none"><polygon points="6 4 20 12 6 20 6 4" /></svg>
            </template>
            {{ $t("tests.start") }}
          </NButton>
          <NButton
            v-if="inProgress && auth.canRun"
            type="primary"
            :loading="busy === 'complete'"
            :disabled="!!busy || !trials.length"
            :title="trials.length ? '' : $t('tests.needsTrial')"
            data-test="complete-test"
            @click="completeTest"
          >
            <template #icon>
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg>
            </template>
            {{ $t("tests.complete") }}
          </NButton>
        </template>
      </template>
    </PageHead>
    <p class="muted" v-if="test.cancelReason" data-test="cancel-reason">
      {{ $t("tests.cancelled", { reason: test.cancelReason }) }}
    </p>
    <p class="muted hint" v-if="inProgress && !trials.length" data-test="needs-trial">{{ $t("tests.needsTrial") }}</p>

    <NCard v-if="canCancel && (planned || inProgress)" :bordered="true" size="small" class="cancel-card">
      <div class="cancel-row">
        <input
          v-model="cancelReason"
          class="reason"
          :placeholder="$t('tests.cancelReason')"
          data-test="cancel-reason-input"
        />
        <NButton :disabled="!!busy || !cancelReason.trim()" :loading="busy === 'cancel'" data-test="cancel-test" @click="cancelTest">
          <template #icon>
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><line x1="6" y1="6" x2="18" y2="18" /><line x1="6" y1="18" x2="18" y2="6" /></svg>
          </template>
          {{ $t("tests.cancel") }}
        </NButton>
      </div>
    </NCard>

    <div class="grid-2">
      <NCard :bordered="true" size="small">
        <template #header>{{ $t("tests.trials") }}</template>
        <p class="muted" data-test="trial-count">
          {{ $t("tests.plannedTrials", { done: trials.length, planned: test.plannedTrials }) }}
        </p>
        <ul class="rows-list" v-if="trials.length">
          <li v-for="trial in trials" :key="trial.id" data-test="trial" :data-repetition="trial.repetition">
            <span>{{ trialLabel(trial) }}</span>
            <span class="muted">{{ new Date(trial.startedAt).toLocaleString() }}</span>
          </li>
        </ul>
        <p v-else class="muted empty">{{ $t("tests.noTrials") }}</p>

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
          <NButton type="primary" attr-type="submit" :loading="busy === 'trial'" :disabled="!!busy" data-test="trial-save">
            {{ $t("tests.recordTrial") }}
          </NButton>
        </form>
      </NCard>

      <NCard :bordered="true" size="small">
        <template #header>{{ $t("tests.recordings") }}</template>
        <ul class="rows-list recordings-list" v-if="recordingList.length" data-test="recordings">
          <li
            v-for="recording in recordingList"
            :key="recording.id"
            data-test="recording"
            :data-video-status="recording.video.status"
          >
            <span class="filename" :title="recording.video.fileName || recording.video.id">
              {{ recording.video.fileName || recording.video.id.slice(0, 8) }}
            </span>
            <span class="rec-meta">
              <span class="rec-tags">
                <NTag size="small" round :bordered="false" :type="tagType(recording.video.status)">
                  {{ $t(`statuses.${recording.video.status}`) }}
                </NTag>
                <NTag size="small" round :bordered="false" :type="tagType(calibrationStatus[recording.id]?.status || 'PENDING')">
                  {{ $t(`statuses.${calibrationStatus[recording.id]?.status || "PENDING"}`) }}
                </NTag>
              </span>
              <NButton
                v-if="auth.canRun"
                size="small"
                secondary
                class="reanalyze-btn"
                :loading="busy === recording.id"
                :disabled="!!busy"
                data-test="reanalyze"
                @click="reanalyze(recording.id)"
              >
                <template #icon>
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-3-6.7" /><polyline points="21 3 21 9 15 9" /></svg>
                </template>
                {{ $t("analysis.reanalyze") }}
              </NButton>
            </span>
          </li>
        </ul>
        <p v-else class="muted empty">{{ $t("tests.noRecordings") }}</p>

        <div class="upload" v-if="auth.canRun">
          <label class="fld narrow">
            <span>{{ $t("tests.clipStart") }}</span>
            <input type="number" min="0" step="0.1" v-model="clip.startS" />
          </label>
          <label class="fld narrow">
            <span>{{ $t("tests.clipEnd") }}</span>
            <input type="number" min="0" step="0.1" v-model="clip.endS" />
          </label>
          <label class="fld grow">
            <span>{{ $t("tests.upload") }}</span>
            <input type="file" accept="video/*" data-test="upload-input" :disabled="!!busy" @change="uploadRecording" />
          </label>
        </div>
        <div class="progress" v-if="busy === 'upload'" data-test="upload-progress">
          <NProgress type="line" :percentage="Math.round(progress * 100)" :height="6" />
          <span class="muted">{{ $t("tests.uploading") }} {{ Math.round(progress * 100) }}%</span>
        </div>
        <p class="muted hint" v-else-if="busy === 'finalize'">{{ $t("tests.finalizing") }}</p>
      </NCard>
    </div>

    <NCard v-if="recordingList.length" :bordered="true" size="small">
      <template #header>{{ $t("calibration.title") }}</template>

      <!-- A test can hold several recordings, so each one carries its own id:
           a calibration belongs to one recording, not to the test. -->
      <div
        v-for="recording in recordingList"
        :key="recording.id"
        class="calibration"
        data-test="calibration"
        :data-recording="recording.id"
      >
        <p class="muted filename">{{ recording.video.fileName || recording.video.id.slice(0, 8) }}</p>
        <p class="muted" v-if="calibrationStatus[recording.id]?.status === 'NOT_REQUIRED'" data-test="calibration-not-required">
          {{ $t("calibration.notRequired") }}
        </p>
        <template v-else>
          <!-- The measured errors are what say whether the recording can be
               turned into centimetres, so they are shown rather than a badge. -->
          <NDescriptions
            v-if="calibrationOf(recording.id)"
            :column="1"
            label-placement="left"
            size="small"
            bordered
            data-test="calibration-summary"
            :data-calibration-status="calibrationOf(recording.id).status"
          >
            <NDescriptionsItem :label="$t('calibration.fitError')">
              <span data-test="fit-error">{{ centimetres(calibrationOf(recording.id).fitRmsErrorCm) }}</span>
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('calibration.checkError')">
              <span data-test="check-error">{{ centimetres(calibrationOf(recording.id).checkRmsErrorCm) }}</span>
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('calibration.maxError')">
              {{ centimetres(calibrationOf(recording.id).checkMaxErrorCm) }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('calibration.tolerance')">
              <span data-test="tolerance">{{ centimetres(calibrationOf(recording.id).toleranceCm) }}</span>
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('tests.status')">
              <NTag size="small" round :bordered="false" :type="tagType(calibrationOf(recording.id).status)">
                {{ $t(`statuses.${calibrationOf(recording.id).status}`) }}
              </NTag>
            </NDescriptionsItem>
          </NDescriptions>
          <p v-else class="muted empty" data-test="calibration-none">{{ $t("calibration.none") }}</p>
          <p class="err" v-if="calibrationOf(recording.id)?.status === 'REJECTED'" data-test="calibration-rejected">
            {{ $t("calibration.rejected") }}
          </p>

          <NButton
            v-if="auth.canRun && calibratable(recording) && calibrating !== recording.id"
            size="small"
            secondary
            class="calibrate-btn"
            data-test="calibrate"
            @click="openCalibration(recording)"
          >
            {{ $t("calibration.enter") }}
          </NButton>

          <form
            v-if="calibrating === recording.id"
            class="calibration-form"
            data-test="calibration-form"
            @submit.prevent="saveCalibration(recording.id)"
          >
            <p class="muted" v-if="calibrationOf(recording.id)" data-test="calibration-correcting">
              {{ $t("calibration.correcting") }}
            </p>
            <div class="setup">
              <label class="fld">
                <span>{{ $t("calibration.camera") }}</span>
                <input v-model="newCalibration.cameraId" data-test="camera-id" required />
              </label>
              <label class="fld narrow">
                <span>{{ $t("calibration.frame") }}</span>
                <input type="number" min="1" v-model="newCalibration.frameWidth" data-test="frame-width" required />
              </label>
              <label class="fld narrow">
                <span>&nbsp;</span>
                <input type="number" min="1" v-model="newCalibration.frameHeight" data-test="frame-height" required />
              </label>
              <label class="fld narrow">
                <span>{{ $t("calibration.crop") }}</span>
                <input type="number" min="0" v-model="newCalibration.crop.width" data-test="crop-width" required />
              </label>
              <label class="fld narrow">
                <span>&nbsp;</span>
                <input type="number" min="0" v-model="newCalibration.crop.height" data-test="crop-height" required />
              </label>
              <label class="fld">
                <span>{{ $t("calibration.plane") }}</span>
                <select v-model="newCalibration.measurementPlane" data-test="plane">
                  <option v-for="plane in PLANES" :key="plane" :value="plane">{{ $t(`calibration.planes.${plane}`) }}</option>
                </select>
              </label>
              <label class="fld narrow">
                <span>{{ $t("calibration.referenceFrame") }}</span>
                <input type="number" min="0" step="0.001" v-model="newCalibration.referenceFrameS" data-test="reference-frame" />
              </label>
            </div>

            <p class="muted hint">{{ $t("calibration.pointsHint") }}</p>

            <h5>{{ $t("calibration.fitPoints") }}</h5>
            <div class="points">
              <div class="point" v-for="(p, index) in newCalibration.fitPoints" :key="`fit-${index}`" data-test="fit-point">
                <label class="fld narrow">
                  <span>{{ $t("calibration.pixel") }} X</span>
                  <input type="number" step="any" v-model="p.pixelX" :data-test="`fit-${index}-pixel-x`" required />
                </label>
                <label class="fld narrow">
                  <span>{{ $t("calibration.pixel") }} Y</span>
                  <input type="number" step="any" v-model="p.pixelY" :data-test="`fit-${index}-pixel-y`" required />
                </label>
                <label class="fld narrow">
                  <span>{{ $t("calibration.plane_") }} X</span>
                  <input type="number" step="any" v-model="p.worldX" :data-test="`fit-${index}-world-x`" required />
                </label>
                <label class="fld narrow">
                  <span>{{ $t("calibration.plane_") }} Y</span>
                  <input type="number" step="any" v-model="p.worldY" :data-test="`fit-${index}-world-y`" required />
                </label>
                <NButton
                  v-if="newCalibration.fitPoints.length > 4"
                  size="tiny"
                  quaternary
                  :data-test="`fit-${index}-remove`"
                  @click="newCalibration.fitPoints.splice(index, 1)"
                >
                  {{ $t("calibration.removePoint") }}
                </NButton>
              </div>
            </div>
            <NButton size="small" dashed data-test="add-fit-point" @click="newCalibration.fitPoints.push(emptyPoint())">
              + {{ $t("calibration.addPoint") }}
            </NButton>

            <h5>{{ $t("calibration.checkPoints") }}</h5>
            <div class="points">
              <div class="point" v-for="(p, index) in newCalibration.checkPoints" :key="`check-${index}`" data-test="check-point">
                <label class="fld narrow">
                  <span>{{ $t("calibration.pixel") }} X</span>
                  <input type="number" step="any" v-model="p.pixelX" :data-test="`check-${index}-pixel-x`" required />
                </label>
                <label class="fld narrow">
                  <span>{{ $t("calibration.pixel") }} Y</span>
                  <input type="number" step="any" v-model="p.pixelY" :data-test="`check-${index}-pixel-y`" required />
                </label>
                <label class="fld narrow">
                  <span>{{ $t("calibration.plane_") }} X</span>
                  <input type="number" step="any" v-model="p.worldX" :data-test="`check-${index}-world-x`" required />
                </label>
                <label class="fld narrow">
                  <span>{{ $t("calibration.plane_") }} Y</span>
                  <input type="number" step="any" v-model="p.worldY" :data-test="`check-${index}-world-y`" required />
                </label>
                <NButton
                  v-if="newCalibration.checkPoints.length > 3"
                  size="tiny"
                  quaternary
                  :data-test="`check-${index}-remove`"
                  @click="newCalibration.checkPoints.splice(index, 1)"
                >
                  {{ $t("calibration.removePoint") }}
                </NButton>
              </div>
            </div>
            <NButton size="small" dashed data-test="add-check-point" @click="newCalibration.checkPoints.push(emptyPoint())">
              + {{ $t("calibration.addPoint") }}
            </NButton>

            <div class="actions">
              <NButton type="primary" attr-type="submit" :loading="busy === 'calibration'" :disabled="!!busy" data-test="calibration-save">
                {{ $t("calibration.save") }}
              </NButton>
              <NButton quaternary data-test="calibration-cancel" @click="calibrating = ''">
                {{ $t("common.cancel") }}
              </NButton>
            </div>
          </form>
        </template>
      </div>
    </NCard>

    <NCard :bordered="true" size="small">
      <template #header>{{ $t("analysis.runs") }}</template>
      <p v-if="!runs.length" class="muted empty" data-test="no-runs">{{ $t("analysis.noRuns") }}</p>
      <div v-else class="runs" data-test="runs">
        <button
          v-for="run in runs"
          :key="run.id"
          type="button"
          class="run"
          :class="{ active: run.id === selectedRunId }"
          data-test="run"
          :data-run-id="run.id"
          :data-selected="run.id === selectedRunId"
          @click="selectRun(run.id)"
        >
          <span>{{ $t(`analysis.${run.trigger}`) }}</span>
          <NTag size="tiny" round :bordered="false" :type="tagType(run.status)">{{ $t(`statuses.${run.status}`) }}</NTag>
          <span class="muted" v-if="run.finishedAt">{{ new Date(run.finishedAt).toLocaleString() }}</span>
        </button>
      </div>

      <template v-if="detail">
        <p v-if="detail.status === 'QUEUED'" class="muted hint" data-test="run-hint">{{ $t("analysis.queuedHint") }}</p>
        <p v-else-if="detail.status === 'RUNNING'" class="muted hint" data-test="run-hint">{{ $t("analysis.runningHint") }}</p>
        <NAlert v-else-if="detail.status === 'FAILED'" type="error" data-test="run-failure" style="margin-top: 10px">
          {{ $t("analysis.failure") }}: {{ detail.failureReason }}
        </NAlert>

        <div v-if="succeeded" class="result">
          <div class="col">
            <h5>{{ $t("analysis.metrics") }}</h5>
            <NDescriptions :column="1" label-placement="left" size="small" bordered data-test="metrics">
              <NDescriptionsItem v-for="metric in metrics" :key="metric.key" :label="metric.key">
                <span :data-test="`metric-${metric.key}`">{{ metricValue(metric) }}</span>
              </NDescriptionsItem>
            </NDescriptions>
            <h5>{{ $t("analysis.events") }}</h5>
            <EventTimeline :events="events" :selected="selectedEvent" @play="selectedEvent = $event" />
          </div>
          <div class="col">
            <VideoPairPanel :run-id="selectedRunId" :play="selectedEvent" @error="error = $event" />
          </div>
        </div>
      </template>
    </NCard>

    <!-- The thread belongs to the test, so it sits at the end of its page. -->
    <TestComments :test-id="testId" />
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; align-items: start; }
@media (max-width: 860px) { .grid-2 { grid-template-columns: 1fr; } }

.cancel-card { padding-top: 2px; }
.cancel-row { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.reason { min-width: 220px; flex: 1; }
.hint { margin: 8px 0 0; }
.empty { margin: 0; }

.rows-list { list-style: none; margin: 0 0 12px; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.rows-list li {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 8px 10px; border-radius: 8px;
}
.rows-list li:hover { background: var(--panel2); }
.filename { font-weight: 600; }

.recordings-list li { flex-direction: column; align-items: stretch; gap: 6px; }
.recordings-list .filename { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rec-meta { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.rec-tags { display: flex; gap: 6px; flex-wrap: wrap; }
.reanalyze-btn { flex-shrink: 0; }

.upload { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 10px; }
.progress { display: flex; align-items: center; gap: 10px; margin-top: 10px; }
.progress :deep(.n-progress) { flex: 1; }

.trial-form { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 14px; }
.calibration + .calibration { margin-top: 18px; border-top: 1px solid var(--line); padding-top: 18px; }
.calibrate-btn { margin-top: 4px; }
.calibration-form { display: flex; flex-direction: column; gap: 10px; margin-top: 12px; }
.calibration-form h5 { margin: 6px 0 0; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.setup { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
.points { display: flex; flex-direction: column; gap: 8px; }
.point {
  display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap;
  padding: 10px; border-radius: 9px; background: var(--panel2);
}
.narrow { max-width: 110px; }
.grow { flex: 1; min-width: 200px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-top: 4px; }

.runs { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
.run {
  display: flex; gap: 8px; align-items: center; font: inherit; color: var(--txt);
  background: var(--panel2); border: 1px solid var(--line); border-radius: 9px; padding: 6px 10px; cursor: pointer;
}
.run:hover { border-color: var(--accent); }
.run.active { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, var(--panel2)); }
.result { display: grid; grid-template-columns: minmax(280px, 420px) 1fr; gap: 24px; margin-top: 14px; align-items: start; }
@media (max-width: 900px) { .result { grid-template-columns: 1fr; } }
.col { min-width: 0; }
.col h5 { margin: 0 0 8px; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.col h5 + h5 { margin-top: 18px; }
.col :deep(.n-descriptions-table-content) { word-break: break-word; }
</style>
