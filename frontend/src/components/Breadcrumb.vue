<script setup>
import { computed, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { useBreadcrumb } from "../stores/breadcrumb.js";

// Reads the active page's title key from the route meta (router.js assigns
// `meta.titleKey` to every route). Off the dashboard it shows "Dashboard /
// <page>"; the dashboard itself is a single crumb. Detail pages can override
// this automatic behavior by writing their own trail to the breadcrumb store
// (e.g. "Dashboard / Paradigms / <name>").
const route = useRoute();
const { t } = useI18n();
const crumb = useBreadcrumb();

// Clear the override on every navigation: the new page rewrites its own trail
// in onMounted; otherwise a stale detail trail would linger on screen.
watch(
  () => route.fullPath,
  () => crumb.clear(),
);

const isHome = computed(() => route.path === "/dashboard");
const currentLabel = computed(() =>
  route.meta?.titleKey ? t(route.meta.titleKey) : "",
);
</script>

<template>
  <nav class="crumbs" aria-label="breadcrumb">
    <!-- If the page wrote a dynamic trail, render it -->
    <template v-if="crumb.trail">
      <template v-for="(c, i) in crumb.trail" :key="i">
        <RouterLink v-if="c.to" :to="c.to">{{ c.label }}</RouterLink>
        <span v-else class="current">{{ c.label }}</span>
        <span v-if="i < crumb.trail.length - 1" class="sep">/</span>
      </template>
    </template>
    <template v-else-if="isHome">
      <span class="current">{{ $t("nav.dashboard") }}</span>
    </template>
    <template v-else>
      <RouterLink to="/dashboard">{{ $t("nav.dashboard") }}</RouterLink>
      <span class="sep">/</span>
      <span class="current">{{ currentLabel }}</span>
    </template>
  </nav>
</template>

<style scoped>
.crumbs { display: flex; align-items: center; gap: 8px; font-size: 13px; margin-bottom: 12px; }
.crumbs .sep { color: var(--muted); }
.crumbs .current { color: var(--muted); }
</style>
