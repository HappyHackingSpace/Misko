<script setup>
import { computed, h, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NConfigProvider, NLayout, NLayoutHeader, NLayoutSider, NLayoutContent, NMenu, dateEnUS, dateTrTR, enUS, trTR } from "naive-ui";
import { useAuth } from "./stores/auth.js";
import { useLab } from "./stores/lab.js";
import { usePrefs } from "./stores/prefs.js";
import { darkTheme, darkThemeOverrides, lightThemeOverrides } from "./theme.js";
import ThemeToggle from "./components/ThemeToggle.vue";
import LangSelect from "./components/LangSelect.vue";
import UserMenu from "./components/UserMenu.vue";
import Breadcrumb from "./components/Breadcrumb.vue";
import NavIcon from "./components/NavIcon.vue";

const auth = useAuth();
const lab = useLab();
const prefs = usePrefs();
const route = useRoute();
const router = useRouter();
const { t } = useI18n();

onMounted(() => lab.load());

const naiveTheme = computed(() => (prefs.theme === "dark" ? darkTheme : null));
const naiveThemeOverrides = computed(() => (prefs.theme === "dark" ? darkThemeOverrides : lightThemeOverrides));
// Naive UI's own strings (pagination, empty states, dropdowns...) live in a
// separate locale system from vue-i18n, so the app's language switch has to
// drive both.
const naiveLocale = computed(() => (prefs.locale === "tr" ? trTR : enUS));
const naiveDateLocale = computed(() => (prefs.locale === "tr" ? dateTrTR : dateEnUS));

// Desktop: the sidebar collapses to an icon rail and remembers the choice.
// Mobile (<860px): it is a full-drawer overlay instead, so "collapsed" there
// means "off-screen", tracked separately and reset on navigation.
const COLLAPSE_KEY = "misko.sidebarCollapsed";
const collapsed = ref(localStorage.getItem(COLLAPSE_KEY) === "1");
function toggleCollapse() {
  collapsed.value = !collapsed.value;
  localStorage.setItem(COLLAPSE_KEY, collapsed.value ? "1" : "0");
}

const mobileOpen = ref(false);
const toggleMobile = () => (mobileOpen.value = !mobileOpen.value);
const closeMobile = () => (mobileOpen.value = false);
watch(() => route.fullPath, closeMobile);

const isMobile = ref(window.matchMedia("(max-width: 860px)").matches);
let mql;
function syncMobile(e) {
  isMobile.value = e.matches;
  if (!e.matches) mobileOpen.value = false;
}
onMounted(() => {
  mql = window.matchMedia("(max-width: 860px)");
  mql.addEventListener("change", syncMobile);
});
onUnmounted(() => mql?.removeEventListener("change", syncMobile));

const railCollapsed = computed(() => !isMobile.value && collapsed.value);

function navIcon(name) {
  return () => h(NavIcon, { name });
}

// Labels are plain strings (translated reactively through `t`) except where a
// browser test hooks into one, which needs a tagged span. The menu's own
// selection event drives navigation with router.push below, rather than
// embedding a RouterLink inside a label render function: Naive UI's collapsed
// icon rail only mounts label content into a hover popover, so a click handler
// living inside it would not fire from the always-visible icon.
function navLabel(key, testId) {
  return testId ? () => h("span", { "data-test": testId }, t(key)) : t(key);
}

// Two separate menus, not one with a Naive `type: "group"` section: a group
// header still renders its full text in the collapsed icon rail (there is no
// per-group collapse content), which just clips. A plain heading we hide
// ourselves when collapsed avoids that.
const primaryOptions = computed(() => [
  { label: navLabel("nav.dashboard", "nav-dashboard"), key: "/dashboard", icon: navIcon("dashboard") },
  { label: navLabel("nav.experiments", "nav-experiments"), key: "/experiments", icon: navIcon("experiments") },
  { label: navLabel("nav.tests", "nav-tests"), key: "/tests", icon: navIcon("test") },
  { label: t("nav.subjects"), key: "/subjects", icon: navIcon("subjects") },
  { label: navLabel("nav.reports", "nav-reports"), key: "/reports", icon: navIcon("reports") },
]);

