<script setup>
// Records one dose. A planned dose comes from the day's list and only asks
// when it was given; an unplanned one asks for everything. A per-kilogram dose
// needs a body weight from the last seven days: when the animal has none, the
// weight is asked for first and recorded before the dose, which then uses it.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NDatePicker, NForm, NFormItem, NInput, NModal, NSelect, NSpace, useMessage } from "naive-ui";
import { dosing, studyDesign } from "../../api/dosing.js";
import { useAuth } from "../../stores/auth.js";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";
import { filterByText, selectOption } from "../../composables/selectOptions.js";

const props = defineProps({
  // { animal, dose } from the due list, or { unplanned: true }.
  target: { type: Object, required: true },
  date: { type: String, default: null },
  today: { type: String, default: null },
  allocations: { type: Array, default: () => [] },
  experimentId: { type: String, required: true },
});
const emit = defineEmits(["close", "recorded"]);
const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const fmt = useExecutionFormat();

const UNITS = ["mg/kg", "ug/kg", "mL/kg", "IU/kg", "mg", "ug", "g", "mL", "uL", "IU"];
const ROUTES = ["ORAL", "INTRAPERITONEAL", "SUBCUTANEOUS", "INTRAVENOUS", "INTRAMUSCULAR", "INTRANASAL", "INTRACEREBROVENTRICULAR", "TOPICAL", "INHALATION"];
const PURPOSES = ["TREATMENT", "VEHICLE", "INDUCTION"];
const WEIGHT_MAX_AGE_MS = 7 * 24 * 3600 * 1000;

const planned = computed(() => !props.target.unplanned);
const animal = computed(() => props.target.animal || null);
const dose = computed(() => props.target.dose || null);

// When it was given: now on today's list, midday of a past day otherwise.
function defaultTime() {
  if (!props.date || props.date === props.today) return Date.now();
  return new Date(`${props.date}T12:00:00`).getTime();
}

const form = ref({
  allocationId: animal.value?.allocationId || null,
  purpose: "TREATMENT",
  substanceId: null,
  amount: dose.value?.dose.amount || "",
  unit: dose.value?.dose.unit || "mg/kg",
  route: dose.value?.route || "ORAL",
  administeredAt: defaultTime(),
  notes: "",
  grams: "",
});
const substances = ref([]);
const saving = ref(false);
const error = ref("");
// A weight recorded in this dialog, kept so a retry does not weigh twice.
const weighed = ref(null);

const allocation = computed(() => props.allocations.find((a) => a.id === form.value.allocationId) || null);
const subjectId = computed(() => animal.value?.subjectId || allocation.value?.subjectId);
const perKg = computed(() => form.value.unit.endsWith("/kg"));
// What the API will use without a new weight: known for a planned dose only.
const knownWeight = computed(() => animal.value?.weight || null);
const weightNeeded = computed(() => {
  if (!perKg.value || weighed.value) return false;
  if (!planned.value) return false;
  if (!knownWeight.value) return true;
  return form.value.administeredAt - new Date(knownWeight.value.measuredAt).getTime() > WEIGHT_MAX_AGE_MS;
});
const canWeigh = computed(() => auth.can("weight:write"));

const animalOptions = computed(() =>
  props.allocations.filter((a) => !a.excluded).map((a) => selectOption("dose-animal", a.id, a.subjectCode)),
);
const substanceOptions = computed(() => substances.value.map((s) => selectOption("dose-substance", s.id, s.name)));
const unitOptions = UNITS.map((u) => selectOption("dose-unit", u, u));
const routeOptions = computed(() => ROUTES.map((r) => selectOption("dose-route", r, fmt.route(r))));
const purposeOptions = computed(() => PURPOSES.map((p) => selectOption("dose-purpose", p, fmt.purpose(p))));

const AMOUNT = /^[0-9]{1,9}(\.[0-9]{1,6})?$/;
const GRAMS = /^[0-9]{1,4}(\.[0-9]{1,3})?$/;
const amountText = (value) => String(value).trim().replace(",", ".");

const valid = computed(() => {
  const f = form.value;
  if (!f.allocationId || !f.administeredAt) return false;
  if (!planned.value && (!f.substanceId || !AMOUNT.test(amountText(f.amount)))) return false;
  if (planned.value && f.amount !== "" && !AMOUNT.test(amountText(f.amount))) return false;
  const grams = amountText(f.grams);
  if (grams && !GRAMS.test(grams)) return false;
  if (weightNeeded.value && !grams) return false;
  return true;
});

async function submit() {
  saving.value = true;
  error.value = "";
  const f = form.value;
  const administeredAt = new Date(f.administeredAt).toISOString();
  try {
    const grams = amountText(f.grams);
    if (perKg.value && grams && !weighed.value) {
      weighed.value = await dosing.recordWeight(subjectId.value, { grams, measuredAt: administeredAt });
    }
    const body = { administeredAt };
    if (planned.value) {
      body.regimenItemId = dose.value.regimenItemId;
      // Only a dose that differs from the plan names its amount and unit.
      const amount = amountText(f.amount);
      if (amount && (amount !== dose.value.dose.amount || f.unit !== dose.value.dose.unit)) {
        body.amount = amount;
        body.unit = f.unit;
      }
      if (f.route !== dose.value.route) body.route = f.route;
    } else {
      Object.assign(body, { purpose: f.purpose, substanceId: f.substanceId, amount: amountText(f.amount), unit: f.unit, route: f.route });
    }
    if (weighed.value) body.weightMeasurementId = weighed.value.id;
    if (f.notes.trim()) body.notes = f.notes.trim();
    const given = await dosing.record(props.experimentId, f.allocationId, body);
    message.success(t("execution.dosing.recorded", { dose: fmt.dose(given.absoluteDose || given.dose) }));
    emit("recorded");
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  if (planned.value) return;
  try {
    substances.value = (await studyDesign.substances()).data || [];
  } catch (e) {
    error.value = e.message;
  }
});
</script>

