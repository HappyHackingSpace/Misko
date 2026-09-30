<script setup>
// One test. The top says what it is and where it stands; below, three tabs hold
// the three kinds of work: the videos with their results, the trials, and the
// comments. A test can hold several videos, so they are a short list beside the
// one being read, and adding one is its own dialog.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDropdown, NInput, NModal, NTabPane, NTabs, NTag, NTooltip } from "naive-ui";
import { analysis, calibration, experiments, recordings, subjects, tests } from "../api/endpoints.js";
import { crc32c, upload } from "../api/upload.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import { useEnvironmentRevisions } from "../composables/useEnvironmentRevisions.js";
import PageHead from "../components/PageHead.vue";
import TestComments from "../components/TestComments.vue";
import FlowSteps from "../components/test/FlowSteps.vue";
import RecordingRail from "../components/test/RecordingRail.vue";
import UploadDialog from "../components/test/UploadDialog.vue";
import TrialsPanel from "../components/test/TrialsPanel.vue";
import CalibrationPanel from "../components/test/CalibrationPanel.vue";
import ResultPanel from "../components/test/ResultPanel.vue";
import RunsPanel from "../components/test/RunsPanel.vue";

const { t } = useI18n();
const route = useRoute();
const auth = useAuth();
const crumb = useBreadcrumb();

const test = ref(null);
// What the test belongs to: its experiment, its subject, and the apparatus and
// revision it was run with.
const experiment = ref(null);
const subject = ref(null);
const revisions = useEnvironmentRevisions();
const apparatus = computed(() => revisions.byId.value[test.value?.environmentRevisionId] || null);
const trials = ref([]);
const recordingList = ref([]);
const calibrationStatus = ref({});
const ownCalibrations = ref({});
const runs = ref([]);
const detail = ref(null);
const selectedRecordingId = ref("");
const selectedRunId = ref("");
const pageTab = ref("");
const paneTab = ref("result");
const error = ref("");
const busy = ref("");
const progress = ref(0);
const clip = ref({ startS: 0, endS: "" });
const uploading = ref(false);
const cancelling = ref(false);
const cancelReason = ref("");

const testId = computed(() => route.params.id);

// A status carries meaning through its color everywhere on this page: the
// test's own lifecycle, a calibration's validity, an analysis run's outcome.
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
const cancellable = computed(() => canCancel.value && (planned.value || inProgress.value));

// ---- videos and what hangs off them ----------------------------------------

const selectedRecording = computed(() => recordingList.value.find((r) => r.id === selectedRecordingId.value) || null);
const selectedNumber = computed(() => recordingList.value.findIndex((r) => r.id === selectedRecordingId.value) + 1);
// Runs come newest first; these are the ones of the video on screen.
const recordingRuns = computed(() => runs.value.filter((run) => run.recordingId === selectedRecordingId.value));
// One analysis per video at a time; the API refuses another while one is queued or running.
const analysisPending = computed(() => recordingRuns.value.some((run) => run.status === "QUEUED" || run.status === "RUNNING"));

// One line per video: the next thing standing between it and its result.
function stageOf(recording) {
  const video = recording.video.status;
  if (video === "REJECTED") return { stage: "rejected", tone: "bad" };
  if (video !== "VERIFIED") return { stage: "verifying", tone: "warn" };
  const own = runs.value.filter((run) => run.recordingId === recording.id);
  if (own.some((run) => run.status === "SUCCEEDED")) return { stage: "ready", tone: "good" };
  if (own[0]?.status === "RUNNING") return { stage: "running", tone: "warn" };
  if (own[0]?.status === "QUEUED") return { stage: "queued", tone: "info" };
  if (own[0]?.status === "FAILED") return { stage: "failed", tone: "bad" };
  if (calibrationStatus.value[recording.id]?.status === "WAITING_FOR_CALIBRATION") return { stage: "waitingCalibration", tone: "warn" };
  return { stage: "analysisPending", tone: "info" };
}
const stages = computed(() => Object.fromEntries(recordingList.value.map((recording) => [recording.id, stageOf(recording)])));

