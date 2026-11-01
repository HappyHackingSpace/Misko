<script setup>
// Inlined rather than <img> so the ink follows the theme through currentColor.
import logoSvg from "../assets/misko-logo.svg?raw";
import markSvg from "../assets/misko-mark.svg?raw";

defineProps({ mark: { type: Boolean, default: false } });
</script>

<template>
  <span class="misko-logo" v-html="mark ? markSvg : logoSvg"></span>
</template>

<style scoped>
.misko-logo { display: inline-flex; color: var(--txt); line-height: 0; }
.misko-logo :deep(svg) { width: auto; height: 100%; }
</style>
