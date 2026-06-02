<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t, locale } = useI18n();

const table = useDataTable("/users", { defaultSort: { field: "createdAt", order: "desc" } });

const columns = computed(() => [
  { key: "name", label: t("common.name"), sortable: true },
  { key: "email", label: t("users.email"), sortable: true, cellClass: "muted" },
  {
    key: "role", label: t("users.role"), sortable: true,
    exportValue: (row) => t(`roles.${row.role}`),
  },
  {
    key: "createdAt", label: t("common.addedAt"), sortable: true, cellClass: "muted",
    exportValue: (row) => new Date(row.createdAt).toLocaleDateString(locale.value),
  },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("users.title") }}</h1>
    <button class="primary" @click="$router.push('/users/new')">{{ $t("common.create") }}</button>
  </div>
  <p class="muted" style="margin-top:0">{{ $t("users.intro") }}</p>

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
      export-name="users"
      :entity-label="$t('users.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-name="{ row }"><RouterLink class="link" :to="`/users/${row.id}`">{{ row.name }}</RouterLink></template>
      <template #cell-role="{ row }">{{ $t(`roles.${row.role}`) }}</template>
      <template #cell-createdAt="{ row }">{{ new Date(row.createdAt).toLocaleDateString(locale) }}</template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
