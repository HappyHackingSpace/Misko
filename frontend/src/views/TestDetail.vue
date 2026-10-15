<script setup>
// One test session: what it is (study, timepoint, protocol step), where it
// stands, its animals each in their own arena with their trials and the
// calibration of their part of the frame, the session's single video and its
// comments. Analysis results of each animal are shown by the analysis screens.
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import {
  NAlert, NButton, NCard, NInput, NModal, NPopconfirm, NSpace, NTag, useMessage,
} from "naive-ui";
import { tests } from "../api/tests.js";
import { calibration } from "../api/media.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import { tagType, useExecutionFormat } from "../composables/useExecutionFormat.js";
import { useParadigmNames } from "../composables/useParadigmNames.js";
import PageHead from "../components/PageHead.vue";
import AnimalCard from "../components/test/AnimalCard.vue";
import VideoPanel from "../components/test/VideoPanel.vue";
import CalibrationDialog from "../components/test/CalibrationDialog.vue";
import SessionAnimalsEditor from "../components/test/SessionAnimalsEditor.vue";
import TestComments from "../components/test/TestComments.vue";
import SubjectResult from "../components/analysis/SubjectResult.vue";

const { t } = useI18n();
const route = useRoute();
const auth = useAuth();
const crumb = useBreadcrumb();
const message = useMessage();
const fmt = useExecutionFormat();
const { paradigmName } = useParadigmNames();

const test = ref(null);
const calibrationRows = ref([]);
const error = ref("");
const busy = ref("");
const cancelling = ref(false);
const cancelReason = ref("");
const editingAnimals = ref(false);
const calibrating = ref(null); // the test subject being calibrated

const testId = computed(() => route.params.id);
const status = computed(() => test.value?.status);
const planned = computed(() => status.value === "PLANNED");
const inProgress = computed(() => status.value === "IN_PROGRESS");
// Starting, completing, trials, videos and calibrations run the session
// (test:run); planning, changing animals and cancelling write it (test:write).
const canRun = computed(() => auth.canRun);
const canWrite = computed(() => auth.canWriteTests);
const calibrationBySubject = computed(() => Object.fromEntries(calibrationRows.value.map((row) => [row.testSubjectId, row])));
const everyAnimalTried = computed(() => test.value?.subjects.every((s) => (s.trials || []).length > 0));
const excludedAnimal = computed(() => test.value?.subjects.some((s) => s.excluded));

async function loadCalibration() {
  try {
    calibrationRows.value = (await calibration.status(testId.value)).data || [];
  } catch {
    calibrationRows.value = [];
  }
}

async function load() {
  error.value = "";
  try {
    test.value = await tests.get(testId.value);
    crumb.set([
      { label: t("execution.worklist.title"), to: "/tests" },
      { label: test.value.experimentCode, to: `/experiments/${test.value.experimentId}?tab=tests` },
      { label: test.value.timepointName },
    ]);
    await loadCalibration();
  } catch (e) {
    error.value = e.message;
  }
}

async function act(name, run, success) {
  busy.value = name;
  error.value = "";
  try {
    await run();
    if (success) message.success(success);
    await load();
    return true;
  } catch (e) {
    error.value = e.message;
    return false;
  } finally {
    busy.value = "";
  }
}

const start = () => act("start", () => tests.start(testId.value), t("execution.session.started"));
const complete = () => act("complete", () => tests.complete(testId.value), t("execution.session.completed"));
async function cancel() {
  if (await act("cancel", () => tests.cancel(testId.value, cancelReason.value.trim()), t("execution.session.cancelled"))) {
    cancelling.value = false;
    cancelReason.value = "";
  }
}

function animalsUpdated(updated) {
  test.value = updated;
  loadCalibration();
}

function calibrated() {
  calibrating.value = null;
  loadCalibration();
}

