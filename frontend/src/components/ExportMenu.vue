<script setup>
import { useI18n } from "vue-i18n";
import { NDropdown, NButton } from "naive-ui";

const props = defineProps({
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(["csv", "pdf"]);

const { t } = useI18n();

const options = [
  { label: () => t("datatable.exportCsv"), key: "csv" },
  { label: () => t("datatable.exportPdf"), key: "pdf" },
];

function onSelect(key) {
  emit(key);
}
</script>

<template>
  <NDropdown trigger="click" :options="options" :disabled="props.disabled" @select="onSelect">
    <NButton :disabled="props.disabled" size="small" quaternary>
      {{ $t("datatable.export") }}
      <template #icon>
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12M7 10l5 5 5-5" /><path d="M5 21h14" /></svg>
      </template>
    </NButton>
  </NDropdown>
</template>
