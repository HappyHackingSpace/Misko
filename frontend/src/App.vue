<script setup>
import { onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useAuth } from "./stores/auth.js";
import { useLab } from "./stores/lab.js";
import ThemeToggle from "./components/ThemeToggle.vue";
import LangSelect from "./components/LangSelect.vue";
import UserMenu from "./components/UserMenu.vue";
import Breadcrumb from "./components/Breadcrumb.vue";

const auth = useAuth();
const lab = useLab();
const route = useRoute();

onMounted(() => lab.load());

// The sidebar becomes a drawer on narrow screens; navigating closes it.
const sidebarOpen = ref(false);
const toggleSidebar = () => (sidebarOpen.value = !sidebarOpen.value);
const closeSidebar = () => (sidebarOpen.value = false);
watch(() => route.fullPath, closeSidebar);
</script>

<template>
  <div v-if="auth.isLoggedIn" class="app">
    <header class="topbar">
      <button class="icon-btn menu-btn" :aria-label="$t('nav.menu')" @click="toggleSidebar">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round">
          <line x1="3" y1="6" x2="21" y2="6" />
          <line x1="3" y1="12" x2="21" y2="12" />
          <line x1="3" y1="18" x2="21" y2="18" />
        </svg>
      </button>
      <div class="brand">
        {{ $t("app.name") }}
        <span class="lab" v-if="lab.labName">{{ lab.labName }}</span>
      </div>
      <div class="spacer"></div>
      <ThemeToggle />
      <LangSelect />
      <UserMenu />
    </header>
    <div class="body">
      <div v-if="sidebarOpen" class="sidebar-overlay" @click="closeSidebar"></div>
      <aside class="sidebar" :class="{ open: sidebarOpen }">
        <nav class="nav">
          <RouterLink to="/experiments" data-test="nav-experiments">{{ $t("nav.experiments") }}</RouterLink>
          <RouterLink to="/subjects">{{ $t("nav.subjects") }}</RouterLink>
          <RouterLink to="/reports" data-test="nav-reports">{{ $t("nav.reports") }}</RouterLink>

          <div class="nav-section">{{ $t("nav.reference") }}</div>
          <RouterLink to="/paradigms">{{ $t("nav.paradigms") }}</RouterLink>
          <RouterLink to="/environments">{{ $t("nav.environments") }}</RouterLink>
          <RouterLink v-if="auth.canManageUsers" to="/users">{{ $t("nav.users") }}</RouterLink>
        </nav>
      </aside>
      <main class="main">
        <Breadcrumb />
        <RouterView />
      </main>
    </div>
  </div>
  <RouterView v-else />
</template>