const managementOptions = computed(() => {
  const options = [
    { label: t("nav.paradigms"), key: "/paradigms", icon: navIcon("paradigms") },
    { label: t("nav.environments"), key: "/environments", icon: navIcon("environments") },
  ];
  if (auth.canManageUsers) options.push({ label: t("nav.users"), key: "/users", icon: navIcon("users") });
  return options;
});

function onMenuSelect(key) {
  if (key && key !== route.path) router.push(key);
}

const topLevelKeys = ["/dashboard", "/experiments", "/tests", "/subjects", "/reports", "/paradigms", "/environments", "/users"];
const activeKey = computed(() => topLevelKeys.find((k) => route.path.startsWith(k)) ?? null);
</script>

<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="naiveThemeOverrides" :locale="naiveLocale" :date-locale="naiveDateLocale" class="theme-root">
    <div v-if="auth.isLoggedIn" class="app">
      <n-layout has-sider class="shell">
        <div v-if="isMobile && mobileOpen" class="sidebar-overlay" @click="closeMobile"></div>
        <n-layout-sider
          bordered
          :width="232"
          :collapsed-width="68"
          :collapsed="isMobile ? !mobileOpen : collapsed"
          :collapse-mode="isMobile ? 'transform' : 'width'"
          :show-trigger="false"
          class="app-sider"
        >
          <div class="sider-inner">
            <RouterLink to="/dashboard" class="brand" :class="{ collapsed: railCollapsed }">
              <!-- A running mouse reduced to four shapes: an ear and body sharing
                   one fill so they read as one silhouette, a tail stroke, and an
                   eye punched through in the badge's own background color. -->
              <svg class="brand-mark" viewBox="0 0 32 32" aria-hidden="true">
                <rect width="32" height="32" rx="9" fill="var(--accent)" />
                <path d="M22 19c3.4.5 6.1 3.1 6.7 6.5" stroke="var(--accent-fg)" stroke-width="1.8" fill="none" stroke-linecap="round" />
                <circle cx="11" cy="10.2" r="4" fill="var(--accent-fg)" />
                <ellipse cx="15.2" cy="17.2" rx="8.4" ry="6.2" transform="rotate(-10 15.2 17.2)" fill="var(--accent-fg)" />
                <circle cx="8.7" cy="15.6" r="1.1" fill="var(--accent)" />
              </svg>
              <span v-if="!railCollapsed" class="brand-text">
                <span class="brand-name">{{ $t("app.name") }}</span>
                <span class="brand-lab" v-if="lab.labName">{{ lab.labName }}</span>
              </span>
            </RouterLink>

            <div class="sider-scroll">
              <div class="section-label first" v-if="!railCollapsed">{{ $t("nav.general") }}</div>
              <div v-else class="section-divider first"></div>
              <n-menu
                :value="activeKey"
                :options="primaryOptions"
                :collapsed="railCollapsed"
                :collapsed-width="68"
                :collapsed-icon-size="20"
                @update:value="onMenuSelect"
              />
              <div class="section-label" v-if="!railCollapsed">{{ $t("nav.reference") }}</div>
              <div v-else class="section-divider"></div>
              <n-menu
                :value="activeKey"
                :options="managementOptions"
                :collapsed="railCollapsed"
                :collapsed-width="68"
                :collapsed-icon-size="20"
                @update:value="onMenuSelect"
              />
            </div>

            <button v-if="!isMobile" class="collapse-toggle" :aria-label="$t('nav.menu')" @click="toggleCollapse">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" :style="{ transform: collapsed ? 'rotate(180deg)' : 'none' }">
                <polyline points="14 6 8 12 14 18" />
              </svg>
              <span v-if="!collapsed">{{ $t("nav.collapse") }}</span>
            </button>
          </div>
        </n-layout-sider>

        <n-layout class="main-col">
          <n-layout-header bordered class="topbar">
            <button class="icon-btn menu-btn" :aria-label="$t('nav.menu')" @click="toggleMobile">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round">
                <line x1="3" y1="6" x2="21" y2="6" />
                <line x1="3" y1="12" x2="21" y2="12" />
                <line x1="3" y1="18" x2="21" y2="18" />
              </svg>
            </button>
            <div class="spacer"></div>
            <ThemeToggle />
            <LangSelect />
            <UserMenu />
          </n-layout-header>
          <n-layout-content content-style="padding: 26px 30px;" class="app-content">
            <Breadcrumb />
            <RouterView />
          </n-layout-content>
        </n-layout>
      </n-layout>
    </div>
    <RouterView v-else />
  </n-config-provider>
