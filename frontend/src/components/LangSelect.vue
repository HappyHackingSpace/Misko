<script setup>
import { usePrefs } from "../stores/prefs.js";

const prefs = usePrefs();
</script>

<template>
  <select
    class="lang-select"
    :value="prefs.locale"
    :title="$t('prefs.language')"
    :aria-label="$t('prefs.language')"
    @change="prefs.setLocale($event.target.value)"
  >
    <!-- Short code (TR, EN); full name kept as the option title for clarity -->
    <option v-for="l in prefs.locales" :key="l.code" :value="l.code" :title="l.name">
      {{ l.code.toUpperCase() }}
    </option>
  </select>
</template>
