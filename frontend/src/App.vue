<script setup>
import { useRouter } from "vue-router";
import { useAuth } from "./stores/auth.js";

const auth = useAuth();
const router = useRouter();

function logout() {
  auth.logout();
  router.push("/login");
}
</script>

<template>
  <div v-if="auth.isLoggedIn" class="app">
    <header class="topbar">
      <div class="brand">🐭 Mişko</div>
      <nav class="nav">
        <RouterLink to="/">Panel</RouterLink>
        <RouterLink to="/tests">Testler</RouterLink>
        <RouterLink to="/scenarios">Senaryolar</RouterLink>
        <RouterLink to="/subjects">Denekler</RouterLink>
        <RouterLink to="/devices">Cihazlar</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/users">Kullanıcılar</RouterLink>
      </nav>
      <div class="spacer"></div>
      <div class="who">{{ auth.user.name }} <span class="muted">· {{ auth.user.role }}</span></div>
      <button @click="logout">Çıkış</button>
    </header>
    <main class="main">
      <RouterView />
    </main>
  </div>
  <RouterView v-else />
</template>
