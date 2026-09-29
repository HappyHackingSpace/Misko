<script setup>
import { NInput } from "naive-ui";
import ExportMenu from "./ExportMenu.vue";

defineProps({
  search: { type: String, default: "" },
  searchable: { type: Boolean, default: true },
  exportable: { type: Boolean, default: true },
  exportDisabled: { type: Boolean, default: false },
});
defineEmits(["update:search", "csv", "pdf"]);
</script>

<template>
  <div class="list-toolbar">
    <NInput
      v-if="searchable"
      class="search"
      data-test="list-search"
      :value="search"
      clearable
      :placeholder="$t('datatable.searchPlaceholder')"
      @update:value="$emit('update:search', $event)"
    >
      <template #prefix>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" /></svg>
      </template>
    </NInput>
    <span v-else class="search-spacer"></span>
    <div class="list-toolbar-spacer"></div>
    <ExportMenu v-if="exportable" :disabled="exportDisabled" @csv="$emit('csv')" @pdf="$emit('pdf')" />
  </div>
</template>

<style scoped>
.list-toolbar { display: flex; align-items: center; gap: 12px; }
.search, .search-spacer { max-width: 320px; width: 100%; }
.list-toolbar-spacer { flex: 1; }
@media (max-width: 520px) {
  .list-toolbar { flex-wrap: wrap; }
  .search, .search-spacer { max-width: none; }
}
</style>
