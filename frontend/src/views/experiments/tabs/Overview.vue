<script setup>
// The study at a glance: its question, how animals are randomised and
// blinded, its lifecycle, and what still keeps a draft from starting. Every
// unmet start rule is written out with its own fix: a button that opens the
// dialog that fixes it, or else a link to the tab that does.
import { computed, inject, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { NAlert, NButton, NCard, NDatePicker, NModal, NPopconfirm, NSpin, NTag, useMessage } from "naive-ui";
import { experiments, groups as groupsApi } from "../../../api/experiments.js";
import { useAuth } from "../../../stores/auth.js";
import { todayISO, useDesignFormat, useTabLink } from "../design/shared.js";

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const fmt = useDesignFormat();
const goTab = useTabLink();
const route = useRoute();
const router = useRouter();
const { experiment, reload, id } = inject("study");

const canWrite = computed(() => auth.can("study:write"));
const readiness = ref(null);
const groups = ref([]);
const loading = ref(false);
const error = ref("");

// Which tab fixes each start rule.
const RULE_TAB = {
  "readiness.noTimepoints": "timeline",
  "readiness.primaryEndpointMissing": "endpoints",
  "readiness.endpointMetricUnavailable": "endpoints",
};
const ruleTab = (rule) => RULE_TAB[rule] || "design";

// Rules a single dialog on the Design tab fixes, and how to open it there
// (see Design.vue). The label names the fix, so two rules on one group never
// read alike.
const ITEM_RULES = {
  "readiness.treatmentItemMissing": ["addTreatment", "TREATMENT"],
  "readiness.inductionItemMissing": ["addInduction", "INDUCTION"],
  "readiness.vehicleItemMissing": ["addVehicle", "VEHICLE"],
};
function ruleAction(problem) {
  const group = groups.value.find((g) => g.id === problem.groupId);
  if (problem.rule === "readiness.noGroups") return { label: "addGroup", query: { addGroup: "" } };
  if (problem.rule === "readiness.healthyControlMissing") {
    return { label: "addHealthyControl", test: "readiness-add-healthy", query: { addGroup: "HEALTHY_CONTROL" } };
  }
  if (!group) return null;
  if (problem.rule === "readiness.diseaseControlMissing") {
    return { label: "addDiseaseControl", query: { addGroup: "DISEASE_CONTROL", diseaseModel: group.diseaseModelId || undefined } };
  }
  if (problem.rule === "readiness.targetSizeMissing") return { label: "setTargetSize", query: { editGroup: group.id } };
  const item = ITEM_RULES[problem.rule];
  return item ? { label: item[0], query: { addItem: group.id, purpose: item[1] } } : null;
}
const runAction = (action) => router.replace({ query: { ...route.query, tab: "design", ...action.query } });

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [r, g] = await Promise.all([experiments.readiness(id.value), groupsApi.list(id.value)]);
    readiness.value = r;
    groups.value = g.data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

const groupLabel = (groupId) => {
  const g = groups.value.find((x) => x.id === groupId);
  return g ? `${g.code} ${g.name}` : "";
};
const ruleText = (problem) => {
  const text = t(`design.readinessRules.${problem.rule.replace("readiness.", "")}`);
  const group = problem.groupId ? groupLabel(problem.groupId) : "";
  return group ? `${group}: ${text}` : text;
};

// Start asks for day 0; the API checks the rules again.
const starting = ref(false);
const dayZero = ref(todayISO());
const startError = ref("");
const busy = ref("");
function openStart() {
  dayZero.value = todayISO();
  startError.value = "";
  starting.value = true;
}
async function start() {
  busy.value = "start";
  startError.value = "";
  try {
    await experiments.start(id.value, dayZero.value);
    starting.value = false;
    message.success(t("design.overview.started"));
    await reload();
    await load();
  } catch (e) {
    startError.value = e.message;
    await load();
  } finally {
    busy.value = "";
  }
}

const completeError = ref("");
async function complete() {
  busy.value = "complete";
  completeError.value = "";
  try {
    await experiments.complete(id.value);
    message.success(t("design.overview.completed"));
    await reload();
    await load();
  } catch (e) {
    completeError.value = e.message;
  } finally {
    busy.value = "";
  }
}

const status = computed(() => experiment.value?.status);
onMounted(load);
watch(status, load);
</script>

<template>
  <div class="overview" v-if="experiment">
    <NAlert v-if="error" type="error" :title="error" />

    <NCard size="small" :title="$t('design.overview.studyCard')">
      <template #header-extra>
        <NButton v-if="canWrite && status !== 'COMPLETED'" size="small" data-test="experiment-edit" @click="$router.push(`/experiments/${id}/edit`)">
          {{ $t("common.edit") }}
        </NButton>
      </template>
      <dl class="facts">
        <dt>{{ $t("design.form.hypothesis") }}</dt>
        <dd data-test="overview-hypothesis">{{ experiment.hypothesis || $t("design.notSet") }}</dd>
        <dt>{{ $t("design.form.randomisation") }}</dt>
        <dd>{{ experiment.randomisation || $t("design.notSet") }}</dd>
        <dt>{{ $t("design.form.blinded") }}</dt>
        <dd>{{ experiment.blinded ? $t("design.overview.blindedYes") : $t("design.overview.blindedNo") }}</dd>
        <dt>{{ $t("design.list.status") }}</dt>
        <dd>
          <NTag size="small" round :bordered="false" :type="{ DRAFT: 'default', RUNNING: 'info', COMPLETED: 'success' }[status]">
            {{ $t(`experiments.status.${status}`) }}
          </NTag>
          <span class="muted small">{{ $t(`design.overview.statusHint.${status}`) }}</span>
        </dd>
        <dt>{{ $t("design.list.dayZero") }}</dt>
        <dd data-test="overview-day-zero">{{ experiment.dayZero ? fmt.date(experiment.dayZero) : $t("design.overview.dayZeroUnset") }}</dd>
        <template v-if="experiment.startedAt">
          <dt>{{ $t("design.overview.startedAt") }}</dt>
          <dd>{{ fmt.dateTime(experiment.startedAt) }}</dd>
        </template>
        <template v-if="experiment.completedAt">
          <dt>{{ $t("design.overview.completedAt") }}</dt>
          <dd>{{ fmt.dateTime(experiment.completedAt) }}</dd>
        </template>
        <template v-if="experiment.description">
          <dt>{{ $t("common.description") }}</dt>
          <dd class="pre">{{ experiment.description }}</dd>
        </template>
      </dl>
    </NCard>

    <NCard v-if="status === 'DRAFT'" size="small" :title="$t('design.overview.readiness')">
      <NSpin :show="loading">
        <template v-if="readiness">
          <NAlert v-if="readiness.ready" type="success" :title="$t('design.overview.ready')" data-test="readiness-ready" />
          <template v-else>
            <p class="muted small">{{ $t("design.overview.notReadyHint") }}</p>
            <ul class="rules" data-test="readiness-unmet">
              <li v-for="(problem, i) in readiness.unmet" :key="i" :data-rule="problem.rule">
                <span class="dot" aria-hidden="true"></span>
                <span class="rule-text">{{ ruleText(problem) }}</span>
                <NButton
                  v-if="canWrite && ruleAction(problem)"
                  size="tiny"
                  secondary
                  type="primary"
                  :data-test="ruleAction(problem).test || 'readiness-action'"
                  @click="runAction(ruleAction(problem))"
                >
                  {{ $t(`design.overview.${ruleAction(problem).label}`) }}
                </NButton>
                <a v-else href="#" class="link" @click.prevent="goTab(ruleTab(problem.rule))">
                  {{ $t("design.overview.fixIn", { tab: $t(`experiments.tabs.${ruleTab(problem.rule)}`) }) }}
                </a>
              </li>
            </ul>
          </template>
        </template>
      </NSpin>
      <div class="actions" v-if="canWrite">
        <NButton type="primary" :disabled="!readiness?.ready" data-test="experiment-start" @click="openStart">
          {{ $t("design.overview.start") }}
        </NButton>
      </div>
    </NCard>

    <NCard v-if="status === 'RUNNING' && canWrite" size="small" :title="$t('design.overview.completeTitle')">
      <p class="muted small">{{ $t("design.overview.completeHint") }}</p>
      <NAlert v-if="completeError" type="error" :title="completeError" data-test="complete-error" style="margin-bottom: 12px" />
      <NPopconfirm @positive-click="complete">
        <template #trigger>
          <NButton type="warning" :loading="busy === 'complete'" data-test="experiment-complete">{{ $t("design.overview.complete") }}</NButton>
        </template>
        {{ $t("design.overview.completeConfirm") }}
      </NPopconfirm>
    </NCard>

    <NModal v-model:show="starting" preset="card" :title="$t('design.overview.startTitle')" style="width: 440px; max-width: 94vw">
      <p class="muted small">{{ $t("design.overview.startHint") }}</p>
      <label class="fld">
        <span>{{ $t("design.list.dayZero") }}</span>
        <NDatePicker v-model:formatted-value="dayZero" value-format="yyyy-MM-dd" type="date" data-test="start-day-zero" />
      </label>
      <NAlert v-if="startError" type="error" :title="startError" data-test="start-error" style="margin-top: 12px" />
      <template #footer>
        <div class="dialog-actions">
          <NButton @click="starting = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :disabled="!dayZero" :loading="busy === 'start'" data-test="start-confirm" @click="start">
            {{ $t("design.overview.start") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.overview { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.facts { display: grid; grid-template-columns: minmax(120px, 200px) 1fr; gap: 10px 16px; margin: 0; }
.facts dt { color: var(--muted); font-size: 13px; }
.facts dd { margin: 0; min-width: 0; overflow-wrap: anywhere; }
.pre { white-space: pre-wrap; }
.small { font-size: 12px; margin-left: 8px; }
p.small { margin: 0 0 10px; }
.rules { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
.rules li { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--warn); flex: none; transform: translateY(-1px); }
.rule-text { flex: 1; min-width: 200px; }
.actions { margin-top: 14px; display: flex; gap: 10px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
@media (max-width: 520px) {
  .facts { grid-template-columns: 1fr; gap: 2px; }
  .facts dd { margin-bottom: 8px; }
}
</style>
