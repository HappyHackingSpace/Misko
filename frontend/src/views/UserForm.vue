<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api/client.js";
import { ROLES, DEFAULT_ROLE } from "../constants/roles.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const isNew = computed(() => !route.params.id);
const form = ref({ name: "", email: "", role: DEFAULT_ROLE, password: "" });
const err = ref("");
const notice = ref(""); // show the generated password once
const saving = ref(false);

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
  if (!confirm(t("users.confirmReset", { email: form.value.email }))) return;
  try {
    const { generatedPassword } = await api(`/users/${route.params.id}/reset-password`, { method: "POST", body: {} });
    notice.value = t("users.newPassword", { email: form.value.email, password: generatedPassword });
  } catch (e) {
    err.value = e.message;
  }
}

async function remove() {
  err.value = "";
  if (!confirm(t("users.confirmDelete", { email: form.value.email }))) return;
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
  <div class="card">
    <h2 style="margin-top:0">{{ isNew ? $t("common.new") : form.name }}</h2>
    <div class="row">
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" :placeholder="$t('users.namePlaceholder')" /></div>
      <div class="field"><label>{{ $t("users.email") }}</label><input v-model="form.email" type="email" :disabled="!isNew" :placeholder="$t('users.emailPlaceholder')" /></div>
      <div class="field">
        <label>{{ $t("users.role") }}</label>
        <select v-model="form.role">
          <option v-for="r in ROLES" :key="r" :value="r">{{ $t(`roles.${r}`) }}</option>
        </select>
      </div>
      <div class="field" v-if="isNew"><label>{{ $t("users.passwordOptional") }}</label><input v-model="form.password" :placeholder="$t('users.passwordPlaceholder')" /></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <p v-if="notice" class="notice">{{ notice }}</p>
    <div class="actions">
      <button @click="router.push('/users')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" @click="resetPassword">{{ $t("users.resetPassword") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.name || !form.email" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; flex-wrap: wrap; }
</style>
