<script setup>
// Create or edit one regimen item of a group: what it receives, how much, how
// and when, in study days. A single dose starts and ends on the same day. Only
// the purposes the group's role can receive are offered. A healthy control
// receives no substance: its item is only the period without one, a first and
// a last study day.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NInput, NInputNumber, NModal, NSelect } from "naive-ui";
import { regimenItems } from "../../../api/experiments.js";
import { filterByText, selectOption } from "../../../composables/selectOptions.js";
import { FREQUENCIES, NO_SUBSTANCE, ROLE_PURPOSES, ROUTES, UNITS } from "./shared.js";

const props = defineProps({
  show: { type: Boolean, default: false },
  experimentId: { type: String, required: true },
  group: { type: Object, default: null },
  item: { type: Object, default: null },
  // The group's current regimen items, to offer a treatment only once.
  groupItems: { type: Array, default: () => [] },
  substances: { type: Array, default: () => [] },
  // Purpose chosen up front for a new item, such as the vehicle the
  // readiness panel asks for; ignored when the role cannot receive it.
  purpose: { type: String, default: null },
});
const emit = defineEmits(["update:show", "saved"]);
const { t } = useI18n();

const empty = () => ({
  purpose: null,
  substanceId: null,
  amount: "",
  unit: "mg/kg",
  route: "INTRAPERITONEAL",
  frequency: "DAILY",
  intervalDays: 2,
  startDay: 0,
  endDay: 0,
});
const form = ref(empty());
const error = ref("");
const saving = ref(false);

// A healthy control's item is a period without any substance. Its form asks
// only for the two study days; a vehicle such a group kept from before is
// turned into a period when saved.
const period = computed(() => props.group?.role === "HEALTHY_CONTROL");

watch(
  () => props.show,
  (open) => {
    if (!open) return;
    error.value = "";
    const i = props.item;
    form.value = i
      ? {
          purpose: i.purpose,
          substanceId: i.substanceId,
          amount: String(i.amount),
          unit: i.unit,
          route: i.route,
          frequency: i.frequency,
          intervalDays: i.intervalDays || 2,
          startDay: i.startDay,
          endDay: i.endDay,
        }
      : { ...empty(), purpose: newPurpose() };
  },
);

function newPurpose() {
  if (props.purpose && allowedPurposes.value.includes(props.purpose)) return props.purpose;
  return allowedPurposes.value.length === 1 ? allowedPurposes.value[0] : null;
}

const once = computed(() => form.value.frequency === "ONCE");
watch(
  () => [form.value.frequency, form.value.startDay],
  () => {
    if (once.value && !period.value) form.value.endDay = form.value.startDay;
  },
);

// Substances whose kind fits the purpose come first; any substance may be used.
const KIND_FOR = { INDUCTION: "INDUCER", VEHICLE: "VEHICLE", TREATMENT: null };
const substanceOptions = computed(() => {
  const preferred = KIND_FOR[form.value.purpose];
  const sorted = [...props.substances].sort((a, b) => {
    const rank = (s) => (preferred ? (s.kind === preferred ? 0 : 1) : s.kind === "VEHICLE" || s.kind === "INDUCER" ? 1 : 0);
    return rank(a) - rank(b) || a.name.localeCompare(b.name);
  });
  return sorted.map((s) => selectOption("regimen-substance", s.id, `${s.name} (${t(`design.substanceKinds.${s.kind}`)})`));
});
// The role's purposes, without a treatment when another item of the group is
// already its treatment. An edited item keeps its own purpose in the list so
// the select can show it even if the role no longer allows it.
const allowedPurposes = computed(() => {
  const taken = props.groupItems.some((i) => i.purpose === "TREATMENT" && i.id !== props.item?.id);
  const allowed = (ROLE_PURPOSES[props.group?.role] || []).filter((p) => !(p === "TREATMENT" && taken));
  return props.item && !allowed.includes(props.item.purpose) ? [...allowed, props.item.purpose] : allowed;
});
const purposeOptions = computed(() => allowedPurposes.value.map((p) => selectOption("regimen-purpose", p, t(`design.purposes.${p}`))));
const unitOptions = computed(() => UNITS.map((u) => selectOption("regimen-unit", u, t(`design.units.${u}`))));
const routeOptions = computed(() => ROUTES.map((r) => selectOption("regimen-route", r, t(`design.routes.${r}`))));
const frequencyOptions = computed(() =>
  FREQUENCIES.map((f) => selectOption("regimen-frequency", f, f === "EVERY_N_DAYS" ? t("design.frequencyEveryN") : t(`design.frequencies.${f}`))),
);

// People type "2,5" in Turkish; the API takes "2.5".
const normalisedAmount = computed(() => form.value.amount.trim().replace(",", "."));

function payload() {
  const f = form.value;
  if (period.value) {
    const body = { purpose: NO_SUBSTANCE, startDay: f.startDay, endDay: f.endDay };
    if (!props.item) body.groupId = props.group.id;
    return body;
  }
  const body = {
    purpose: f.purpose,
    substanceId: f.substanceId,
    amount: normalisedAmount.value,
    unit: f.unit,
    route: f.route,
    frequency: f.frequency,
    startDay: f.startDay,
    endDay: once.value ? f.startDay : f.endDay,
  };
  if (f.frequency === "EVERY_N_DAYS") body.intervalDays = f.intervalDays;
  else if (props.item) body.intervalDays = 0;
  if (!props.item) body.groupId = props.group.id;
  return body;
}

