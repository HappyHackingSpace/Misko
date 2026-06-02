<script setup>
import { ref, onMounted, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import { api } from "../api.js";

const { locale } = useI18n();
const router = useRouter();
const items = ref([]);
const err = ref("");

async function load() {
  err.value = "";
  try {
    items.value = await api(`/paradigms?lang=${locale.value}`);
  } catch (e) {
    err.value = e.message;
  }
}

// Open the detail page when a card is clicked.
function open(key) {
  router.push(`/paradigms/${key}`);
}

// Re-fetch backend-localized labels when the language changes.
watch(locale, load);

onMounted(load);
</script>

<template>
  <h1>{{ $t("paradigms.title") }}</h1>
  <p class="muted" style="margin-top:0">{{ $t("paradigms.intro") }}</p>
  <p class="err" v-if="err">{{ err }}</p>

  <!-- Read-only template catalog: click a card to open the detail page. -->
  <div class="list">
    <div
      v-for="p in items"
      :key="p.key"
      class="pcard"
      @click="open(p.key)"
    >
      <div class="pcard-head">
        <span class="pcard-name">{{ p.name }}</span>
      </div>
      <span class="pcard-cat">{{ $t("paradigms.categories." + p.category) }}</span>
      <div class="pcard-foot">
        <span class="pill">{{ p.metricCount }} {{ $t("paradigms.metricsShort") }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Flexible card grid: wraps on narrow screens, lays cards side by side when wide. */
.list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px; margin-bottom: 16px;
}
.pcard {
  display: flex; flex-direction: column; gap: 6px;
  background: var(--panel); border: 1px solid var(--line); border-radius: 14px;
  padding: 14px; cursor: pointer;
  transition: transform .12s ease, box-shadow .12s ease, border-color .12s ease;
}
.pcard:hover { transform: translateY(-2px); box-shadow: 0 6px 18px rgba(0,0,0,.18); border-color: var(--accent); }
.pcard-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.pcard-name { font-weight: 700; }
.pcard-cat { color: var(--muted); font-size: 12px; }
.pcard-foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 2px; }
</style>