onMounted(load);
watch(testId, load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />

  <div v-if="test" class="detail">
    <PageHead :title="`${test.experimentCode} · ${test.timepointName} · ${paradigmName(test.paradigmKey)}`">
      <template #actions>
        <NTag :type="tagType(test.status)" round :bordered="false" data-test="test-status" :data-status="test.status">{{ fmt.status(test.status) }}</NTag>
        <NButton v-if="planned && canRun" type="primary" :loading="busy === 'start'" :disabled="!!busy || excludedAnimal" data-test="start-test" @click="start">
          {{ $t("execution.session.start") }}
        </NButton>
        <NPopconfirm v-if="inProgress && canRun" :disabled="!everyAnimalTried" @positive-click="complete">
          <template #trigger>
            <NButton type="primary" :loading="busy === 'complete'" :disabled="!!busy || !everyAnimalTried" data-test="complete-test">
              {{ $t("execution.session.complete") }}
            </NButton>
          </template>
          {{ $t("execution.session.completeConfirm") }}
        </NPopconfirm>
        <NButton v-if="(planned || inProgress) && canWrite" secondary type="error" :disabled="!!busy" data-test="cancel-open" @click="cancelling = true">
          {{ $t("execution.session.cancel") }}
        </NButton>
      </template>
    </PageHead>

    <NAlert v-if="planned && excludedAnimal" type="warning" :show-icon="true" data-test="excluded-warning">{{ $t("execution.session.excludedHint") }}</NAlert>
    <NAlert v-if="inProgress && !everyAnimalTried" type="info" :show-icon="true" data-test="needs-trials">{{ $t("execution.session.needsTrials") }}</NAlert>
    <NAlert v-if="test.status === 'CANCELLED'" type="error" :show-icon="false" data-test="cancel-reason">
      {{ $t("execution.session.cancelledBecause", { reason: test.cancelReason, at: fmt.dateTime(test.cancelledAt) }) }}
    </NAlert>

    <NCard size="small" :bordered="true">
      <dl class="facts">
        <div class="fact"><dt>{{ $t('execution.experiment') }}</dt><dd>
          <RouterLink class="link" :to="`/experiments/${test.experimentId}?tab=tests`">{{ test.experimentCode }}</RouterLink>
        </dd></div>
        <div class="fact"><dt>{{ $t('execution.timepoint') }}</dt><dd>{{ test.timepointName }} · {{ $t("execution.studyDay", { day: test.studyDay }) }}</dd></div>
        <div class="fact"><dt>{{ $t('execution.protocol') }}</dt><dd>
          {{ test.protocolName }} · {{ $t("execution.tests.versionN", { n: test.protocolVersion }) }} · {{ $t("execution.tests.stepN", { n: test.stepPosition }) }}
        </dd></div>
        <div class="fact"><dt>{{ $t('execution.paradigm') }}</dt><dd>{{ paradigmName(test.paradigmKey) }}</dd></div>
        <div class="fact"><dt>{{ $t('execution.session.plannedTrials') }}</dt><dd>{{ fmt.number(test.plannedTrials) }}</dd></div>
        <div class="fact"><dt>{{ $t('execution.tests.scheduledAt') }}</dt><dd>{{ fmt.dateTime(test.scheduledAt) }}</dd></div>
        <div v-if="test.startedAt" class="fact"><dt>{{ $t('execution.session.startedAt') }}</dt><dd>{{ fmt.dateTime(test.startedAt) }}</dd></div>
        <div v-if="test.completedAt" class="fact"><dt>{{ $t('execution.session.completedAt') }}</dt><dd>{{ fmt.dateTime(test.completedAt) }}</dd></div>
        <div v-if="test.notes" class="fact"><dt>{{ $t('execution.notes') }}</dt><dd>{{ test.notes }}</dd></div>
      </dl>
    </NCard>

    <section>
      <div class="section-head">
        <h3>{{ $t("execution.tests.animals") }} <span class="muted">({{ test.subjects.length }})</span></h3>
        <NButton v-if="planned && canWrite" size="small" secondary data-test="edit-animals" @click="editingAnimals = true">{{ $t("execution.tests.editAnimals") }}</NButton>
      </div>
      <div class="animals">
        <AnimalCard
          v-for="subject in test.subjects"
          :key="subject.id"
          :test="test"
          :subject="subject"
          :calibration="calibrationBySubject[subject.id]"
          :can-run="canRun"
          @changed="load"
          @calibrate="(s) => (calibrating = s)"
        />
      </div>
    </section>

    <VideoPanel :test="test" :can-upload="canRun" @changed="loadCalibration" />

    <!-- Analysis results and runs, one block per animal. -->
    <NCard size="small" :bordered="true" :title="$t('execution.session.analysisTitle')" data-test="analysis-slot">
      <div class="analysis-list">
        <SubjectResult v-for="s in test.subjects" :key="s.id" :test-id="test.id" :test-subject-id="s.id" @changed="load" />
      </div>
    </NCard>

    <TestComments :test-id="testId" />

    <NModal v-model:show="cancelling" preset="card" :title="$t('execution.session.cancel')" style="width: 440px; max-width: 94vw">
      <p class="muted">{{ $t("execution.session.cancelHint") }}</p>
      <NInput v-model:value="cancelReason" type="textarea" maxlength="2000" :placeholder="$t('execution.session.cancelReason')" :input-props="{ 'data-test': 'cancel-reason-input' }" />
      <template #footer>
        <NSpace justify="end">
          <NButton @click="cancelling = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="error" :disabled="!!busy || !cancelReason.trim()" :loading="busy === 'cancel'" data-test="cancel-test" @click="cancel">
            {{ $t("execution.session.cancel") }}
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <NModal v-model:show="editingAnimals" preset="card" :title="$t('execution.tests.editAnimals')" style="width: 520px; max-width: 94vw">
      <SessionAnimalsEditor :test="test" @updated="animalsUpdated" />
    </NModal>

    <CalibrationDialog
      v-if="calibrating"
      :test-id="test.id"
      :subject="calibrating"
      :status="calibrationBySubject[calibrating.id]"
      @close="calibrating = null"
      @saved="calibrated"
    />
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.section-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; margin-bottom: 10px; }
.section-head h3 { margin: 0; font-size: 16px; }
.animals { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 380px), 1fr)); gap: 16px; }
.facts { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 12px 20px; margin: 0; }
.fact dt { font-size: 12px; color: var(--muted); margin-bottom: 2px; }
.fact dd { margin: 0; overflow-wrap: anywhere; }
.analysis-list { display: flex; flex-direction: column; gap: 16px; }
</style>
