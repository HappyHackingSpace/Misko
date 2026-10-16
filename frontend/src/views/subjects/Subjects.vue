<script setup>
// Laboratory animals with the group each one is allocated to. Anyone signed in
// may read them; creating, editing and deleting needs subject:write.
import { computed, h, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NButton, NCard, NCheckbox, NDataTable, NPopconfirm, NSelect, NSpace, NTag, useMessage } from "naive-ui";
import { subjects } from "../../api/subjects.js";
import { useDataTable } from "../../composables/useDataTable.js";
import { useTableExport } from "../../composables/useTableExport.js";
import { useAuth } from "../../stores/auth.js";
import PageHead from "../../components/PageHead.vue";
import ListToolbar from "../../components/ListToolbar.vue";
import SubjectDrawer from "./SubjectDrawer.vue";
import { ROLE_TAG, SEXES, SPECIES, useSubjectLabels } from "./useSubjectLabels.js";

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const labels = useSubjectLabels();
const canWrite = computed(() => auth.can("subject:write"));

const filters = ref({ species: null, sex: null, unallocated: false });
const table = useDataTable("/subjects", {
  defaultSort: { field: "createdAt", order: "desc" },
  extraParams: () => ({ species: filters.value.species, sex: filters.value.sex, unallocated: filters.value.unallocated ? "true" : "" }),
});
const filtered = computed(() => !!(table.state.search || filters.value.species || filters.value.sex || filters.value.unallocated));
const setFilter = (key, value) => {
  filters.value[key] = value;
  table.setPage(1);
};

const speciesOptions = computed(() => SPECIES.map((value) => ({ value, label: labels.species(value) })));
const sexOptions = computed(() => SEXES.map((value) => ({ value, label: labels.sex(value) })));

function sortOrderFor(key) {
  if (table.state.sort.field !== key) return false;
  return table.state.sort.order === "asc" ? "ascend" : "descend";
}

function onUpdateSorter(sorter) {
  if (!sorter || !sorter.order) table.setSort({ field: "createdAt", order: "desc" });
  else table.setSort({ field: sorter.columnKey, order: sorter.order === "ascend" ? "asc" : "desc" });
}

// "EXP-01 / G2 Treatment", or "Unallocated".
function allocationCell(row) {
  const a = row.allocation;
  if (!a) return h("span", { class: "muted", "data-test": "subject-unallocated" }, t("subjects.unallocated"));
  return h("div", { class: "alloc", "data-test": "subject-allocation" }, [
    h(RouterLink, { class: "link", to: `/experiments/${a.experimentId}` }, () => a.experimentCode),
    h("span", { class: "muted" }, a.groupCode),
    h(NTag, { size: "small", round: true, bordered: false, type: ROLE_TAG[a.groupRole] }, () => labels.role(a.groupRole)),
    a.excluded ? h(NTag, { size: "small", round: true, type: "error", "data-test": "subject-excluded" }, () => t("subjects.excluded")) : null,
  ]);
}

const deleteError = ref("");
async function remove(row) {
  deleteError.value = "";
  try {
    await subjects.remove(row.id);
    message.success(t("subjects.deleted", { code: row.code }));
    table.reload();
  } catch (e) {
    deleteError.value = e.message;
  }
}

