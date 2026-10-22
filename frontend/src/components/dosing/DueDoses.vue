<script setup>
// The doses due on one day, one table per group, one row per animal. Each dose
// says whether it was given, and offers to record it when it was not.
import { computed, h } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NEmpty, NSpin, NTag } from "naive-ui";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";

const props = defineProps({
  schedule: { type: Object, default: null },
  groups: { type: Array, default: () => [] },
  // Groups without any regimen item: never due a dose, listed as such.
  undosedGroups: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  // The day shown is before or after the laboratory's today.
  past: { type: Boolean, default: false },
  future: { type: Boolean, default: false },
  canRecord: { type: Boolean, default: false },
});
const emit = defineEmits(["record"]);
const { t } = useI18n();
const fmt = useExecutionFormat();

const WEIGHT_MAX_AGE_MS = 7 * 24 * 3600 * 1000;
const perKg = (unit) => unit.endsWith("/kg");

const sections = computed(() => {
  const byGroup = new Map();
  for (const animal of props.schedule?.animals || []) {
    if (!byGroup.has(animal.groupId)) {
      const group = props.groups.find((g) => g.id === animal.groupId);
      byGroup.set(animal.groupId, { id: animal.groupId, code: animal.groupCode, group, animals: [] });
    }
    byGroup.get(animal.groupId).animals.push(animal);
  }
  return [...byGroup.values()];
});

// A weight is usable for a per-kilogram dose for seven days.
function weightState(animal) {
  if (!animal.doses.some((d) => perKg(d.dose.unit))) return "";
  if (!animal.weight) return "missing";
  const endOfDay = new Date(`${props.schedule.date}T23:59:59`).getTime();
  return endOfDay - new Date(animal.weight.measuredAt).getTime() > WEIGHT_MAX_AGE_MS ? "stale" : "";
}

function doseState(d) {
  if (d.complete) return { key: "given", type: "success" };
  if (props.future) return { key: "planned", type: "default" };
  return props.past ? { key: "missing", type: "error" } : { key: "due", type: "warning" };
}

function renderDose(animal, d) {
  const state = doseState(d);
  const absolute = d.absoluteDose && perKg(d.dose.unit) ? ` = ${fmt.dose(d.absoluteDose)}` : "";
  return h("div", { class: "dose", "data-test": "due-dose", "data-animal": animal.subjectCode, "data-state": state.key }, [
    h("div", { class: "dose-text" }, [
      h("strong", d.substanceName),
      h("span", ` · ${fmt.dose(d.dose)}${absolute}`),
      h("span", { class: "muted" }, ` · ${fmt.route(d.route)} · ${fmt.purpose(d.purpose)}`),
    ]),
    h("div", { class: "dose-actions" }, [
      h(NTag, { size: "small", round: true, bordered: false, type: state.type, "data-test": "dose-state" }, () =>
        `${t(`execution.dosing.state.${state.key}`)}${d.due > 1 ? ` ${d.given}/${d.due}` : ""}`,
      ),
      props.canRecord && !d.complete && !props.future
        ? h(NButton, { size: "tiny", type: "primary", secondary: true, "data-test": "record-dose", onClick: () => emit("record", animal, d) }, () =>
            t("execution.dosing.record"),
          )
        : null,
    ]),
  ]);
}

// Renders the animal and weight cells of one row.
function renderAnimal(animal) {
  return h("div", { class: "who" }, [
    h("strong", { "data-test": "due-animal" }, animal.subjectCode),
    animal.cage ? h("span", { class: "muted small" }, t("execution.cage", { cage: animal.cage })) : null,
    animal.excluded ? h(NTag, { size: "tiny", type: "error", bordered: false }, () => t("execution.excluded")) : null,
  ]);
}
function renderWeight(animal) {
  const state = weightState(animal);
  return h("div", { class: "weight", "data-test": "due-weight", "data-state": state || "ok" }, [
    animal.weight ? h("span", fmt.grams(animal.weight.grams)) : null,
    animal.weight ? h("span", { class: "muted small" }, fmt.date(animal.weight.measuredAt)) : null,
    state ? h(NTag, { size: "tiny", type: "warning", bordered: false }, () => t(`execution.dosing.weightState.${state}`)) : null,
  ]);
}
const Row = (rowProps) =>
  h("div", { class: "row", "data-test": "due-row", "data-animal": rowProps.animal.subjectCode }, [
    renderAnimal(rowProps.animal),
    renderWeight(rowProps.animal),
    h("div", { class: "doses" }, rowProps.animal.doses.map((d) => renderDose(rowProps.animal, d))),
  ]);
Row.props = ["animal"];
</script>

<template>
  <NSpin :show="loading">
    <NEmpty v-if="schedule && !sections.length" :description="$t('execution.dosing.nothingDue')" data-test="nothing-due" />
    <div v-for="section in sections" :key="section.id" class="section" data-test="due-group" :data-group="section.code">
      <h4>
        {{ section.code }}<template v-if="section.group"> · {{ section.group.name }}</template>
        <NTag v-if="section.group" size="small" round :bordered="false" class="role">{{ $t(`execution.roles.${section.group.role}`) }}</NTag>
      </h4>
      <div class="rows">
        <div class="row head muted">
          <span>{{ $t("execution.animal") }}</span><span>{{ $t("execution.dosing.weight") }}</span><span>{{ $t("execution.dosing.doses") }}</span>
        </div>
        <Row v-for="animal in section.animals" :key="animal.allocationId" :animal="animal" />
      </div>
    </div>
    <ul v-if="schedule && undosedGroups.length" class="undosed">
      <li v-for="g in undosedGroups" :key="g.id" data-test="undosed-group" :data-group="g.code">
        <strong>{{ g.code }}</strong> · {{ g.name }}
        <NTag size="small" round :bordered="false" class="role">{{ $t(`execution.roles.${g.role}`) }}</NTag>
        <span class="muted">{{ $t("execution.dosing.noDosing") }}</span>
      </li>
    </ul>
  </NSpin>
</template>

<style scoped>
.section + .section { margin-top: 18px; }
h4 { margin: 0 0 8px; font-size: 14px; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.role { font-weight: 500; }
.undosed { list-style: none; margin: 18px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; font-size: 14px; }
.undosed li { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.rows { border: 1px solid var(--line); border-radius: 10px; overflow: hidden; }
:deep(.row) { display: grid; grid-template-columns: 110px 130px minmax(0, 1fr); gap: 12px; padding: 10px 12px; align-items: center; }
:deep(.row + .row) { border-top: 1px solid var(--line); }
.head { font-size: 12px; font-weight: 600; background: var(--panel2); }
:deep(.who), :deep(.weight) { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; }
@media (max-width: 640px) {
  :deep(.row) { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
  :deep(.row .doses) { grid-column: 1 / -1; }
  .head { display: none; }
}
:deep(.doses) { display: flex; flex-direction: column; gap: 8px; }
:deep(.dose) { display: flex; justify-content: space-between; align-items: center; gap: 10px; flex-wrap: wrap; }
:deep(.dose-actions) { display: flex; align-items: center; gap: 8px; }
:deep(.small) { font-size: 12px; }
</style>
