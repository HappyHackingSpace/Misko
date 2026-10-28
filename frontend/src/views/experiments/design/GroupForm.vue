<script setup>
// Create or edit one group: its code, name, role, disease model and target
// size. A healthy control has no disease model; every other role needs one.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NInput, NInputNumber, NModal, NSelect } from "naive-ui";
import { groups as groupsApi } from "../../../api/experiments.js";
import { filterByText, selectOption } from "../../../composables/selectOptions.js";
import { ROLES } from "./shared.js";

const props = defineProps({
  show: { type: Boolean, default: false },
  experimentId: { type: String, required: true },
  group: { type: Object, default: null },
  diseaseModels: { type: Array, default: () => [] },
  // Code suggested for a new group (G1, G2 ...).
  nextCode: { type: String, default: "" },
  // Role chosen up front for a new group, such as a healthy control the
  // readiness panel asks for.
  role: { type: String, default: null },
  // Disease model chosen up front, such as the one a disease control must
  // share with the group the readiness panel compares it to.
  diseaseModelId: { type: String, default: null },
});
const emit = defineEmits(["update:show", "saved"]);
const { t } = useI18n();

const empty = () => ({ code: "", name: "", role: null, diseaseModelId: null, targetSize: null, description: "" });
const form = ref(empty());
const error = ref("");
const saving = ref(false);

watch(
  () => props.show,
  (open) => {
    if (!open) return;
    error.value = "";
    const g = props.group;
    form.value = g
      ? {
          code: g.code,
          name: g.name,
          role: g.role,
          diseaseModelId: g.diseaseModelId,
          targetSize: g.targetSize,
          description: g.description || "",
        }
      : {
          ...empty(),
          code: props.nextCode,
          role: props.role,
          name: props.role ? t(`design.roles.${props.role}`) : "",
          diseaseModelId: props.diseaseModelId,
        };
  },
);

const healthy = computed(() => form.value.role === "HEALTHY_CONTROL");
watch(healthy, (isHealthy) => {
  if (isHealthy) form.value.diseaseModelId = null;
});

const roleOptions = computed(() => ROLES.map((r) => selectOption("group-role", r, t(`design.roles.${r}`))));
const diseaseOptions = computed(() => props.diseaseModels.map((m) => selectOption("group-disease", m.id, m.name)));

function payload() {
  const f = form.value;
  const body = {
    code: f.code.trim(),
    name: f.name.trim(),
    role: f.role,
    targetSize: f.targetSize || 0,
    description: f.description.trim(),
  };
  if (!props.group) {
    if (f.diseaseModelId && !healthy.value) body.diseaseModelId = f.diseaseModelId;
    if (!body.description) delete body.description;
    return body;
  }
  // An edit sends the disease model explicitly so a role change can clear it.
  body.diseaseModelId = healthy.value ? "" : f.diseaseModelId || "";
  return body;
}

async function save() {
  error.value = "";
  saving.value = true;
  try {
    const saved = props.group
      ? await groupsApi.update(props.experimentId, props.group.id, payload())
      : await groupsApi.create(props.experimentId, payload());
    emit("saved", saved);
    emit("update:show", false);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="group ? $t('design.groups.edit') : $t('design.groups.add')"
    style="width: 560px; max-width: 94vw"
    @update:show="emit('update:show', $event)"
  >
    <form class="form" data-test="group-form" @submit.prevent="save">
      <label class="fld">
        <span>{{ $t("design.groups.code") }}</span>
        <NInput v-model:value="form.code" :maxlength="16" :input-props="{ 'data-test': 'group-code', required: true }" />
      </label>
      <label class="fld">
        <span>{{ $t("design.groups.name") }}</span>
        <NInput v-model:value="form.name" :maxlength="120" :input-props="{ 'data-test': 'group-name', required: true }" />
      </label>
      <div class="fld wide">
        <span>{{ $t("design.groups.role") }}</span>
        <NSelect v-model:value="form.role" :options="roleOptions" data-test="group-role" />
        <small v-if="form.role" class="muted">{{ $t(`design.roleHints.${form.role}`) }}</small>
      </div>
      <div class="fld wide" v-if="!healthy">
        <span>{{ $t("design.groups.diseaseModel") }}</span>
        <NSelect
          v-model:value="form.diseaseModelId"
          :options="diseaseOptions"
          filterable
          :filter="filterByText"
          :placeholder="$t('design.groups.pickDiseaseModel')"
          data-test="group-disease"
        />
        <small v-if="!diseaseModels.length" class="muted">
          {{ $t("design.groups.noDiseaseModels") }}
          <RouterLink to="/disease-models" data-test="group-disease-models-link">{{ $t("design.groups.manageDiseaseModels") }}</RouterLink>
        </small>
      </div>
      <label class="fld">
        <span>{{ $t("design.groups.targetSize") }}</span>
        <NInputNumber v-model:value="form.targetSize" :min="0" :max="100000" :precision="0" clearable :input-props="{ 'data-test': 'group-target' }" />
      </label>
      <label class="fld wide">
        <span>{{ $t("common.description") }}</span>
        <NInput v-model:value="form.description" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" :maxlength="5000" />
      </label>
      <NAlert v-if="error" class="wide" type="error" :title="error" data-test="group-error" />
    </form>
    <template #footer>
      <div class="dialog-actions">
        <NButton @click="emit('update:show', false)">{{ $t("common.cancel") }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!form.code || !form.name || !form.role" data-test="group-save" @click="save">
          {{ $t("common.save") }}
        </NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.fld > span { font-size: 12px; color: var(--muted); }
.wide { grid-column: 1 / -1; }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
