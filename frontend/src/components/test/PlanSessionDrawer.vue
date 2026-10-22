<script setup>
// Plans one test session: a timepoint, a protocol step, when it is filmed, and
// the animals in it, each in its own arena of the step's paradigm. An animal is
// offered only when it is not excluded and not already in another session of
// the same timepoint and step.
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import {
  NAlert, NButton, NDatePicker, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NSelect, NSpace, useMessage,
} from "naive-ui";
import { tests } from "../../api/tests.js";
import { studyDesign } from "../../api/dosing.js";
import { useArenas } from "../../composables/useArenas.js";
import { useParadigmNames } from "../../composables/useParadigmNames.js";
import { filterByText, selectOption } from "../../composables/selectOptions.js";

const props = defineProps({
  experiment: { type: Object, required: true },
  timepoints: { type: Array, default: () => [] },
  allocations: { type: Array, default: () => [] },
  // Sessions of the study, to leave out animals already planned in a slot.
  sessions: { type: Array, default: () => [] },
  // Preselected timepoint, when planning from a timepoint's section.
  timepointId: { type: String, default: null },
});
const emit = defineEmits(["close", "planned"]);
const { t } = useI18n();
const message = useMessage();
const arenas = useArenas();
const { paradigmName } = useParadigmNames();

const MAX_ANIMALS = 12;
const protocols = ref([]); // [{ protocol, versions }]
const error = ref("");
const saving = ref(false);
const form = ref({
  timepointId: props.timepointId || props.timepoints[0]?.id || null,
  protocolId: null,
  versionId: null,
  stepPosition: null,
  scheduledAt: null,
  notes: "",
  subjects: [{ allocationId: null, environmentRevisionId: null }],
});
const scheduleTouched = ref(false);

const timepoint = computed(() => props.timepoints.find((tp) => tp.id === form.value.timepointId) || null);
const protocolEntry = computed(() => protocols.value.find((p) => p.protocol.id === form.value.protocolId) || null);
const version = computed(() => protocolEntry.value?.versions.find((v) => v.id === form.value.versionId) || null);
const step = computed(() => version.value?.steps.find((s) => s.position === form.value.stepPosition) || null);

// Day 0 plus the timepoint's study day, at nine in the morning.
function defaultSchedule() {
  if (!timepoint.value || !props.experiment.dayZero) return null;
  const [y, m, d] = props.experiment.dayZero.split("-").map(Number);
  return new Date(y, m - 1, d + timepoint.value.studyDay, 9, 0, 0).getTime();
}
watch(
  () => form.value.timepointId,
  () => {
    if (!scheduleTouched.value) form.value.scheduledAt = defaultSchedule();
  },
  { immediate: true },
);
watch(
  () => form.value.protocolId,
  () => {
    const versions = protocolEntry.value?.versions || [];
    form.value.versionId = versions[versions.length - 1]?.id || null;
  },
);
watch(
  () => form.value.versionId,
  () => {
    form.value.stepPosition = version.value?.steps[0]?.position || null;
  },
);
// A different paradigm has different arenas.
watch(step, () => form.value.subjects.forEach((s) => (s.environmentRevisionId = null)));

const timepointOptions = computed(() =>
  props.timepoints.map((tp) => selectOption("plan-timepoint", tp.id, `${tp.name} · ${t("execution.studyDay", { day: tp.studyDay })}`)),
);
const protocolOptions = computed(() => protocols.value.map((p) => selectOption("plan-protocol", p.protocol.id, p.protocol.name)));
const versionOptions = computed(() =>
  (protocolEntry.value?.versions || []).map((v) => selectOption("plan-version", v.id, t("execution.tests.versionN", { n: v.number }))),
);
const stepOptions = computed(() =>
  (version.value?.steps || []).map((s) =>
    selectOption("plan-step", s.position, t("execution.tests.stepLabel", { position: s.position, paradigm: paradigmName(s.paradigmKey), trials: s.trials })),
  ),
);

// Animals already in a live session of this slot.
const taken = computed(() => {
  const set = new Set();
  for (const session of props.sessions) {
    if (session.status === "CANCELLED") continue;
    if (session.timepointId !== form.value.timepointId || session.protocolVersionId !== form.value.versionId || session.stepPosition !== form.value.stepPosition) continue;
    for (const s of session.subjects) set.add(s.allocationId);
  }
  return set;
});
function animalOptions(row) {
  const chosen = new Set(form.value.subjects.filter((s) => s !== row).map((s) => s.allocationId));
  return props.allocations
    .filter((a) => !a.excluded && !taken.value.has(a.id) && !chosen.has(a.id))
    .map((a) => selectOption("plan-animal", a.id, a.subjectCode));
}
function arenaOptions(row) {
  if (!step.value) return [];
  const chosen = new Set(form.value.subjects.filter((s) => s !== row).map((s) => s.environmentRevisionId));
  return arenas
    .forParadigm(step.value.paradigmKey, step.value.paradigmVersion)
    .filter((r) => !chosen.has(r.id))
    .map((r) => selectOption("plan-arena", r.id, `${r.environmentName} · ${t("execution.tests.revisionN", { n: r.number })}`));
}
const noArenas = computed(() => step.value && !arenas.forParadigm(step.value.paradigmKey, step.value.paradigmVersion).length);

