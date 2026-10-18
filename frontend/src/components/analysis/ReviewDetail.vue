<script setup>
// One animal's analysis result as a reviewer reads it: the analysed video pair,
// the quality checks, the metrics and events of a run, the choice of run and the
// decision. In a blinded experiment nothing about the group, dose or substance
// is shown; the API already leaves the group out.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NButton, NCollapse, NCollapseItem, NInput, NSelect, NSpin, NTag, useMessage } from "naive-ui";
import { jobs, results, runs } from "../../api/analysis.js";
import { selectOption } from "../../composables/selectOptions.js";
import EventTimeline from "../EventTimeline.vue";
import MetricTiles from "./MetricTiles.vue";
import VideoPairPanel from "../VideoPairPanel.vue";
import { reviewTag, useNumbers, useParadigmText } from "./format.js";

const props = defineProps({
  // GET /analysis/results/{testSubjectId}: the review item with its succeeded runs.
  item: { type: Object, required: true },
  canReview: { type: Boolean, default: false },
  // Hides the link to the test when the panel already sits on the test page.
  showTestLink: { type: Boolean, default: true },
});
const emit = defineEmits(["changed"]);
const { t, te } = useI18n();
const message = useMessage();
const fmt = useNumbers();
const text = useParadigmText();

const REASONS = ["TRACKING_LOST", "WRONG_ANIMAL", "VIDEO_DEFECT", "PROTOCOL_DEVIATION", "OTHER"];

const viewRunId = ref("");
const detail = ref(null);
const history = ref([]);
const loadingRun = ref(false);
const error = ref("");
const busy = ref("");
const note = ref("");
const reason = ref(null);
const playing = ref(null);

const closed = computed(() => props.item.experimentStatus === "COMPLETED");
const writable = computed(() => props.canReview && !closed.value);
const underReview = computed(() => viewRunId.value === props.item.run.id);
const metrics = computed(() => detail.value?.result?.metrics || (underReview.value ? props.item.run.metrics : []));
const events = computed(() => detail.value?.result?.events || []);
const missing = computed(() => metrics.value.filter((m) => m.value === null));
const manifest = computed(() => text.manifestOf(props.item.paradigmKey, props.item.paradigmVersion));
const metricLabel = (key) => text.metricLabel(props.item.paradigmKey, key);
const eventLabels = computed(() => text.eventLabels(props.item.paradigmKey));

const runOptions = computed(() =>
  (props.item.runs || []).map((run) => {
    const parts = [t(`analysis.${run.trigger}`), fmt.dateTime(run.finishedAt), run.modelVersion].filter(Boolean);
    const badge = run.id === props.item.run.id ? t("review.underReview") : run.id === props.item.latestRunId ? t("review.newest") : "";
    return selectOption("review-run", run.id, parts.join(" · "), badge);
  }),
);

// Quality checks in words. A succeeded run already passed the paradigm's
// measurable QC rules (tracking confidence and lost frames, checked by the API
// on the trajectory); what is left for the reviewer is listed here.
const MEASURED_QC = ["min_tracking_confidence", "max_lost_frame_ratio"];
const failedRuns = computed(() => history.value.filter((run) => run.status === "FAILED"));
function qcRule(rule) {
  const value = rule.unit === "ratio" ? `${fmt.number(rule.value * 100)}%` : fmt.metric(rule.value, rule.unit);
  return te(`review.qcRules.${rule.key}`) ? t(`review.qcRules.${rule.key}`, { value }) : `${rule.key} ${rule.operator} ${value}`;
}
const checks = computed(() => {
  const list = [];
  const rules = (manifest.value?.qc || []).filter((rule) => MEASURED_QC.includes(rule.key));
  if (rules.length) list.push({ tone: "success", key: "qc", text: t("review.checks.passed", { rules: rules.map(qcRule).join(", ") }) });
  if (missing.value.length) {
    const names = missing.value.map((m) => `${metricLabel(m.key)} (${fmt.missingReason(m.missingReason)})`).join(", ");
    list.push({ tone: "warning", key: "missing", text: t("review.checks.missing", { count: missing.value.length, names }) });
  } else if (metrics.value.length) {
    list.push({ tone: "success", key: "measured", text: t("review.checks.allMeasured") });
  }
  if (props.item.newerRunAvailable) list.push({ tone: "info", key: "newer", text: t("review.checks.newerRun") });
  if (props.item.excluded) list.push({ tone: "warning", key: "excluded", text: t("review.checks.excluded") });
  if (failedRuns.value.length) {
    const reasons = [...new Set(failedRuns.value.map((run) => failureText(run.failureReason)))].join(", ");
    list.push({ tone: "warning", key: "failed", text: t("review.checks.failedRuns", { count: failedRuns.value.length, reasons }) });
  }
  list.push({
    tone: "default",
    key: "calibration",
    text: props.item.run.calibrationId ? t("review.checks.calibrated") : t("review.checks.noCalibration"),
  });
  return list;
});
const failureText = (reason) => fmt.failure(reason);

