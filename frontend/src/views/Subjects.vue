<script setup>
// Laboratory animals. Anyone signed in may read them; creating and editing
// needs subject:write, so the controls appear only for a role that has it.
import { computed, h } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NButton, NCard, NDataTable, NTag } from "naive-ui";
import { useDataTable } from "../composables/useDataTable.js";
import { useTableExport } from "../composables/useTableExport.js";
import { useAuth } from "../stores/auth.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t } = useI18n();
const auth = useAuth();
const canWrite = computed(() => auth.can("subject:write"));
const table = useDataTable("/subjects", { defaultSort: { field: "createdAt", order: "desc" } });

const sexLabel = (sex) => t(`subjects.sex${sex.charAt(0) + sex.slice(1).toLowerCase()}`);
const speciesLabel = (species) => t(`subjects.species${species.charAt(0) + species.slice(1).toLowerCase()}`);

function sortOrderFor(key) {
  if (table.state.sort.field !== key) return false;
  return table.state.sort.order === "asc" ? "ascend" : "descend";
}

function onUpdateSorter(sorter) {
  if (!sorter || !sorter.order) return;
  table.setSort({ field: sorter.columnKey, order: sorter.order === "ascend" ? "asc" : "desc" });
}

const columns = computed(() => {
  const cols = [
    {
      title: "#",
      key: "__index",
      width: 48,
      render: (_row, index) => (table.state.page - 1) * table.state.pageSize + index + 1,
    },
    { title: t("subjects.code"), key: "code", sorter: true, sortOrder: sortOrderFor("code") },
    {
      title: t("subjects.species"),
      key: "species",
      sorter: true,
      sortOrder: sortOrderFor("species"),
      render: (row) =>
        h(NTag, { size: "small", round: true, bordered: false, type: row.species === "MOUSE" ? "info" : "warning" }, () => speciesLabel(row.species)),
    },
    {
      title: t("subjects.sex"),
      key: "sex",
      sorter: true,
      sortOrder: sortOrderFor("sex"),
      render: (row) =>
        h(
          NTag,
          {
            size: "small",
            round: true,
            bordered: false,
            "data-sex": row.sex,
            type: row.sex === "FEMALE" ? "error" : row.sex === "MALE" ? "info" : "default",
          },
          () => sexLabel(row.sex),
        ),
    },
    { title: t("subjects.strain"), key: "strain", render: (row) => row.strain || "—" },
    { title: t("common.notes"), key: "notes", ellipsis: { tooltip: true }, render: (row) => row.notes || "—" },
  ];
  if (canWrite.value) {
    cols.push({
      title: "",
      key: "actions",
      width: 90,
      render: (row) => h(RouterLink, { to: `/subjects/${row.id}`, class: "link", "data-test": `subject-edit-${row.code}` }, () => t("common.edit")),
    });
  }
  return cols;
});

const exportColumns = [
  { key: "code", label: t("subjects.code") },
  { key: "species", label: t("subjects.species"), value: (row) => speciesLabel(row.species) },
  { key: "sex", label: t("subjects.sex"), value: (row) => sexLabel(row.sex) },
  { key: "strain", label: t("subjects.strain") },
  { key: "notes", label: t("common.notes") },
];
const { exportCsv, exportPdf } = useTableExport({
  state: table.state,
  columns: exportColumns,
  exportName: "subjects",
  entityLabel: t("subjects.title"),
});
</script>

<template>
  <PageHead :title="$t('subjects.title')" :subtitle="$t('subjects.subtitle')" :count="table.state.total">
    <template #actions>
      <NButton v-if="canWrite" type="primary" data-test="subject-new" @click="$router.push('/subjects/new')">
        <template #icon>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
        </template>
        {{ $t("subjects.new") }}
      </NButton>
    </template>
  </PageHead>

  <NCard :bordered="true" size="small">
    <template #header>
      <ListToolbar :search="table.state.search" :export-disabled="!table.state.rows.length" @update:search="table.setSearch" @csv="exportCsv" @pdf="exportPdf" />
    </template>
    <NDataTable
      remote
      :columns="columns"
      :data="table.state.rows"
      :loading="table.state.loading"
      :row-key="(row) => row.id"
      :pagination="{
        page: table.state.page,
        pageSize: table.state.pageSize,
        itemCount: table.state.total,
        pageSizes: [10, 20, 50],
        showSizePicker: true,
        onUpdatePage: table.setPage,
        onUpdatePageSize: table.setPageSize,
      }"
      @update:sorter="onUpdateSorter"
    />
  </NCard>
</template>
