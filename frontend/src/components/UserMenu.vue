<script setup>
import { ref, onMounted, onBeforeUnmount } from "vue";
import { useRouter } from "vue-router";
import { useAuth } from "../stores/auth.js";

const auth = useAuth();
const router = useRouter();

const open = ref(false);
const root = ref(null);

function toggle() {
  open.value = !open.value;
}

function close() {
  open.value = false;
}

function onClickOutside(e) {
  if (root.value && !root.value.contains(e.target)) close();
}

function onKeydown(e) {
  if (e.key === "Escape") close();
}

function logout() {
  close();
  auth.logout();
  router.push("/login");
}

onMounted(() => {
  document.addEventListener("click", onClickOutside);
  document.addEventListener("keydown", onKeydown);
});
onBeforeUnmount(() => {
  document.removeEventListener("click", onClickOutside);
  document.removeEventListener("keydown", onKeydown);
});
</script>

<template>
  <div class="usermenu" ref="root">
    <button class="trigger" data-test="user-menu" @click="toggle" :aria-expanded="open">
      <span class="avatar">{{ (auth.user.name || "?").charAt(0).toUpperCase() }}</span>
      <span class="name">{{ auth.user.name }}</span>
      <svg class="chev" :class="{ up: open }" width="14" height="14" viewBox="0 0 24 24"
        fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="6 9 12 15 18 9" />
      </svg>
    </button>

    <div class="dropdown" v-if="open">
      <div class="head">
        <div class="who-name">{{ auth.user.name }}</div>
        <div class="who-meta">{{ auth.user.email }}</div>
        <span class="pill">{{ $t(`roles.${auth.user.role}`) }}</span>
      </div>
      <div class="sep"></div>
      <button class="item danger" data-test="logout" @click="logout">{{ $t("nav.logout") }}</button>
    </div>
  </div>
</template>

<style scoped>
.usermenu { position: relative; }
.trigger {
  display: inline-flex; align-items: center; gap: 8px;
  background: transparent; border: 1px solid transparent;
  padding: 4px 10px 4px 4px; border-radius: 999px; font-weight: 600;
  height: 36px; transition: background .12s ease, border-color .12s ease;
}
.trigger:hover { background: var(--btn-bg); border-color: var(--line); }
.avatar {
  width: 27px; height: 27px; border-radius: 50%;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--accent); color: var(--accent-fg); font-size: 12.5px; font-weight: 800;
  flex-shrink: 0;
}
.name { font-size: 13px; max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 520px) {
  .name { display: none; }
  .dropdown { right: -10px; min-width: 200px; }
}
.chev { color: var(--muted); transition: transform .15s ease; }
.chev.up { transform: rotate(180deg); }
.dropdown {
  position: absolute; right: 0; top: calc(100% + 6px); min-width: 220px;
  background: var(--panel); border: 1px solid var(--line); border-radius: 12px;
  box-shadow: 0 10px 30px rgba(0,0,0,.25); padding: 6px; z-index: 50;
}
.head { padding: 10px 10px 8px; }
.who-name { font-weight: 700; font-size: 14px; }
.who-meta { color: var(--muted); font-size: 12px; margin: 2px 0 8px; word-break: break-all; }
.sep { height: 1px; background: var(--line); margin: 4px 0; }
.item {
  display: block; width: 100%; text-align: left; border: none; background: transparent;
  padding: 9px 10px; border-radius: 8px; font-weight: 600;
}
.item:hover { background: var(--active-bg); }
.item.danger { color: var(--bad); }
</style>