async function loadRun() {
  if (!viewRunId.value) return;
  loadingRun.value = true;
  playing.value = null;
  try {
    detail.value = await jobs.get(viewRunId.value);
  } catch (e) {
    detail.value = null;
    error.value = e.message;
  } finally {
    loadingRun.value = false;
  }
}
async function loadHistory() {
  try {
    history.value = (await runs.ofTest(props.item.testId, props.item.testSubjectId)).data || [];
  } catch {
    history.value = [];
  }
}

watch(
  () => props.item,
  (item, old) => {
    error.value = "";
    text.need(item.paradigmKey, item.paradigmVersion);
    if (!old || old.testSubjectId !== item.testSubjectId) {
      note.value = "";
      reason.value = null;
      loadHistory();
    }
    if (!old || old.testSubjectId !== item.testSubjectId || old.run.id !== item.run.id) {
      viewRunId.value = item.run.id;
    }
  },
  { immediate: true },
);
watch(viewRunId, loadRun, { immediate: true });

async function act(kind, call, success) {
  busy.value = kind;
  error.value = "";
  try {
    const updated = await call();
    message.success(success);
    note.value = "";
    reason.value = null;
    emit("changed", updated);
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = "";
  }
}
const accept = () => act("accept", () => results.accept(props.item.testSubjectId, note.value.trim()), t("review.accepted"));
const reject = () => act("reject", () => results.reject(props.item.testSubjectId, reason.value, note.value.trim()), t("review.rejected"));
const choose = (runId) => act("choose", () => results.chooseRun(props.item.testSubjectId, runId), t("review.runChosen"));

const group = computed(() => props.item.group);
const groupDose = computed(() => (group.value?.doseMicro != null ? fmt.dose(group.value.doseMicro, group.value.unit) : ""));
</script>

