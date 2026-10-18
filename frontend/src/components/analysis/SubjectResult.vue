<script setup>
// One animal of a test: its analysis runs, a manual reanalysis, and the result
// under review with the analysed video, quality checks, metrics and decision.
// Self-contained so the test page only mounts it per animal:
//   <SubjectResult :test-id="test.id" :test-subject-id="subject.id" />
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCollapse, NCollapseItem, NSpin, useMessage } from "naive-ui";
import { results, runs } from "../../api/analysis.js";
import { useAuth } from "../../stores/auth.js";
import ReviewDetail from "./ReviewDetail.vue";
import RunHistory from "./RunHistory.vue";

const props = defineProps({
  testId: { type: String, required: true },
  testSubjectId: { type: String, required: true },
});
// "changed" carries the review item after a decision, a run choice or a reanalysis.
const emit = defineEmits(["changed"]);
const { t } = useI18n();
const auth = useAuth();
const message = useMessage();

const history = ref([]);
const item = ref(null);
const loading = ref(false);
const error = ref("");
const queueing = ref(false);
let timer = null;

const active = computed(() => history.value.some((run) => run.status === "QUEUED" || run.status === "RUNNING"));
const latest = computed(() => history.value[0] || null);
// Without a succeeded run there is nothing to review yet; say why.
const waiting = computed(() => {
  if (item.value) return "";
  if (!history.value.length) return t("subjectResult.noRun");
  if (active.value) return t("subjectResult.inProgress");
  return t("subjectResult.lastFailed");
});

async function load(quiet = false) {
  if (!quiet) loading.value = true;
  try {
    const [list, result] = await Promise.all([
      runs.ofTest(props.testId, props.testSubjectId),
      results.get(props.testSubjectId).catch((e) => (e.code === "analysis.resultNotFound" ? null : Promise.reject(e))),
    ]);
    history.value = list.data || [];
    item.value = result;
    error.value = "";
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
    schedule();
  }
}

// While a run is queued or running, look again every few seconds.
function schedule() {
  clearTimeout(timer);
  if (active.value) timer = setTimeout(() => load(true), 5000);
}

async function reanalyze() {
  queueing.value = true;
  error.value = "";
  try {
    await runs.reanalyze(props.testSubjectId);
    message.success(t("subjectResult.queued"));
    await load(true);
  } catch (e) {
    error.value = e.message;
  } finally {
    queueing.value = false;
  }
}

function onChanged(updated) {
  emit("changed", updated);
  load(true);
}

watch(() => [props.testId, props.testSubjectId], () => load(), { immediate: true });
onBeforeUnmount(() => clearTimeout(timer));
</script>

<template>
  <div class="subject-result" data-test="subject-result" :data-test-subject="testSubjectId">
    <NSpin :show="loading && !item && !history.length">
      <div class="head">
        <p v-if="waiting" class="muted" data-test="result-waiting">{{ waiting }}</p>
        <span class="spacer"></span>
        <NButton
          v-if="auth.can('test:run')"
          size="small"
          secondary
          :loading="queueing"
          :disabled="active"
          data-test="reanalyze"
          @click="reanalyze"
        >{{ $t("subjectResult.reanalyze") }}</NButton>
      </div>
      <p v-if="active" class="muted" data-test="run-active">{{ $t(`subjectResult.active.${latest.status}`) }}</p>
      <NAlert v-if="error" type="error" :show-icon="false" data-test="subject-result-error" style="margin-bottom: 10px">{{ error }}</NAlert>

      <ReviewDetail v-if="item" :item="item" :can-review="auth.can('test:run')" :show-test-link="false" @changed="onChanged" />

      <NCollapse class="history" :default-expanded-names="item ? [] : ['runs']">
        <NCollapseItem :title="`${$t('runs.title')} (${history.length})`" name="runs">
          <RunHistory :runs="history" :reviewed-run-id="item?.run.id || ''" :loading="loading" />
        </NCollapseItem>
      </NCollapse>
    </NSpin>
  </div>
</template>

<style scoped>
.subject-result { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-bottom: 8px; }
.head p { margin: 0; }
.spacer { flex: 1; }
.history { margin-top: 14px; }
</style>