const valid = computed(
  () =>
    form.value.timepointId &&
    form.value.versionId &&
    form.value.stepPosition &&
    form.value.subjects.length > 0 &&
    form.value.subjects.every((s) => s.allocationId && s.environmentRevisionId),
);

async function save() {
  saving.value = true;
  error.value = "";
  try {
    const body = {
      timepointId: form.value.timepointId,
      protocolVersionId: form.value.versionId,
      stepPosition: form.value.stepPosition,
      subjects: form.value.subjects.map((s) => ({ allocationId: s.allocationId, environmentRevisionId: s.environmentRevisionId })),
    };
    if (form.value.scheduledAt) body.scheduledAt = new Date(form.value.scheduledAt).toISOString();
    if (form.value.notes.trim()) body.notes = form.value.notes.trim();
    const created = await tests.plan(props.experiment.id, body);
    message.success(t("execution.tests.planned"));
    emit("planned", created);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  try {
    await arenas.load();
    const list = (await studyDesign.protocols(props.experiment.id)).data || [];
    protocols.value = await Promise.all(
      list.map(async (protocol) => ({ protocol, versions: (await studyDesign.protocolVersions(props.experiment.id, protocol.id)).data || [] })),
    );
    if (protocols.value.length) form.value.protocolId = protocols.value[0].protocol.id;
  } catch (e) {
    error.value = e.message;
  }
});
</script>

<template>
  <NDrawer :show="true" :width="560" style="max-width: 100vw" placement="right" @update:show="(v) => !v && emit('close')">
    <NDrawerContent :title="$t('execution.tests.planTitle')" closable data-test="plan-drawer">
      <NForm label-placement="top" :show-feedback="false" class="form">
        <NFormItem :label="$t('execution.timepoint')">
          <NSelect v-model:value="form.timepointId" :options="timepointOptions" data-test="plan-timepoint" />
        </NFormItem>
        <NAlert v-if="!protocols.length && !error" type="info" :show-icon="false">{{ $t("execution.tests.noProtocols") }}</NAlert>
        <div class="row">
          <NFormItem :label="$t('execution.protocol')">
            <NSelect v-model:value="form.protocolId" :options="protocolOptions" data-test="plan-protocol" />
          </NFormItem>
          <NFormItem :label="$t('execution.tests.version')">
            <NSelect v-model:value="form.versionId" :options="versionOptions" data-test="plan-version" />
          </NFormItem>
        </div>
        <NFormItem :label="$t('execution.tests.step')">
          <NSelect v-model:value="form.stepPosition" :options="stepOptions" data-test="plan-step" />
        </NFormItem>
        <NFormItem :label="$t('execution.tests.scheduledAt')">
          <NDatePicker
            v-model:value="form.scheduledAt"
            type="datetime" :format="$t('execution.dateTimeFormat')"
            clearable
            :placeholder="$t('execution.tests.scheduledDefault')"
            data-test="plan-scheduled"
            @update:value="scheduleTouched = true"
          />
        </NFormItem>

        <div class="animals">
          <div class="animals-head">
            <strong>{{ $t("execution.tests.animalsInArenas") }}</strong>
            <span class="muted">{{ form.subjects.length }} / {{ MAX_ANIMALS }}</span>
          </div>
          <NAlert v-if="noArenas" type="warning" :show-icon="false" data-test="plan-no-arenas">
            {{ $t("execution.tests.noArenas", { paradigm: paradigmName(step.paradigmKey) }) }}
          </NAlert>
          <div v-for="(row, index) in form.subjects" :key="index" class="animal-row" data-test="plan-animal-row">
            <NSelect
              v-model:value="row.allocationId"
              filterable
              :filter="filterByText"
              :placeholder="$t('execution.animal')"
              :options="animalOptions(row)"
              :data-test="`plan-animal-${index}`"
            />
            <NSelect v-model:value="row.environmentRevisionId" :placeholder="$t('execution.arena')" :options="arenaOptions(row)" :data-test="`plan-arena-${index}`" />
            <NButton quaternary size="small" :disabled="form.subjects.length === 1" :aria-label="$t('execution.tests.removeRow')" @click="form.subjects.splice(index, 1)">
              ×
            </NButton>
          </div>
          <NButton
            dashed
            size="small"
            :disabled="form.subjects.length >= MAX_ANIMALS"
            data-test="plan-add-animal"
            @click="form.subjects.push({ allocationId: null, environmentRevisionId: null })"
          >
            + {{ $t("execution.tests.addAnimal") }}
          </NButton>
        </div>

        <NFormItem :label="$t('execution.notes')">
          <NInput v-model:value="form.notes" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" maxlength="2000" />
        </NFormItem>
        <NAlert v-if="error" type="error" :title="error" data-test="plan-error" />
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="emit('close')">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :disabled="!valid" :loading="saving" data-test="plan-save" @click="save">{{ $t("execution.tests.plan") }}</NButton>
        </NSpace>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.form { display: flex; flex-direction: column; gap: 12px; }
.row { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; }
.animals { display: flex; flex-direction: column; gap: 8px; padding: 10px; border: 1px solid var(--line); border-radius: 10px; }
.animals-head { display: flex; justify-content: space-between; }
.animal-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) auto; gap: 8px; align-items: center; }
</style>