</template>

<style scoped>
.theme-root { display: contents; }
.app { height: 100vh; }
/* Naive's layouts paint an opaque bodyColor; make the content area transparent so
   the dotted body background (style.css) shows through. */
.shell, .main-col, .app-content { background-color: transparent !important; }
.shell { height: 100%; }
.main-col { height: 100%; }

.sider-inner { display: flex; flex-direction: column; height: 100%; }

.brand {
  display: flex; align-items: center; gap: 10px; min-width: 0;
  padding: 16px 18px; height: 56px; flex-shrink: 0;
}
.brand.collapsed { padding: 14px; justify-content: center; }
.brand-mark { width: 30px; height: 30px; flex-shrink: 0; }
.brand-text { display: flex; flex-direction: column; min-width: 0; gap: 2px; }
.brand-name {
  font-weight: 700; font-size: 18px; letter-spacing: -0.03em;
  color: var(--txt); white-space: nowrap; line-height: 1.1;
}
.brand-lab {
  font-weight: 600; font-size: 10.5px; letter-spacing: 0.07em; text-transform: uppercase;
  color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; line-height: 1.1;
}

.sider-scroll { flex: 1; min-height: 0; overflow-y: auto; padding: 8px 0; }
.section-label {
  margin: 14px 16px 6px; font-size: 11px; font-weight: 700;
  text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted);
}
.section-label.first { margin-top: 18px; }
.section-divider { margin: 8px auto; width: 32px; border-top: 1px solid var(--line); }
.section-divider.first { margin-top: 18px; }

/* Replace Naive's filled "pill" selection with a soft tint plus a left
   accent bar, and make the state change a transition instead of a snap. */
:deep(.n-menu-item-content) {
  position: relative;
  transition: color 0.15s ease, background-color 0.15s ease;
}
:deep(.n-menu-item-content::before) { transition: background-color 0.15s ease, opacity 0.15s ease; }
:deep(.n-menu-item-content--selected)::after {
  content: "";
  position: absolute;
  left: 0; top: 22%; bottom: 22%;
  width: 3px; border-radius: 0 3px 3px 0;
  background: var(--accent);
}

.collapse-toggle {
  display: flex; align-items: center; justify-content: center; gap: 8px; flex-shrink: 0;
  margin: 8px; padding: 9px 10px; border-radius: 8px;
  border: none; background: transparent; color: var(--muted);
  font-size: 12.5px; font-weight: 600; cursor: pointer;
}
.collapse-toggle:hover { background: var(--active-bg); color: var(--txt); }
.collapse-toggle svg { flex-shrink: 0; transition: transform 0.15s ease; }

.topbar { display: flex; align-items: center; gap: 12px; padding: 0 20px; height: 56px; }
.spacer { flex: 1; }
.menu-btn { display: none; }
.app-content { height: 100%; overflow-y: auto; }

@media (max-width: 860px) {
  .menu-btn { display: inline-flex; }
  .topbar { padding: 0 14px; gap: 10px; }
  :deep(.app-sider) { position: fixed; top: 0; left: 0; bottom: 0; z-index: 35; }
  .app-content { padding: 16px !important; }
}
</style>
