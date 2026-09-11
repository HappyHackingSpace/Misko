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
const succeeded = computed(() => detail.value?.status === "SUCCEEDED");
const metrics = computed(() => detail.value?.result?.metrics || []);
const events = computed(() => detail.value?.result?.events || []);

async function load() {
  error.value = "";
  try {
    test.value = await tests.get(testId.value);
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
      <p class="muted">{{ $t("tests.status") }}: {{ $t(`statuses.${test.status}`) }}</p>
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
