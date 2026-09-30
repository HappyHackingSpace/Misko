<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NPopconfirm, NSelect } from "naive-ui";
import { api } from "../api/client.js";
import { ASSIGNABLE_ROLES, DEFAULT_ROLE } from "../constants/roles.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const isNew = computed(() => !route.params.id);
const form = ref({ name: "", email: "", role: DEFAULT_ROLE, password: "" });
const err = ref("");
const notice = ref(""); // show the generated password once
const saving = ref(false);

const roleOptions = ASSIGNABLE_ROLES.map((r) => ({ value: r, label: () => t(`roles.${r}`) }));
// The founding SUPERADMIN account: the API rejects any rename, role change,
// password reset or deletion of it, by anyone, so the form does not offer them.
const isSuperAdmin = computed(() => !isNew.value && form.value.role === "SUPERADMIN");

function syncCrumb() {
  crumb.set([
    { label: t("nav.experiments"), to: "/experiments" },
    { label: t("users.title"), to: "/users" },
    { label: isNew.value ? t("common.new") : form.value.name || form.value.email || route.params.id },
  ]);
}

async function load() {
  err.value = "";
  if (isNew.value) {
    syncCrumb();
    return;
  }
  try {
    const u = await api(`/users/${route.params.id}`);
    form.value = { name: u.name, email: u.email, role: u.role, password: "" };
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function save() {
  err.value = "";
  notice.value = "";
  saving.value = true;
  try {
    if (isNew.value) {
      const body = { name: form.value.name, email: form.value.email, role: form.value.role };
      if (form.value.password) body.password = form.value.password;
      const { user, generatedPassword } = await api("/users", { method: "POST", body });
      if (generatedPassword) {
        notice.value = t("users.createdWithPassword", { email: user.email, password: generatedPassword });
        form.value = { name: "", email: "", role: DEFAULT_ROLE, password: "" };
        return; // stay on page so the one-time password can be copied
      }
      router.push("/users");
    } else {
      await api(`/users/${route.params.id}`, { method: "PATCH", body: { name: form.value.name, role: form.value.role } });
      router.push("/users");
    }
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function resetPassword() {
  err.value = "";
  notice.value = "";
  try {
    const { generatedPassword } = await api(`/users/${route.params.id}/reset-password`, { method: "POST", body: {} });
    notice.value = t("users.newPassword", { email: form.value.email, password: generatedPassword });
  } catch (e) {
    err.value = e.message;
  }
}

async function remove() {
  err.value = "";
  try {
    await api(`/users/${route.params.id}`, { method: "DELETE" });
    router.push("/users");
  } catch (e) {
    err.value = e.message;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <PageHead :title="isNew ? $t('common.new') : form.name" />

  <NAlert v-if="err" type="error" :title="err" style="margin-bottom: 16px" />
  <NAlert v-if="notice" type="success" :title="notice" style="margin-bottom: 16px" />
  <NAlert v-if="isSuperAdmin" type="info" :title="$t('users.superAdminProtected')" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small">
    <div class="form">
      <label class="fld">
        <span>{{ $t("common.name") }}</span>
        <input v-model="form.name" :disabled="isSuperAdmin" :placeholder="$t('users.namePlaceholder')" />
      </label>
      <label class="fld">
        <span>{{ $t("users.email") }}</span>
        <input v-model="form.email" type="email" :disabled="!isNew" :placeholder="$t('users.emailPlaceholder')" />
      </label>
      <label class="fld">
        <span>{{ $t("users.role") }}</span>
        <NSelect v-if="!isSuperAdmin" v-model:value="form.role" :options="roleOptions" />
        <input v-else :value="$t('roles.SUPERADMIN')" disabled />
      </label>
      <label class="fld" v-if="isNew">
        <span>{{ $t("users.passwordOptional") }}</span>
        <input v-model="form.password" :placeholder="$t('users.passwordPlaceholder')" />
      </label>

      <div class="actions wide">
        <NButton @click="router.push('/users')">{{ $t("common.back") }}</NButton>
        <!-- Both act at once and cannot be undone, so each asks in place. -->
        <NPopconfirm v-if="!isNew && !isSuperAdmin" @positive-click="resetPassword">
          <template #trigger><NButton data-test="user-reset">{{ $t("users.resetPassword") }}</NButton></template>
          {{ $t("users.confirmReset", { email: form.email }) }}
        </NPopconfirm>
        <NPopconfirm v-if="!isNew && !isSuperAdmin" @positive-click="remove">
          <template #trigger><NButton type="error" ghost data-test="user-delete">{{ $t("common.delete") }}</NButton></template>
          {{ $t("users.confirmDelete", { email: form.email }) }}
        </NPopconfirm>
        <NButton v-if="!isSuperAdmin" type="primary" :loading="saving" :disabled="!form.name || !form.email" @click="save">
          {{ isNew ? $t("common.create") : $t("common.save") }}
        </NButton>
      </div>
    </div>
  </NCard>
</template>

<style scoped>
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; align-items: start; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.fld :deep(.n-select) { width: 100%; }
.wide { grid-column: 1 / -1; }
.actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
</style>
