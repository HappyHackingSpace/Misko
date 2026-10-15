<script setup>
// Every dose given in the study, newest first, one page at a time, with the
// amount actually given and the weight it was computed from.
import { computed, h, inject, reactive, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NCard, NDataTable, NSelect, NTag } from "naive-ui";
import { dosing } from "../../api/dosing.js";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";
import { filterByText, selectOption } from "../../composables/selectOptions.js";

const props = defineProps({
  allocations: { type: Array, default: () => [] },
  groups: { type: Array, default: () => [] },
});
const { t } = useI18n();
const { id } = inject("study");
const fmt = useExecutionFormat();

const state = reactive({ rows: [], total: 0, page: 1, pageSize: 10, loading: false, error: "", allocationId: null, purpose: null });

let seq = 0;
async function load() {
  const mine = ++seq;
  state.loading = true;
  state.error = "";
  try {
    const page = await dosing.history(id.value, {
      page: state.page,
      pageSize: state.pageSize,
      allocationId: state.allocationId,
      purpose: state.purpose,
    });
    if (mine !== seq) return;
    state.rows = page.data || [];
    state.total = page.total || 0;
  } catch (e) {
    if (mine === seq) state.error = e.message;
  } finally {
    if (mine === seq) state.loading = false;
  }
}
watch(
  () => [state.allocationId, state.purpose],
  () => {
    state.page = 1;
    load();
  },
);
load();

const animalCode = computed(() => Object.fromEntries(props.allocations.map((a) => [a.id, a.subjectCode])));
const groupCode = computed(() => Object.fromEntries(props.groups.map((g) => [g.id, g.code])));
const animalOptions = computed(() => props.allocations.map((a) => selectOption("history-animal", a.id, a.subjectCode)));
const purposeOptions = computed(() => ["INDUCTION", "VEHICLE", "TREATMENT"].map((p) => selectOption("history-purpose", p, fmt.purpose(p))));

const columns = computed(() => [
  { title: t("execution.dosing.givenAt"), key: "administeredAt", width: 170, render: (row) => fmt.dateTime(row.administeredAt) },
  {
    title: t("execution.animal"),
    key: "allocationId",
    render: (row) => h("span", { "data-test": "history-animal" }, `${animalCode.value[row.allocationId] || ""} (${groupCode.value[row.groupId] || ""})`),
  },
  { title: t("execution.dosing.substance"), key: "substanceName" },
  { title: t("execution.dosing.purpose"), key: "purpose", render: (row) => fmt.purpose(row.purpose) },
  { title: t("execution.dosing.dose"), key: "dose", render: (row) => h("span", { "data-test": "history-dose" }, fmt.dose(row.dose)) },
  { title: t("execution.dosing.given"), key: "absoluteDose", render: (row) => fmt.dose(row.absoluteDose) },
  { title: t("execution.dosing.weight"), key: "bodyWeightGrams", render: (row) => fmt.grams(row.bodyWeightGrams) },
  { title: t("execution.dosing.route"), key: "route", render: (row) => fmt.route(row.route) },
  {
    title: t("execution.dosing.plan"),
    key: "regimenItemId",
    render: (row) =>
      h(NTag, { size: "small", round: true, bordered: false, type: row.regimenItemId ? "default" : "warning" }, () =>
        row.regimenItemId ? t("execution.dosing.planned") : t("execution.dosing.unplannedTag"),
      ),
  },
  { title: t("execution.notes"), key: "notes", ellipsis: { tooltip: true } },
]);
</script>

<template>
  <NCard size="small" :bordered="true" :title="$t('execution.dosing.historyTitle')" data-test="dose-history">
    <template #header-extra>
      <div class="filters">
        <NSelect
          v-model:value="state.allocationId"
          size="small"
          clearable
          filterable
          :filter="filterByText"
          :placeholder="$t('execution.allAnimals')"
          :options="animalOptions"
          data-test="history-filter-animal"
        />
        <NSelect v-model:value="state.purpose" size="small" clearable :placeholder="$t('execution.dosing.allPurposes')" :options="purposeOptions" />
      </div>
    </template>
    <p v-if="state.error" class="err">{{ state.error }}</p>
    <NDataTable
      remote
      size="small"
      :columns="columns"
      :data="state.rows"
      :loading="state.loading"
      :row-key="(row) => row.id"
      :scroll-x="1000"
      :pagination="{
        page: state.page,
        pageSize: state.pageSize,
        itemCount: state.total,
        pageSizes: [10, 25, 50],
        showSizePicker: true,
        onUpdatePage: (p) => ((state.page = p), load()),
        onUpdatePageSize: (s) => ((state.pageSize = s), (state.page = 1), load()),
      }"
    >
      <template #empty>
        <p class="muted">{{ $t("execution.dosing.noHistory") }}</p>
      </template>
    </NDataTable>
  </NCard>
</template>

<style scoped>
.filters { display: flex; gap: 8px; flex-wrap: wrap; }
.filters :deep(.n-select) { width: 160px; }
</style>