<template>
  <NModal
    :show="true"
    preset="card"
    :title="planned ? $t('execution.dosing.recordTitle', { animal: animal.subjectCode }) : $t('execution.dosing.unplannedTitle')"
    style="width: 520px; max-width: 94vw"
    data-test="dose-dialog"
    @update:show="(v) => !v && emit('close')"
  >
    <p v-if="planned" class="planned" data-test="dose-planned">
      <strong>{{ dose.substanceName }}</strong> · {{ fmt.dose(dose.dose) }} · {{ fmt.route(dose.route) }} · {{ fmt.purpose(dose.purpose) }}
      <template v-if="dose.absoluteDose && dose.dose.unit.endsWith('/kg')"> = {{ fmt.dose(dose.absoluteDose) }}</template>
    </p>
    <NForm label-placement="top" :show-feedback="false" class="form" @submit.prevent="submit">
      <template v-if="!planned">
        <NFormItem :label="$t('execution.animal')">
          <NSelect v-model:value="form.allocationId" filterable :filter="filterByText" :options="animalOptions" data-test="dose-animal" />
        </NFormItem>
        <div class="row">
          <NFormItem :label="$t('execution.dosing.purpose')">
            <NSelect v-model:value="form.purpose" :options="purposeOptions" data-test="dose-purpose" />
          </NFormItem>
          <NFormItem :label="$t('execution.dosing.substance')">
            <NSelect v-model:value="form.substanceId" filterable :filter="filterByText" :options="substanceOptions" data-test="dose-substance" />
          </NFormItem>
        </div>
      </template>

      <!-- The weight comes first: a per-kilogram dose is computed from it. -->
      <div v-if="perKg" class="weight" data-test="dose-weight-step">
        <NAlert v-if="weightNeeded" type="warning" :show-icon="true" data-test="weight-needed">
          {{ knownWeight ? $t("execution.dosing.weightStale", { date: fmt.date(knownWeight.measuredAt) }) : $t("execution.dosing.weightMissing") }}
        </NAlert>
        <p v-else-if="weighed" class="muted" data-test="weight-recorded">{{ $t("execution.dosing.weightRecorded", { weight: fmt.grams(weighed.grams) }) }}</p>
        <p v-else-if="knownWeight" class="muted">
          {{ $t("execution.dosing.weightKnown", { weight: fmt.grams(knownWeight.grams), date: fmt.date(knownWeight.measuredAt) }) }}
        </p>
        <p v-else class="muted">{{ $t("execution.dosing.weightOptional") }}</p>
        <NFormItem v-if="!weighed && canWeigh" :label="$t('execution.dosing.bodyWeight')">
          <NInput v-model:value="form.grams" :placeholder="$t('execution.dosing.gramsPlaceholder')" :input-props="{ inputmode: 'decimal', 'data-test': 'dose-grams' }">
            <template #suffix>g</template>
          </NInput>
        </NFormItem>
        <NAlert v-else-if="weightNeeded" type="error" :show-icon="false">{{ $t("execution.dosing.cannotWeigh") }}</NAlert>
      </div>

      <div class="row">
        <NFormItem :label="planned ? $t('execution.dosing.actualAmount') : $t('execution.dosing.amount')">
          <NInput v-model:value="form.amount" :input-props="{ inputmode: 'decimal', 'data-test': 'dose-amount' }" />
        </NFormItem>
        <NFormItem :label="$t('execution.dosing.unit')">
          <NSelect v-model:value="form.unit" :options="unitOptions" data-test="dose-unit" />
        </NFormItem>
        <NFormItem :label="$t('execution.dosing.route')">
          <NSelect v-model:value="form.route" :options="routeOptions" data-test="dose-route" />
        </NFormItem>
      </div>
      <NFormItem :label="$t('execution.dosing.givenAt')">
        <NDatePicker v-model:value="form.administeredAt" type="datetime" :format="$t('execution.dateTimeFormat')" :clearable="false" :is-date-disabled="(ts) => ts > Date.now()" data-test="dose-time" />
      </NFormItem>
      <NFormItem :label="$t('execution.notes')">
        <NInput v-model:value="form.notes" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" maxlength="2000" :input-props="{ 'data-test': 'dose-notes' }" />
      </NFormItem>
      <NAlert v-if="error" type="error" :title="error" data-test="dose-error" />
    </NForm>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="emit('close')">{{ $t("common.cancel") }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!valid" data-test="dose-save" @click="submit">
          {{ weightNeeded || (perKg && form.grams && !weighed) ? $t("execution.dosing.saveWeightAndDose") : $t("execution.dosing.save") }}
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>

<style scoped>
.planned { margin: 0 0 12px; }
.form { display: flex; flex-direction: column; gap: 12px; }
.row { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; }
.weight { display: flex; flex-direction: column; gap: 8px; padding: 10px; border: 1px solid var(--line); border-radius: 10px; }
.weight p { margin: 0; }
</style>
