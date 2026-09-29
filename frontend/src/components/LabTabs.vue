<script setup>
// Centered tab switcher shared by the Paradigms (read-only catalog) and
// Environments (lab setups) pages, so they read as one "lab setup" section.
</script>

<template>
  <nav class="lab-tabs">
    <RouterLink to="/paradigms" class="tab">{{ $t("nav.paradigms") }}</RouterLink>
    <RouterLink to="/environments" class="tab">{{ $t("nav.environments") }}</RouterLink>
  </nav>
</template>

<style scoped>
.lab-tabs { display: inline-flex; gap: 4px; padding: 4px; border: 1px solid var(--line); border-radius: 11px; background: var(--panel2); }
.tab { padding: 7px 16px; border-radius: 8px; color: var(--muted); font-weight: 600; }
.tab:hover { color: var(--txt); }
.tab.router-link-active { background: var(--accent); color: var(--accent-fg); }
</style>
