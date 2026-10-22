<script setup>
// The Dosing tab of a study: what is due on a day for every animal, grouped by
// group, with a quick way to record it; unplanned doses; the history of every
// dose given; and, for diseased groups, whether each animal's model was
// confirmed. Doses are recorded only while the study runs.
import { computed, inject, onMounted, ref, watch } from "vue";
import { NAlert, NButton, NCard, NDatePicker, NSpace, NTag } from "naive-ui";
import { dosing, studyDesign } from "../../../api/dosing.js";
import { useAuth } from "../../../stores/auth.js";
import { addDays, useExecutionFormat } from "../../../composables/useExecutionFormat.js";
import DueDoses from "../../../components/dosing/DueDoses.vue";
import RecordDoseModal from "../../../components/dosing/RecordDoseModal.vue";
import DoseHistory from "../../../components/dosing/DoseHistory.vue";
import InductionOutcomes from "../../../components/dosing/InductionOutcomes.vue";

const { experiment, id } = inject("study");
const auth = useAuth();
const fmt = useExecutionFormat();

const running = computed(() => experiment.value?.status === "RUNNING");
const started = computed(() => experiment.value?.status !== "DRAFT");
const canRecord = computed(() => running.value && auth.canRun);

const date = ref(null); // "YYYY-MM-DD"; null until the API told us today's date
const today = ref(null);
const schedule = ref(null);
const groups = ref([]);
const regimenItems = ref([]);
const allocations = ref([]);
const loading = ref(false);
const error = ref("");
const historyKey = ref(0);
const outcomesKey = ref(0);

// What the record dialog is about: { animal, dose } for a planned dose, or
// { unplanned: true } for a dose outside the regimen.
const recording = ref(null);

async function loadSchedule() {
  if (!started.value) return;
  loading.value = true;
  error.value = "";
  try {
    schedule.value = await dosing.schedule(id.value, date.value || undefined);
    if (!date.value) {
      date.value = schedule.value.date;
      today.value = schedule.value.date;
    }
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

async function loadDesign() {
  try {
    const [groupList, itemList, allocationList] = await Promise.all([
      studyDesign.groups(id.value),
      studyDesign.regimenItems(id.value),
      studyDesign.allocations(id.value),
    ]);
    groups.value = groupList.data || [];
    regimenItems.value = itemList.data || [];
    allocations.value = allocationList.data || [];
  } catch (e) {
    error.value = e.message;
  }
}

watch(date, (value, previous) => {
  if (previous) loadSchedule();
});

// Groups with no regimen item (a naive healthy control) are never due a dose;
// the schedule names them so their absence is not mistaken for a gap.
const undosedGroups = computed(() => groups.value.filter((g) => !regimenItems.value.some((i) => i.groupId === g.id)));

const shift = (days) => (date.value = addDays(date.value, days));
// NDatePicker speaks timestamps; the tab speaks calendar dates.
const pickerValue = computed({
  get: () => (date.value ? new Date(`${date.value}T12:00:00`).getTime() : null),
  set: (value) => {
    if (!value) return;
    const d = new Date(value);
    date.value = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  },
});

const totals = computed(() => {
  let due = 0;
  let given = 0;
  for (const animal of schedule.value?.animals || []) {
    for (const d of animal.doses) {
      due += d.due;
      given += Math.min(d.given, d.due);
    }
  }
  return { due, given };
});

function recorded() {
  recording.value = null;
  loadSchedule();
  historyKey.value++;
}

onMounted(() => {
  loadDesign();
  loadSchedule();
});
watch(started, (value) => value && loadSchedule());
</script>

<template>
  <div class="dosing" data-test="dosing-tab">
    <NAlert v-if="!started" type="info" :title="$t('execution.dosing.notStarted')" data-test="dosing-not-started" />
    <template v-else>
      <NAlert v-if="error" type="error" :title="error" data-test="dosing-error" />

      <NCard size="small" :bordered="true">
        <template #header>
          <span>{{ $t("execution.dosing.dueTitle") }}</span>
        </template>
        <template #header-extra>
          <NButton v-if="canRecord" size="small" secondary data-test="unplanned-dose" @click="recording = { unplanned: true }">
            {{ $t("execution.dosing.unplanned") }}
          </NButton>
        </template>
        <div class="daybar">
          <NSpace align="center" :wrap="true" :size="8">
            <NButton size="small" quaternary :aria-label="$t('execution.dosing.previousDay')" data-test="day-previous" :disabled="!date" @click="shift(-1)">‹</NButton>
            <NDatePicker
              v-model:value="pickerValue"
              type="date"
              size="small"
              :format="$t('execution.dateFormat')"
              :clearable="false"
              data-test="dosing-date"
              style="width: 150px"
            />
            <NButton size="small" quaternary :aria-label="$t('execution.dosing.nextDay')" data-test="day-next" :disabled="!date" @click="shift(1)">›</NButton>
            <NButton v-if="today && date !== today" size="small" text type="primary" data-test="day-today" @click="date = today">
              {{ $t("execution.dosing.today") }}
            </NButton>
          </NSpace>
          <NSpace v-if="schedule" align="center" :size="8">
            <NTag round :bordered="false" data-test="study-day">{{ $t("execution.studyDay", { day: schedule.studyDay }) }}</NTag>
            <span class="muted" data-test="dosing-totals">
              {{ $t("execution.dosing.totals", { given: fmt.number(totals.given), due: fmt.number(totals.due) }) }}
            </span>
          </NSpace>
        </div>
        <DueDoses
          :schedule="schedule"
          :groups="groups"
          :undosed-groups="undosedGroups"
          :loading="loading"
          :past="!!today && date < today"
          :future="!!today && date > today"
          :can-record="canRecord"
          @record="(animal, dose) => (recording = { animal, dose })"
        />
      </NCard>

      <InductionOutcomes :key="outcomesKey" :groups="groups" :allocations="allocations" :can-record="canRecord" @recorded="outcomesKey++" />

      <DoseHistory :key="historyKey" :allocations="allocations" :groups="groups" />

      <RecordDoseModal
        v-if="recording"
        :target="recording"
        :date="date"
        :today="today"
        :allocations="allocations"
        :experiment-id="id"
        @close="recording = null"
        @recorded="recorded"
      />
    </template>
  </div>
</template>

<style scoped>
.dosing { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.daybar { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 12px; }
</style>
