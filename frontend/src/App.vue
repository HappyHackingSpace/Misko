<script setup>
import { onMounted } from "vue";
import { useAuth } from "./stores/auth.js";
import { useLab } from "./stores/lab.js";
import ThemeToggle from "./components/ThemeToggle.vue";
import LangSelect from "./components/LangSelect.vue";
import UserMenu from "./components/UserMenu.vue";
import Breadcrumb from "./components/Breadcrumb.vue";

const auth = useAuth();
const lab = useLab();

onMounted(() => lab.load());
</script>

<template>
  <div v-if="auth.isLoggedIn" class="app">
    <header class="topbar">
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
      <aside class="sidebar">
        <nav class="nav">
          <RouterLink to="/">{{ $t("nav.dashboard") }}</RouterLink>
          <RouterLink to="/tests">{{ $t("nav.tests") }}</RouterLink>
          <RouterLink to="/scenarios">{{ $t("nav.scenarios") }}</RouterLink>
          <RouterLink to="/paradigms">{{ $t("nav.paradigms") }}</RouterLink>
          <RouterLink to="/environments">{{ $t("nav.environments") }}</RouterLink>
          <RouterLink to="/subjects">{{ $t("nav.subjects") }}</RouterLink>
          <RouterLink to="/devices">{{ $t("nav.devices") }}</RouterLink>
          <RouterLink v-if="auth.isAdmin" to="/users">{{ $t("nav.users") }}</RouterLink>
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
