<script setup>
// One animal of a session in its arena: its trials, the form to record the next
// one while the session runs, and whether its part of the video is calibrated.
// A trial has an order, the repetition of the protocol step it belongs to and
// an attempt: the same repetition run again is attempt 2.
import { computed, ref } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import { NButton, NCard, NDatePicker, NInput, NInputNumber, NTag, useMessage } from "naive-ui";
import { tests } from "../../api/tests.js";
import { tagType, useExecutionFormat } from "../../composables/useExecutionFormat.js";

const props = defineProps({
  test: { type: Object, required: true },
  subject: { type: Object, required: true },
  // This animal's row of the test's calibration status, if loaded.
  calibration: { type: Object, default: null },
  canRun: { type: Boolean, default: false },
  // The paradigm is scored by an observer on the video.
  scorable: { type: Boolean, default: false },
});
const emit = defineEmits(["changed", "calibrate", "score"]);
const { t } = useI18n();
const message = useMessage();
const fmt = useExecutionFormat();

const trials = computed(() => props.subject.trials || []);
const inProgress = computed(() => props.test.status === "IN_PROGRESS");
const repetitionsDone = computed(() => new Set(trials.value.map((tr) => tr.repetition)).size);

// The next repetition, and a start never before the session's own start,
// which the API refuses.
function nextStart() {
  const started = props.test.startedAt ? new Date(props.test.startedAt).getTime() + 1000 : 0;
  return Math.floor(Math.max(Date.now(), started) / 1000) * 1000;
}
const blank = () => ({
  repetition: Math.min(repetitionsDone.value + 1, props.test.plannedTrials),
  startedAt: nextStart(),
  endedAt: null,
  notes: "",
});
const form = ref(blank());
const open = ref(false);
const saving = ref(false);
const error = ref("");

function start() {
  form.value = blank();
  error.value = "";
  open.value = true;
}