async function save() {
  error.value = "";
  saving.value = true;
  try {
    const saved = props.item
      ? await regimenItems.update(props.experimentId, props.item.id, payload())
      : await regimenItems.create(props.experimentId, payload());
    emit("saved", saved);
    emit("update:show", false);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

// A period needs both days; their order is checked by the API (an input
// commits its number only when it loses focus, so the button must stay
// clickable while the last day is still being typed).
const complete = computed(() =>
  period.value
    ? Number.isInteger(form.value.startDay) && Number.isInteger(form.value.endDay)
    : form.value.purpose && form.value.substanceId && normalisedAmount.value !== "" && form.value.unit && form.value.route && form.value.frequency,
);
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="(item ? $t('design.regimen.edit') : $t('design.regimen.add')) + (group ? ` · ${group.code} ${group.name}` : '')"
    style="width: 620px; max-width: 94vw"
    @update:show="emit('update:show', $event)"
  >
    <form v-if="period" class="form" data-test="regimen-form" data-kind="period" @submit.prevent="save">
      <label class="fld">
        <span>{{ $t("design.regimen.startDay") }}</span>
        <NInputNumber v-model:value="form.startDay" :min="-365" :max="3650" :precision="0" :input-props="{ 'data-test': 'regimen-start' }" />
      </label>
      <label class="fld">
        <span>{{ $t("design.regimen.endDay") }}</span>
        <NInputNumber v-model:value="form.endDay" :min="-365" :max="3650" :precision="0" :input-props="{ 'data-test': 'regimen-end' }" />
      </label>
      <small class="muted wide">{{ $t("design.regimen.periodHint") }} {{ $t("design.regimen.daysHint") }}</small>
      <NAlert v-if="error" class="wide" type="error" :title="error" data-test="regimen-error" />
    </form>
    <form v-else class="form" data-test="regimen-form" @submit.prevent="save">
      <div class="fld">
        <span>{{ $t("design.regimen.purpose") }}</span>
        <NSelect v-model:value="form.purpose" :options="purposeOptions" data-test="regimen-purpose" />
      </div>
      <div class="fld">
        <span>{{ $t("design.regimen.substance") }}</span>
        <NSelect v-model:value="form.substanceId" :options="substanceOptions" filterable :filter="filterByText" data-test="regimen-substance" />
        <small v-if="!substances.length" class="muted">
          {{ $t("design.regimen.noSubstances") }}
          <RouterLink to="/substances" data-test="regimen-substances-link">{{ $t("design.regimen.manageSubstances") }}</RouterLink>
        </small>
      </div>
      <label class="fld">
        <span>{{ $t("design.regimen.amount") }}</span>
        <NInput v-model:value="form.amount" inputmode="decimal" :placeholder="$t('design.regimen.amountPlaceholder')" :input-props="{ 'data-test': 'regimen-amount' }" />
      </label>
      <div class="fld">
        <span>{{ $t("design.regimen.unit") }}</span>
        <NSelect v-model:value="form.unit" :options="unitOptions" data-test="regimen-unit" />
      </div>
      <div class="fld">
        <span>{{ $t("design.regimen.route") }}</span>
        <NSelect v-model:value="form.route" :options="routeOptions" data-test="regimen-route" />
      </div>
      <div class="fld">
        <span>{{ $t("design.regimen.frequency") }}</span>
        <NSelect v-model:value="form.frequency" :options="frequencyOptions" data-test="regimen-frequency" />
      </div>
      <label class="fld" v-if="form.frequency === 'EVERY_N_DAYS'">
        <span>{{ $t("design.regimen.intervalDays") }}</span>
        <NInputNumber v-model:value="form.intervalDays" :min="2" :max="365" :precision="0" :input-props="{ 'data-test': 'regimen-interval' }" />
      </label>
      <label class="fld">
        <span>{{ once ? $t("design.regimen.day") : $t("design.regimen.startDay") }}</span>
        <NInputNumber v-model:value="form.startDay" :min="-365" :max="3650" :precision="0" :input-props="{ 'data-test': 'regimen-start' }" />
      </label>
      <label class="fld" v-if="!once">
        <span>{{ $t("design.regimen.endDay") }}</span>
        <NInputNumber v-model:value="form.endDay" :min="-365" :max="3650" :precision="0" :input-props="{ 'data-test': 'regimen-end' }" />
      </label>
      <small class="muted wide">{{ $t("design.regimen.daysHint") }}</small>
      <NAlert v-if="error" class="wide" type="error" :title="error" data-test="regimen-error" />
    </form>
    <template #footer>
      <div class="dialog-actions">
        <NButton @click="emit('update:show', false)">{{ $t("common.cancel") }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!complete" data-test="regimen-save" @click="save">{{ $t("common.save") }}</NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 14px; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.fld > span { font-size: 12px; color: var(--muted); }
.wide { grid-column: 1 / -1; }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
