<script setup>
// Shared list/detail page header: title (+ optional subtitle) on the left, an
// optional count chip and page-level actions on the right.
defineProps({
  title: { type: String, required: true },
  subtitle: { type: String, default: "" },
  count: { type: [Number, String], default: undefined },
});
</script>

<template>
  <div class="page-head">
    <div class="page-head-text">
      <h1>{{ title }}</h1>
      <p v-if="subtitle" class="page-head-sub">{{ subtitle }}</p>
    </div>
    <div class="page-head-spacer"></div>
    <span v-if="count !== undefined" class="count-chip">{{ count }}</span>
    <slot name="actions" />
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: center; gap: 14px; margin-bottom: 18px; flex-wrap: wrap; }
.page-head-text { min-width: 0; }
.page-head-text h1 { margin: 0; font-size: 22px; letter-spacing: -0.01em; }
.page-head-sub { margin: 2px 0 0; font-size: 13px; color: var(--muted); }
.page-head-spacer { flex: 1; min-width: 8px; }
.count-chip {
  font-size: 12px; font-weight: 700; color: var(--muted);
  background: var(--btn-bg); border: 1px solid var(--line); border-radius: 999px;
  padding: 5px 11px; white-space: nowrap;
}

@media (max-width: 520px) {
  .page-head-sub { display: none; }
}
</style>
