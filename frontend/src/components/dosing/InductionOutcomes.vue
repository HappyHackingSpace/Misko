<script setup>
// Whether each induced animal developed its disease model. Only groups that
// are induced are listed; an outcome is recorded once and never changed.
import { computed, h, inject, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDataTable, NDatePicker, NForm, NFormItem, NInput, NModal, NRadioButton, NRadioGroup, NSpace, NTag, useMessage } from "naive-ui";
import { dosing } from "../../api/dosing.js";
import { tagType, useExecutionFormat } from "../../composables/useExecutionFormat.js";

const props = defineProps({
  groups: { type: Array, default: () => [] },
  allocations: { type: Array, default: () => [] },
  canRecord: { type: Boolean, default: false },
});
const emit = defineEmits(["recorded"]);
const { t } = useI18n();
const { id } = inject("study");
const message = useMessage();
const fmt = useExecutionFormat();

const INDUCED = ["DISEASE_CONTROL", "POSITIVE_CONTROL", "TREATMENT"];
const outcomes = ref([]);
const error = ref("");
const loading = ref(false);
const editing = ref(null);
const form = ref({ status: "CONFIRMED", assessedAt: Date.now(), note: "" });
const saving = ref(false);
const formError = ref("");

const induced = computed(() => props.groups.filter((g) => INDUCED.includes(g.role)));
const rows = computed(() => {
  const byAllocation = Object.fromEntries(outcomes.value.map((o) => [o.allocationId, o]));
  const groupIds = new Set(induced.value.map((g) => g.id));
  const groupCode = Object.fromEntries(props.groups.map((g) => [g.id, g.code]));
  return props.allocations
    .filter((a) => groupIds.has(a.groupId))
    .map((a) => ({ ...a, groupCode: groupCode[a.groupId], outcome: byAllocation[a.id] || null }));
});

async function load() {
  loading.value = true;
  error.value = "";
  try {
    outcomes.value = (await dosing.inductionOutcomes(id.value)).data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

function open(row) {
  editing.value = row;
  form.value = { status: "CONFIRMED", assessedAt: Date.now(), note: "" };
  formError.value = "";
}

async function save() {
  saving.value = true;
  formError.value = "";
  try {
    const body = { status: form.value.status, assessedAt: new Date(form.value.assessedAt).toISOString() };
    if (form.value.note.trim()) body.note = form.value.note.trim();
    await dosing.recordInductionOutcome(id.value, editing.value.id, body);
    message.success(t("execution.induction.recorded"));
    editing.value = null;
    emit("recorded");
  } catch (e) {
    formError.value = e.message;
  } finally {
    saving.value = false;
  }
}

const columns = computed(() => [
  { title: t("execution.animal"), key: "subjectCode", render: (row) => h("strong", row.subjectCode) },
  { title: t("execution.group"), key: "groupCode" },
  {
    title: t("execution.induction.outcome"),
    key: "outcome",
    render: (row) =>
      h(
        NTag,
        { size: "small", round: true, bordered: false, type: row.outcome ? tagType(row.outcome.status) : "default", "data-test": "induction-status" },
        () => (row.outcome ? fmt.status(row.outcome.status) : t("execution.induction.notAssessed")),
      ),
  },
  { title: t("execution.induction.assessedAt"), key: "assessedAt", render: (row) => fmt.dateTime(row.outcome?.assessedAt) },
  { title: t("execution.notes"), key: "note", ellipsis: { tooltip: true }, render: (row) => row.outcome?.note || "" },
  {
    title: "",
    key: "actions",
    align: "right",
    render: (row) =>
      props.canRecord && !row.outcome && !row.excluded
        ? h(NButton, { size: "tiny", secondary: true, "data-test": "record-induction", onClick: () => open(row) }, () => t("execution.induction.record"))
        : null,
  },
]);

onMounted(load);
</script>

<template>
  <NCard v-if="induced.length" size="small" :bordered="true" :title="$t('execution.induction.title')" data-test="induction-outcomes">
    <NAlert v-if="error" type="error" :title="error" />
    <NDataTable size="small" :columns="columns" :data="rows" :loading="loading" :row-key="(row) => row.id" :scroll-x="640" />

    <NModal
      :show="!!editing"
      preset="card"
      :title="$t('execution.induction.recordTitle', { animal: editing?.subjectCode || '' })"
      style="width: 440px; max-width: 94vw"
      @update:show="(v) => !v && (editing = null)"
    >
      <NForm label-placement="top" :show-feedback="false" class="form">
        <NFormItem :label="$t('execution.induction.outcome')">
          <NRadioGroup v-model:value="form.status" data-test="induction-choice">
            <NRadioButton value="CONFIRMED">{{ fmt.status("CONFIRMED") }}</NRadioButton>
            <NRadioButton value="NOT_CONFIRMED">{{ fmt.status("NOT_CONFIRMED") }}</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem :label="$t('execution.induction.assessedAt')">
          <NDatePicker v-model:value="form.assessedAt" type="datetime" :format="$t('execution.dateTimeFormat')" :clearable="false" />
        </NFormItem>
        <NFormItem :label="$t('execution.notes')">
          <NInput v-model:value="form.note" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" maxlength="2000" />
        </NFormItem>
        <p class="muted">{{ $t("execution.induction.once") }}</p>
        <NAlert v-if="formError" type="error" :title="formError" />
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="editing = null">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" data-test="induction-save" @click="save">{{ $t("common.save") }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </NCard>
</template>

<style scoped>
.form { display: flex; flex-direction: column; gap: 12px; }
.form p { margin: 0; }
</style>