<template>
  <div class="review-detail" data-test="review-detail" :data-test-subject="item.testSubjectId">
    <header class="who">
      <div class="who-main">
        <span class="animal" data-test="review-animal">{{ item.subjectCode }}</span>
        <NTag size="small" round :bordered="false" :type="reviewTag(item.status)" data-test="review-status">
          {{ $t(`review.status.${item.status}`) }}
        </NTag>
        <NTag v-if="item.blinded" size="small" round :bordered="false" type="info" data-test="blinded">{{ $t("review.blinded") }}</NTag>
      </div>
      <dl class="facts">
        <div><dt>{{ $t("review.experiment") }}</dt><dd>{{ item.experimentCode }}</dd></div>
        <div><dt>{{ $t("review.timepoint") }}</dt><dd>{{ item.timepointName }}</dd></div>
        <div><dt>{{ $t("review.paradigm") }}</dt><dd>{{ text.paradigmName(item.paradigmKey) }}</dd></div>
        <div><dt>{{ $t("review.arena") }}</dt><dd>{{ item.arena.environmentName }} · {{ $t("review.revision", { n: item.arena.revision }) }}</dd></div>
        <div><dt>{{ $t("review.session") }}</dt><dd>{{ fmt.dateTime(item.scheduledAt) }}</dd></div>
        <div v-if="group && !item.blinded" data-test="review-group">
          <dt>{{ $t("review.group") }}</dt>
          <dd>
            {{ group.code }} · {{ group.name }}
            <span class="muted">({{ $t(`studyReport.roles.${group.role}`) }}<template v-if="group.substanceName">, {{ group.substanceName }} {{ groupDose }}</template>)</span>
          </dd>
        </div>
      </dl>
      <RouterLink v-if="showTestLink" class="link test-link" :to="`/tests/${item.testId}`" data-test="open-test">{{ $t("review.openTest") }}</RouterLink>
    </header>

    <NAlert v-if="item.status !== 'PENDING'" :type="item.status === 'ACCEPTED' ? 'success' : 'error'" :show-icon="false" data-test="decision">
      <strong>{{ $t(`review.decision.${item.status}`) }}</strong>
      <span v-if="item.reviewedAt" class="muted"> · {{ fmt.dateTime(item.reviewedAt) }}</span>
      <div v-if="item.reason">{{ $t(`review.reasons.${item.reason}`) }}</div>
      <div v-if="item.note" class="note">{{ item.note }}</div>
    </NAlert>
    <NAlert v-if="closed" type="default" :show-icon="false">{{ $t("review.closed") }}</NAlert>

    <section>
      <h4>{{ $t("review.quality") }}</h4>
      <ul class="checks" data-test="quality-checks">
        <li v-for="check in checks" :key="check.key" :class="`tone-${check.tone}`" :data-test="`check-${check.key}`">
          <span class="dot"></span><span>{{ check.text }}</span>
          <NButton
            v-if="check.key === 'newer' && writable"
            size="tiny"
            secondary
            :loading="busy === 'choose'"
            data-test="review-newer"
            @click="choose(item.latestRunId)"
          >{{ $t("review.reviewNewer") }}</NButton>
        </li>
      </ul>
    </section>

    <section class="run-pick">
      <h4>{{ $t("review.run") }}</h4>
      <div class="bar">
        <NSelect v-model:value="viewRunId" class="pick" :options="runOptions" data-test="review-run-select" />
        <NButton
          v-if="writable && !underReview"
          type="primary"
          secondary
          :loading="busy === 'choose'"
          data-test="choose-run"
          @click="choose(viewRunId)"
        >{{ $t("review.useThisRun") }}</NButton>
      </div>
      <p v-if="!underReview" class="muted hint">{{ $t("review.previewHint") }}</p>
    </section>

    <NSpin :show="loadingRun">
      <section>
        <h4>{{ $t("review.metrics") }}</h4>
        <MetricTiles class="metrics" :metrics="metrics" :label="metricLabel" test-prefix="review-metric" />
      </section>
      <VideoPairPanel v-if="viewRunId" :job-id="viewRunId" :play="playing" />
      <NCollapse v-if="events.length" class="events">
        <NCollapseItem :title="`${$t('analysis.events')} (${events.length})`" name="events">
          <EventTimeline :events="events" :selected="playing" :labels="eventLabels" @play="playing = $event" />
        </NCollapseItem>
      </NCollapse>
    </NSpin>

    <p v-if="error" class="err" data-test="review-error">{{ error }}</p>

    <section v-if="writable && underReview" class="decide" data-test="review-actions">
      <h4>{{ $t("review.decide") }}</h4>
      <NInput
        v-model:value="note"
        type="textarea"
        :autosize="{ minRows: 2, maxRows: 5 }"
        :maxlength="2000"
        :placeholder="$t('review.notePlaceholder')"
        data-test="review-note"
      />
      <div class="decide-row">
        <NButton type="primary" :loading="busy === 'accept'" :disabled="!!busy" data-test="review-accept" @click="accept">
          {{ item.status === "ACCEPTED" ? $t("review.acceptAgain") : $t("review.accept") }}
        </NButton>
        <span class="spacer"></span>
        <NSelect
          v-model:value="reason"
          class="reason"
          :placeholder="$t('review.reasonPlaceholder')"
          :options="REASONS.map((r) => selectOption('reject-reason', r, $t(`review.reasons.${r}`)))"
          data-test="review-reason"
        />
        <NButton type="error" secondary :disabled="!reason || !!busy" :loading="busy === 'reject'" data-test="review-reject" @click="reject">
          {{ $t("review.reject") }}
        </NButton>
      </div>
      <p v-if="reason === 'OTHER' && !note.trim()" class="muted hint">{{ $t("review.otherNeedsNote") }}</p>
      <NButton v-if="item.status !== 'PENDING'" quaternary size="small" :loading="busy === 'choose'" data-test="review-reopen" @click="choose(item.run.id)">
        {{ $t("review.reopen") }}
      </NButton>
    </section>
  </div>
</template>

<style scoped>
.review-detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.who { display: flex; flex-direction: column; gap: 8px; }
.who-main { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.animal { font-size: 20px; font-weight: 700; letter-spacing: -0.01em; }
.facts { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 6px 14px; margin: 0; }
.facts div { min-width: 0; }
.facts dt { font-size: 11.5px; color: var(--muted); }
.facts dd { margin: 0; font-size: 13.5px; overflow-wrap: anywhere; }
.test-link { font-size: 13px; align-self: flex-start; }
.note { white-space: pre-wrap; margin-top: 4px; }
h4 { margin: 0 0 8px; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }
.checks { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
.checks li { display: grid; grid-template-columns: 8px minmax(0, 1fr) auto; align-items: baseline; gap: 8px; font-size: 13px; }
.checks .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--muted); align-self: center; }
.tone-success .dot { background: var(--good); }
.tone-warning .dot { background: var(--warn); }
.tone-info .dot { background: var(--accent); }
.bar { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.pick { flex: 1; min-width: 220px; }
.hint { margin: 6px 0 0; }
.metrics { margin-bottom: 14px; }
.events { margin-top: 12px; }
.decide { display: flex; flex-direction: column; gap: 10px; border-top: 1px solid var(--line); padding-top: 14px; }
.decide-row { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.spacer { flex: 1; }
.reason { width: 220px; max-width: 100%; }
.decide > :deep(.n-button) { align-self: flex-start; }
</style>