const columns = computed(() => {
  const cols = [
    {
      title: t("subjects.code"),
      key: "code",
      sorter: true,
      sortOrder: sortOrderFor("code"),
      render: (row) => h(RouterLink, { class: "link", to: `/subjects/${row.id}`, "data-test": `subject-open-${row.code}` }, () => row.code),
    },
    {
      title: t("subjects.species"),
      key: "species",
      width: 110,
      sorter: true,
      sortOrder: sortOrderFor("species"),
      render: (row) => h(NTag, { size: "small", round: true, bordered: false, type: row.species === "MOUSE" ? "info" : "warning" }, () => labels.species(row.species)),
    },
    {
      title: t("subjects.sex"),
      key: "sex",
      width: 110,
      sorter: true,
      sortOrder: sortOrderFor("sex"),
      render: (row) => h("span", { "data-sex": row.sex }, labels.sex(row.sex)),
    },
    { title: t("subjects.allocation"), key: "allocation", render: allocationCell },
    {
      title: t("subjects.createdAt"),
      key: "createdAt",
      width: 150,
      sorter: true,
      sortOrder: sortOrderFor("createdAt"),
      render: (row) => h("span", { class: "muted" }, labels.date(row.createdAt)),
    },
  ];
  if (canWrite.value) {
    cols.push({
      title: "",
      key: "actions",
      width: 150,
      render: (row) =>
        h(NSpace, { size: 6, wrap: false, justify: "end" }, () => [
          h(NButton, { size: "tiny", secondary: true, "data-test": `subject-edit-${row.code}`, onClick: () => openEdit(row) }, () => t("common.edit")),
          h(
            NPopconfirm,
            { onPositiveClick: () => remove(row), positiveText: t("common.delete"), negativeText: t("common.cancel") },
            {
              trigger: () => h(NButton, { size: "tiny", secondary: true, type: "error", "data-test": `subject-delete-${row.code}` }, () => t("common.delete")),
              default: () => t("subjects.confirmDelete", { code: row.code }),
            },
          ),
        ]),
    });
  }
  return cols;
});

const drawer = ref(false);
const editing = ref(null);
function openNew() {
  editing.value = null;
  drawer.value = true;
}
function openEdit(row) {
  editing.value = row;
  drawer.value = true;
}

const allocationText = (row) => {
  const a = row.allocation;
  if (!a) return t("subjects.unallocated");
  return `${a.experimentCode} ${a.groupCode} ${labels.role(a.groupRole)}${a.excluded ? ` (${t("subjects.excluded")})` : ""}`;
};
const { exportCsv, exportPdf } = useTableExport({
  state: table.state,
  columns: [
    { key: "code", label: t("subjects.code") },
    { key: "species", label: t("subjects.species"), value: (row) => labels.species(row.species) },
    { key: "sex", label: t("subjects.sex"), value: (row) => labels.sex(row.sex) },
    { key: "allocation", label: t("subjects.allocation"), value: allocationText },
  ],
  exportName: "subjects",
  entityLabel: t("subjects.title"),
});
</script>

<template>
  <PageHead :title="$t('subjects.title')" :subtitle="$t('subjects.subtitle')" :count="table.state.total">
    <template #actions>
      <NButton v-if="canWrite" type="primary" data-test="subject-new" @click="openNew">{{ $t("subjects.new") }}</NButton>
    </template>
  </PageHead>

  <NCard :bordered="true" size="small">
    <template #header>
      <ListToolbar :search="table.state.search" :export-disabled="!table.state.rows.length" @update:search="table.setSearch" @csv="exportCsv" @pdf="exportPdf" />
      <div class="filters">
        <NSelect :value="filters.species" class="filter" clearable data-test="subject-filter-species" :options="speciesOptions" :placeholder="$t('subjects.allSpecies')" @update:value="setFilter('species', $event)" />
        <NSelect :value="filters.sex" class="filter" clearable data-test="subject-filter-sex" :options="sexOptions" :placeholder="$t('subjects.allSexes')" @update:value="setFilter('sex', $event)" />
        <NCheckbox :checked="filters.unallocated" data-test="subject-filter-unallocated" @update:checked="setFilter('unallocated', $event)">
          {{ $t("subjects.onlyUnallocated") }}
        </NCheckbox>
      </div>
    </template>
    <NAlert v-if="table.state.error || deleteError" type="error" :title="table.state.error || deleteError" class="alert" data-test="subject-list-error" closable @close="deleteError = ''" />
    <NDataTable
      remote
      :columns="columns"
      :data="table.state.rows"
      :loading="table.state.loading"
      :row-key="(row) => row.id"
      :row-props="(row) => ({ 'data-test': 'subject-row', 'data-code': row.code })"
      :scroll-x="820"
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
    >
      <template #empty>
        <span class="muted">{{ filtered ? $t("subjects.noMatch") : $t("subjects.empty") }}</span>
      </template>
    </NDataTable>
  </NCard>

  <SubjectDrawer v-model:show="drawer" :subject="editing" @saved="table.reload()" />
</template>

<style scoped>
.filters { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 10px; }
.filter { width: 170px; }
.alert { margin-bottom: 12px; }
:deep(.alloc) { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
@media (max-width: 520px) {
  .filter { width: 100%; }
}
</style>