// What the top of the page needs to know about the test's progress.
const progressCounts = computed(() => ({
  verifiedVideos: recordingList.value.filter((r) => r.video.status === "VERIFIED").length,
  activeRuns: runs.value.filter((run) => run.status === "QUEUED" || run.status === "RUNNING").length,
  publishedRuns: runs.value.filter((run) => run.status === "SUCCEEDED").length,
}));

// The newest published run, or else the newest one: a later failed attempt must
// not hide the result the laboratory can read.
function defaultRun(list) {
  return (list.find((run) => run.status === "SUCCEEDED") || list[0])?.id || "";
}

// Opens the video whose result is newest, so the page lands on something to read.
function defaultRecording() {
  const newest = runs.value.find((run) => run.status === "SUCCEEDED") || runs.value[0];
  return newest?.recordingId || recordingList.value[recordingList.value.length - 1]?.id || "";
}

function ensureSelection() {
  if (!recordingList.value.some((r) => r.id === selectedRecordingId.value)) selectedRecordingId.value = defaultRecording();
  if (!recordingRuns.value.some((run) => run.id === selectedRunId.value)) selectedRunId.value = defaultRun(recordingRuns.value);
}

async function loadRun() {
  detail.value = selectedRunId.value ? await analysis.run(selectedRunId.value) : null;
}

async function selectRecording(id) {
  selectedRecordingId.value = id;
  selectedRunId.value = defaultRun(recordingRuns.value);
  await loadRun();
}

async function selectRun(id) {
  selectedRunId.value = id;
  await loadRun();
}

async function openRun(id) {
  await selectRun(id);
  paneTab.value = "result";
}

// Why the pane has no result yet, in the order the reader can act on it.
const resultHint = computed(() => {
  const recording = selectedRecording.value;
  if (!recording || recordingRuns.value.length) return "";
  if (recording.video.status !== "VERIFIED") return "tests.waitingVerification";
  if (calibrationStatus.value[recording.id]?.status === "WAITING_FOR_CALIBRATION") return "tests.waitingCalibration";
  return "";
});

async function load() {
  error.value = "";
  try {
    // The test carries its trials, so recording one needs no second call.
    test.value = await tests.get(testId.value);
    // Not awaited: what the test belongs to must not hold up the rest of the page.
    if (!apparatus.value) revisions.load().catch(() => {});
    if (!experiment.value) experiments.get(test.value.experimentId).then((value) => (experiment.value = value)).catch(() => {});
    if (!subject.value) subjects.get(test.value.subjectId).then((value) => (subject.value = value)).catch(() => {});
    trials.value = test.value.trials || [];
    recordingList.value = (await recordings.list(testId.value)).data || [];
    await Promise.all(
      recordingList.value.map(async (recording) => {
        calibrationStatus.value[recording.id] = await calibration.status(testId.value, recording.id);
        ownCalibrations.value[recording.id] = (await calibration.list(testId.value, recording.id)).data || [];
      }),
    );
    runs.value = (await analysis.runsOfTest(testId.value)).data || [];
    ensureSelection();
    // The first time the page opens, it opens where the work is: on the trials
    // for a test that is waiting for them and has no video yet, otherwise on the
    // videos.
    if (!pageTab.value) {
      const waitingForTrials = inProgress.value && trials.value.length < test.value.plannedTrials && !recordingList.value.length;
      pageTab.value = waitingForTrials ? "trials" : "videos";
    }
    await loadRun();
    crumb.set([{ label: t("tests.title"), to: "/tests" }, { label: test.value.paradigmKey }]);
  } catch (e) {
    error.value = e.message;
  }
}

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

async function saveCalibration(recordingId, body, done) {
  await act("calibration", async () => {
    await calibration.create(testId.value, recordingId, body);
    done();
  });
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
    uploading.value = false;
    // The new video is what the reader wants to see next.
    await load();
    pageTab.value = "videos";
    await selectRecording(started.recording.id);
  } catch (e) {
    uploading.value = false;
    error.value = e.message;
  } finally {
    busy.value = "";
    event.target.value = "";
  }
}

function reanalyze() {
  const id = selectedRecordingId.value;
  if (analysisPending.value) return;
  return act(id, () => analysis.reanalyze(testId.value, id));
}

// ---- the test and its trials ------------------------------------------------

