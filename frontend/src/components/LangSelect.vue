<script setup>
import { computed } from "vue";
import { NButton, NDropdown } from "naive-ui";
import { usePrefs } from "../stores/prefs.js";

const prefs = usePrefs();

const options = computed(() =>
  prefs.locales.map((l) => ({ label: l.name, key: l.code })),
);

function onSelect(code) {
  prefs.setLocale(code);
}
</script>

<template>
  <NDropdown trigger="click" :options="options" @select="onSelect">
    <NButton quaternary size="small" class="lang-trigger" :title="$t('prefs.language')" :aria-label="$t('prefs.language')">
      {{ prefs.locale.toUpperCase() }}
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" class="chev"><polyline points="6 9 12 15 18 9" /></svg>
    </NButton>
  </NDropdown>
</template>

<style scoped>
.lang-trigger { font-weight: 700; letter-spacing: 0.02em; display: inline-flex; align-items: center; gap: 4px; }
.chev { opacity: 0.7; }
</style>
