<script setup>
// Laboratory animals. Anyone signed in may read them; creating and editing
// needs subject:write, so the controls appear only for a role that has it.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";
import { useAuth } from "../stores/auth.js";

const { t } = useI18n();
const auth = useAuth();
const canWrite = computed(() => auth.can("subject:write"));
const table = useDataTable("/subjects", { defaultSort: { field: "createdAt", order: "desc" } });

const sexLabel = (sex) => t(`subjects.sex${sex.charAt(0) + sex.slice(1).toLowerCase()}`);

const columns = computed(() => [
  { key: "code", label: t("subjects.code"), sortable: true },
  { key: "species", label: t("subjects.species"), sortable: true },
  { key: "sex", label: t("subjects.sex"), sortable: true, exportValue: (row) => sexLabel(row.sex) },
  { key: "strain", label: t("subjects.strain"), cellClass: "muted" },
  { key: "notes", label: t("common.notes"), cellClass: "muted" },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("subjects.title") }}</h1>
    <button v-if="canWrite" class="primary" data-test="subject-new" @click="$router.push('/subjects/new')">
      {{ $t("subjects.new") }}
    </button>
  </div>

  <div class="card">
    <DataTable
      :columns="columns"
      :rows="table.state.rows"
      :total="table.state.total"
      :page="table.state.page"
      :page-size="table.state.pageSize"
      :sort="table.state.sort"
      :search="table.state.search"
      :loading="table.state.loading"
      export-name="subjects"
      :entity-label="$t('subjects.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <!-- The label is translated, so the raw value travels in an attribute for
           anything that needs to read it without knowing the language. -->
      <template #cell-sex="{ row }">
        <span :data-sex="row.sex">{{ sexLabel(row.sex) }}</span>
      </template>
      <template v-if="canWrite" #actions="{ row }">
        <RouterLink class="link" :to="`/subjects/${row.id}`" :data-test="`subject-edit-${row.code}`">
          {{ $t("common.edit") }}
        </RouterLink>
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
</style>