// A started test waits for its trials, so that is where the page goes next.
async function startTest() {
  await act("start", () => tests.start(testId.value));
  pageTab.value = "trials";
}
const completeTest = () => act("complete", () => tests.complete(testId.value));
const recordTrial = (body) => act("trial", () => tests.recordTrial(testId.value, body));

async function cancelTest() {
  await act("cancel", () => tests.cancel(testId.value, cancelReason.value.trim()));
  cancelling.value = false;
  cancelReason.value = "";
}

const menuOptions = computed(() => [{ label: t("tests.cancel"), key: "cancel", props: { "data-test": "cancel-open" } }]);
const onMenu = (key) => {
  if (key === "cancel") cancelling.value = true;
};

// A run that is waiting or working changes without anyone touching the page, so
// it is read again until it settles.
let poll = null;
function watchActiveRuns() {
  clearInterval(poll);
  poll = setInterval(() => {
    if (!busy.value && runs.value.some((run) => run.status === "QUEUED" || run.status === "RUNNING")) load();
  }, 5000);
}

onMounted(() => {
  load();
  watchActiveRuns();
});
onUnmounted(() => {
  clearInterval(poll);
  crumb.clear();
});
</script>

<template>
  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />

  <div class="detail" v-if="test">
    <NCard :bordered="true" size="small" class="head">
      <PageHead :title="`${test.paradigmKey}${subject ? ` · ${subject.code}` : ''}`">
        <template #actions>
          <NTag :type="tagType(test.status)" round data-test="test-status" :data-status="test.status">
            {{ $t(`statuses.${test.status}`) }}
          </NTag>
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
          <NDropdown v-if="cancellable" trigger="click" :options="menuOptions" @select="onMenu">
            <NButton secondary :aria-label="$t('tests.moreActions')" data-test="test-menu">
              <svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="2" /><circle cx="12" cy="12" r="2" /><circle cx="19" cy="12" r="2" /></svg>
            </NButton>
          </NDropdown>
        </template>
      </PageHead>

      <p class="muted meta" data-test="test-environment" :title="$t('tests.revisionHint')">
        <RouterLink v-if="experiment" class="link" :to="`/experiments/${experiment.id}`">{{ experiment.code }}</RouterLink>
        <template v-if="experiment"> &middot; </template>
        {{ $t("tests.environment") }}:
        <RouterLink v-if="apparatus" class="link" :to="`/environments/${apparatus.environmentId}`">{{ apparatus.environmentName }}</RouterLink>
        <span v-if="apparatus" data-test="test-revision" :data-revision="apparatus.number">
          &middot; {{ $t("tests.revisionNumber", { number: apparatus.number }) }}
        </span>
        &middot; {{ $t("tests.stepOf", { position: test.stepPosition }) }}
      </p>

      <p class="muted" v-if="test.cancelReason" data-test="cancel-reason">
        {{ $t("tests.cancelled", { reason: test.cancelReason }) }}
      </p>
      <FlowSteps
        v-else
        :status="test.status"
        :trials-done="trials.length"
        :trials-planned="test.plannedTrials"
        v-bind="progressCounts"
      />
    </NCard>

    <NTabs v-model:value="pageTab" type="line" display-directive="show:lazy" class="page-tabs">
      <NTabPane name="videos">
        <template #tab><span data-test="page-tab-videos">{{ $t("tests.tabVideos") }}</span></template>
        <div class="workspace">
          <NCard :bordered="true" size="small" class="rail-card">
            <RecordingRail
              :recordings="recordingList"
              :selected-id="selectedRecordingId"
              :stages="stages"
              :can-add="auth.canRun"
              @select="selectRecording"
              @add="uploading = true"
            />
          </NCard>

          <NCard :bordered="true" size="small" class="pane" v-if="selectedRecording" data-test="recording-pane" :data-recording="selectedRecording.id">
            <template #header>
              <span class="pane-title">{{ $t("tests.videoN", { n: selectedNumber }) }}</span>
              <span class="muted file" :title="selectedRecording.video.fileName">{{ selectedRecording.video.fileName }}</span>
            </template>
            <template #header-extra>
              <NTooltip v-if="auth.canRun" :disabled="!analysisPending">
                <template #trigger>
                  <span class="reanalyze-trigger" data-test="reanalyze-wrap">
                    <NButton
                      size="small"
                      secondary
                      :loading="busy === selectedRecordingId"
                      :disabled="!!busy || analysisPending"
                      data-test="reanalyze"
                      @click="reanalyze"
                    >
                <template #icon>
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-3-6.7" /><polyline points="21 3 21 9 15 9" /></svg>
                </template>
                {{ $t("analysis.reanalyze") }}
                    </NButton>
                  </span>
                </template>
                {{ $t("analysis.reanalyzePending") }}
              </NTooltip>
            </template>

            <NTabs v-model:value="paneTab" type="line" animated display-directive="show:lazy">
              <NTabPane name="result">
                <template #tab><span data-test="tab-result">{{ $t("tests.tabResult") }}</span></template>
                <template v-if="resultHint">
                  <p class="muted" data-test="result-hint">{{ $t(resultHint) }}</p>
                  <NButton v-if="resultHint === 'tests.waitingCalibration'" size="small" secondary data-test="go-calibration" @click="paneTab = 'calibration'">
                    {{ $t("tests.tabCalibration") }}
                  </NButton>
                </template>
                <ResultPanel
                  v-else
                  :runs="recordingRuns"
                  :run-id="selectedRunId"
                  :detail="detail"
                  :tag-type="tagType"
                  @select-run="selectRun"
                  @error="error = $event"
                />
              </NTabPane>
              <NTabPane name="calibration">
                <template #tab><span data-test="tab-calibration">{{ $t("tests.tabCalibration") }}</span></template>
                <CalibrationPanel
                  :recording="selectedRecording"
                  :status="calibrationStatus[selectedRecording.id]"
                  :own="ownCalibrations[selectedRecording.id] || []"
                  :can-enter="auth.canRun"
                  :saving="busy === 'calibration'"
                  :disabled="!!busy"
                  :tag-type="tagType"
                  @save="(body, done) => saveCalibration(selectedRecording.id, body, done)"
                />
              </NTabPane>
              <NTabPane name="runs">
                <template #tab><span data-test="tab-runs">{{ $t("tests.tabRuns") }} ({{ recordingRuns.length }})</span></template>
                <RunsPanel :runs="recordingRuns" :selected-id="selectedRunId" :tag-type="tagType" @open="openRun" />
              </NTabPane>
            </NTabs>
          </NCard>
          <NCard v-else :bordered="true" size="small" class="pane">
            <p class="muted" data-test="no-recording">{{ $t("tests.selectRecording") }}</p>
          </NCard>
        </div>
      </NTabPane>

      <NTabPane name="trials">
        <template #tab><span data-test="page-tab-trials">{{ $t("tests.tabTrials") }} ({{ trials.length }})</span></template>
        <NCard :bordered="true" size="small">
          <TrialsPanel :test="test" :trials="trials" :can-record="auth.canRun" :busy="busy" @record="recordTrial" />
        </NCard>
      </NTabPane>

      <!-- The thread belongs to the test. -->
      <NTabPane name="comments">
        <template #tab><span data-test="page-tab-comments">{{ $t("tests.tabComments") }}</span></template>
        <TestComments :test-id="testId" />
      </NTabPane>
    </NTabs>

    <UploadDialog v-model:show="uploading" v-model:clip="clip" :busy="busy" :progress="progress" @upload="uploadRecording" />

    <NModal v-model:show="cancelling" preset="card" :title="$t('tests.cancel')" style="width: 420px; max-width: 94vw">
      <NInput v-model:value="cancelReason" :placeholder="$t('tests.cancelReason')" :input-props="{ 'data-test': 'cancel-reason-input' }" />
      <template #footer>
        <div class="dialog-actions">
          <NButton @click="cancelling = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="error" :disabled="!!busy || !cancelReason.trim()" :loading="busy === 'cancel'" data-test="cancel-test" @click="cancelTest">
            {{ $t("tests.cancel") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.head :deep(.page-head) { margin-bottom: 4px; }
.meta { margin: 0 0 14px; }
.workspace { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 16px; align-items: start; }
.pane { min-width: 0; }
.pane-title { font-weight: 700; margin-right: 10px; }
.file { font-weight: 400; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: inline-block; max-width: 40ch; vertical-align: bottom; }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
@media (max-width: 900px) { .workspace { grid-template-columns: minmax(0, 1fr); } }
</style>