async function save() {
  saving.value = true;
  error.value = "";
  try {
    const body = { repetition: form.value.repetition, startedAt: new Date(form.value.startedAt).toISOString() };
    if (form.value.endedAt) body.endedAt = new Date(form.value.endedAt).toISOString();
    if (form.value.notes.trim()) body.notes = form.value.notes.trim();
    await tests.recordTrial(props.test.id, props.subject.id, body);
    message.success(t("execution.trials.recorded", { animal: props.subject.subjectCode }));
    open.value = false;
    emit("changed");
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

const calibrationStatus = computed(() => props.calibration?.status || null);
const source = computed(() => props.calibration?.source || null);
const rejectedOverride = computed(() => calibrationStatus.value === "WAITING_FOR_CALIBRATION" && source.value === "OVERRIDE");
const canCalibrate = computed(() => props.canRun && props.calibration && props.calibration.recordingId && calibrationStatus.value !== "NOT_REQUIRED");
</script>

<template>
  <NCard size="small" :bordered="true" class="animal" data-test="test-animal" :data-animal="subject.subjectCode">
    <template #header>
      <span class="code">{{ subject.subjectCode }}</span>
      <RouterLink class="arena link-muted" :to="`/environments/${subject.environmentId}`">
        {{ subject.environmentName }} · {{ $t("execution.tests.revisionN", { n: subject.environmentRevision }) }}
      </RouterLink>
    </template>
    <template #header-extra>
      <NTag v-if="subject.excluded" size="small" type="error" :bordered="false" data-test="animal-excluded">{{ $t("execution.excluded") }}</NTag>
    </template>

    <div class="block">
      <div class="block-head">
        <strong>{{ $t("execution.trials.title") }}</strong>
        <span class="muted" data-test="trial-count">{{ $t("execution.trials.count", { done: repetitionsDone, planned: test.plannedTrials }) }}</span>
      </div>
      <table v-if="trials.length" class="table">
        <thead>
          <tr>
            <th>#</th>
            <th>{{ $t("execution.trials.repetition") }}</th>
            <th>{{ $t("execution.trials.attempt") }}</th>
            <th>{{ $t("execution.trials.started") }}</th>
            <th>{{ $t("execution.trials.ended") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="trial in trials" :key="trial.id" data-test="trial" :data-repetition="trial.repetition">
            <td>{{ trial.number }}</td>
            <td>{{ trial.repetition }} / {{ test.plannedTrials }}</td>
            <td>{{ trial.attempt }}</td>
            <td>{{ fmt.time(trial.startedAt) }}</td>
            <td>{{ trial.endedAt ? fmt.time(trial.endedAt) : inProgress ? $t("execution.trials.open") : "" }}</td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted small">{{ inProgress ? $t("execution.trials.needed") : $t("execution.trials.none") }}</p>

      <template v-if="inProgress && canRun && !subject.excluded">
        <NButton v-if="!open" size="small" secondary data-test="trial-open" @click="start">+ {{ $t("execution.trials.record") }}</NButton>
        <form v-else class="trial-form" data-test="trial-form" @submit.prevent="save">
          <label class="fld narrow">
            <span>{{ $t("execution.trials.repetition") }}</span>
            <NInputNumber v-model:value="form.repetition" :min="1" :max="test.plannedTrials" size="small" data-test="trial-repetition" />
          </label>
          <label class="fld">
            <span>{{ $t("execution.trials.started") }}</span>
            <NDatePicker v-model:value="form.startedAt" type="datetime" :format="$t('execution.dateTimeFormat')" size="small" :clearable="false" data-test="trial-start" />
          </label>
          <label class="fld">
            <span>{{ $t("execution.trials.ended") }}</span>
            <NDatePicker v-model:value="form.endedAt" type="datetime" :format="$t('execution.dateTimeFormat')" size="small" clearable data-test="trial-end" />
          </label>
          <label class="fld wide">
            <span>{{ $t("execution.notes") }}</span>
            <NInput v-model:value="form.notes" size="small" maxlength="2000" />
          </label>
          <div class="trial-actions">
            <NButton size="small" @click="open = false">{{ $t("common.cancel") }}</NButton>
            <NButton size="small" type="primary" attr-type="submit" :loading="saving" :disabled="!form.repetition || !form.startedAt" data-test="trial-save">
              {{ $t("execution.trials.save") }}
            </NButton>
          </div>
          <p v-if="error" class="err" data-test="trial-error">{{ error }}</p>
        </form>
      </template>
    </div>

    <div class="block calibration" data-test="animal-calibration" :data-status="calibrationStatus || ''">
      <div class="block-head">
        <strong>{{ $t("execution.calibration.title") }}</strong>
        <NTag v-if="calibrationStatus" size="small" round :bordered="false" :type="tagType(calibrationStatus)" data-test="calibration-status">
          {{ fmt.status(calibrationStatus) }}
        </NTag>
      </div>
      <p v-if="calibration" class="muted small">
        <template v-if="calibrationStatus === 'WAITING_FOR_VIDEO'">{{ $t("execution.calibration.waitingVideo") }}</template>
        <template v-else-if="calibrationStatus === 'NOT_REQUIRED'">{{ $t("execution.calibration.notRequired") }}</template>
        <template v-else-if="rejectedOverride">{{ $t("execution.calibration.overrideRejected") }}</template>
        <template v-else-if="calibrationStatus === 'WAITING_FOR_CALIBRATION'">{{ $t("execution.calibration.waitingCalibration") }}</template>
        <template v-else-if="source">
          <span data-test="calibration-source" :data-source="source">{{ $t(`execution.calibration.sources.${source}`) }}</span>
          <template v-if="calibration.calibration">
            · {{ $t("execution.calibration.checkError", { error: fmt.number(calibration.calibration.checkRmsErrorCm, 2), tolerance: fmt.number(calibration.calibration.toleranceCm, 2) }) }}
          </template>
        </template>
      </p>
      <NButton v-if="canCalibrate" size="small" secondary data-test="calibrate" @click="emit('calibrate', subject)">
        {{ source === "OVERRIDE" ? $t("execution.calibration.correct") : $t("execution.calibration.override") }}
      </NButton>
    </div>

    <div v-if="scorable" class="block" data-test="animal-scoring">
      <strong>{{ $t("scoring.cardTitle") }}</strong>
      <p class="muted small">{{ $t("scoring.cardHint") }}</p>
      <NButton size="small" secondary data-test="score-open" @click="emit('score', subject)">{{ $t("scoring.open") }}</NButton>
    </div>
  </NCard>
</template>

<style scoped>
.animal { min-width: 0; }
.code { font-weight: 700; margin-right: 8px; }
.arena { font-size: 13px; font-weight: 400; }
.block { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
.block > * { max-width: 100%; }
.block-head, .table, .trial-form { align-self: stretch; }
.block + .block { margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--line); }
.block-head { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.small { font-size: 12px; margin: 0; }
.table { width: 100%; border-collapse: collapse; font-size: 13px; }
.table th { text-align: left; font-weight: 600; font-size: 12px; color: var(--muted); padding: 4px 6px; border-bottom: 1px solid var(--line); }
.table td { padding: 6px; border-bottom: 1px solid var(--line); font-variant-numeric: tabular-nums; }
.trial-form { display: flex; gap: 10px; flex-wrap: wrap; align-items: flex-end; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.narrow { width: 110px; }
.wide { flex: 1; min-width: 160px; }
.trial-actions { display: flex; gap: 8px; }
.link-muted { color: var(--muted); text-decoration: none; }
.link-muted:hover { color: var(--txt); text-decoration: underline; }
.err { color: var(--bad); margin: 0; width: 100%; }
</style>
