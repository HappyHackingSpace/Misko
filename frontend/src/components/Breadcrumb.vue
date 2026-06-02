<script setup>
import { computed, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { useBreadcrumb } from "../stores/breadcrumb.js";

// Aktif sayfanin baslik anahtarini route meta'sindan okur (router.js'de
// her route'a `meta.titleKey` verilir). Pano disindaki sayfalarda
// "Pano / <sayfa>" gosterir; pano kendisi tek kirinti olur.
// Detay sayfalari breadcrumb store'a kendi zincirini yazarak (or.
// "Pano / Paradigmalar / <ad>") bu otomatik davranisi gecersiz kilabilir.
const route = useRoute();
const { t } = useI18n();
const crumb = useBreadcrumb();

// Her gezinmede override'i temizle: yeni sayfa kendi zincirini onMounted'da
// yeniden yazar; aksi halde eski detay zinciri ekranda asili kalir.
watch(
  () => route.fullPath,
  () => crumb.clear(),
);

const isHome = computed(() => route.path === "/");
const currentLabel = computed(() =>
  route.meta?.titleKey ? t(route.meta.titleKey) : "",
);
</script>

<template>
  <nav class="crumbs" aria-label="breadcrumb">
    <!-- Sayfanin yazdigi dinamik zincir varsa onu goster -->
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
      <RouterLink to="/">{{ $t("nav.dashboard") }}</RouterLink>
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
