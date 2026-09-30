<script setup>
// The trials of a test: what was recorded, and the form to record the next one.
// A trial has an order in the test, the repetition of the protocol it belongs to,
// and an attempt: the same repetition run again after it went wrong is attempt 2.
import { computed, ref, watch } from "vue";
import { NButton } from "naive-ui";

const props = defineProps({
  test: { type: Object, required: true },
  trials: { type: Array, default: () => [] },
  canRecord: { type: Boolean, default: false },
  busy: { type: String, default: "" },
});
const emit = defineEmits(["record"]);

const inProgress = computed(() => props.test.status === "IN_PROGRESS");
const when = (value) => (value ? new Date(value).toLocaleString() : "");

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
function defaultStart() {
  const second = 1000;
  const now = Math.floor(Date.now() / second) * second;
  const started = props.test.startedAt ? Math.ceil(new Date(props.test.startedAt).getTime() / second) * second : 0;
  return localValue(new Date(Math.max(now, started)));
}

const instant = (value) => (value ? new Date(value).toISOString() : null);

const blank = () => ({ repetition: 1, startedAt: defaultStart(), endedAt: "", notes: "" });
const form = ref(blank());
// Once the technician edits the start, the screen stops refreshing it.
const touched = ref(false);
watch(
  () => [props.test.startedAt, props.trials.length],
  () => {
    if (!touched.value) form.value.startedAt = defaultStart();
  },
);

function submit() {
  emit("record", {
    repetition: Number(form.value.repetition),
    startedAt: instant(form.value.startedAt) || new Date().toISOString(),
    // An open trial is one that has started and not yet ended.
    endedAt: instant(form.value.endedAt),
    notes: form.value.notes.trim(),
  });
  form.value = blank();
  touched.value = false;
}
</script>

<template>
  <div class="trials">
    <p class="muted summary" data-test="trial-count">
      {{ $t("tests.plannedTrials", { done: trials.length, planned: test.plannedTrials }) }}
    </p>
    <p class="muted needs" v-if="inProgress && !trials.length" data-test="needs-trial">{{ $t("tests.needsTrial") }}</p>

    <form v-if="inProgress && canRecord" class="form" data-test="trial-form" @submit.prevent="submit">
      <label class="fld narrow">
        <span>{{ $t("tests.repetition") }}</span>
        <input type="number" min="1" :max="test.plannedTrials" v-model="form.repetition" data-test="trial-repetition" required />
      </label>
      <label class="fld">
        <span>{{ $t("tests.trialStart") }}</span>
        <input type="datetime-local" step="1" v-model="form.startedAt" data-test="trial-start" required @input="touched = true" />
      </label>
      <label class="fld">
        <span>{{ $t("tests.trialEnd") }}</span>
        <input type="datetime-local" step="1" v-model="form.endedAt" data-test="trial-end" />
      </label>
      <NButton type="primary" attr-type="submit" :loading="busy === 'trial'" :disabled="!!busy" data-test="trial-save">
        {{ $t("tests.recordTrial") }}
      </NButton>
    </form>

    <table v-if="trials.length" class="table">
      <thead>
        <tr>
          <th>{{ $t("tests.col.order") }}</th>
          <th>{{ $t("tests.col.repetition") }}</th>
          <th>{{ $t("tests.col.attempt") }}</th>
          <th>{{ $t("tests.col.started") }}</th>
          <th>{{ $t("tests.col.ended") }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="trial in trials" :key="trial.id" data-test="trial" :data-repetition="trial.repetition">
          <td>{{ trial.number }}</td>
          <td>{{ trial.repetition }} / {{ test.plannedTrials }}</td>
          <td>{{ trial.attempt }}</td>
          <td>{{ when(trial.startedAt) }}</td>
          <td>{{ when(trial.endedAt) || "…" }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else-if="!inProgress" class="muted">{{ $t("tests.noTrials") }}</p>
    <p v-if="trials.length" class="muted help">{{ $t("tests.attemptHelp") }}</p>
  </div>
</template>

<style scoped>
.trials { display: flex; flex-direction: column; gap: 12px; }
.summary, .needs { margin: 0; }
.form { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.narrow { max-width: 110px; }
.table { width: 100%; border-collapse: collapse; font-size: 13px; }
.table th { text-align: left; font-weight: 600; font-size: 12px; color: var(--muted); padding: 6px 10px; border-bottom: 1px solid var(--line); }
.table td { padding: 8px 10px; border-bottom: 1px solid var(--line); font-variant-numeric: tabular-nums; }
.help { margin: 0; font-size: 12px; }
</style>
