<script setup>
import { NButton } from "naive-ui";
import { usePrefs } from "../stores/prefs.js";

const prefs = usePrefs();
</script>

<template>
  <NButton
    quaternary
    circle
    :title="$t('prefs.theme')"
    :aria-label="$t('prefs.theme')"
    @click="prefs.toggleTheme()"
  >
    <template #icon>
      <!-- Show the icon of the mode you'll switch to -->
      <svg v-if="prefs.theme === 'dark'" class="ico" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5L19 19M19 5l-1.5 1.5M6.5 17.5L5 19" />
      </svg>
      <svg v-else class="ico" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z" />
      </svg>
    </template>
  </NButton>
</template>

<style scoped>
.ico {
  width: 17px;
  height: 17px;
  display: block;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
/* Moon reads better filled */
.ico path[d^="M21"] {
  fill: currentColor;
  stroke: none;
}
</style>
