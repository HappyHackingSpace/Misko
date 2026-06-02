<script setup>
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { ROLES, DEFAULT_ROLE } from "../constants/roles.js";

const { t, locale } = useI18n();
const items = ref([]);
const form = ref({ name: "", email: "", role: DEFAULT_ROLE, password: "" });
const err = ref("");
const notice = ref(""); // show the generated password once

async function load() {
  items.value = await api("/users");
}
onMounted(load);

async function create() {
  err.value = "";
  notice.value = "";
  try {
    const body = { name: form.value.name, email: form.value.email, role: form.value.role };
    if (form.value.password) body.password = form.value.password;
    const { user, generatedPassword } = await api("/users", { method: "POST", body });
    if (generatedPassword) {
      notice.value = t("users.createdWithPassword", { email: user.email, password: generatedPassword });
    }
    form.value = { name: "", email: "", role: DEFAULT_ROLE, password: "" };
    await load();
  } catch (e) {
    err.value = e.message;
  }
}

async function resetPassword(u) {
  err.value = "";
  notice.value = "";
  if (!confirm(t("users.confirmReset", { email: u.email }))) return;
  try {
    const { generatedPassword } = await api(`/users/${u.id}/reset-password`, { method: "POST", body: {} });
    notice.value = t("users.newPassword", { email: u.email, password: generatedPassword });
  } catch (e) {
    err.value = e.message;
  }
}

async function changeRole(u, role) {
  err.value = "";
  try {
    await api(`/users/${u.id}`, { method: "PATCH", body: { role } });
    await load();
  } catch (e) {
    err.value = e.message;
    await load();
  }
}

async function remove(u) {
  err.value = "";
  if (!confirm(t("users.confirmDelete", { email: u.email }))) return;
  try {
    await api(`/users/${u.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    err.value = e.message;
  }
}
</script>

<template>
  <h1>{{ $t("users.title") }}</h1>

  <div class="card">
    <p class="muted" style="margin-top:0">{{ $t("users.intro") }}</p>
    <div class="row">
      <div class="field"><label>{{ $t("common.name") }}</label><input v-model="form.name" :placeholder="$t('users.namePlaceholder')" /></div>
      <div class="field"><label>{{ $t("users.email") }}</label><input v-model="form.email" type="email" :placeholder="$t('users.emailPlaceholder')" /></div>
      <div class="field">
        <label>{{ $t("users.role") }}</label>
        <select v-model="form.role">
          <option v-for="r in ROLES" :key="r" :value="r">{{ r }}</option>
        </select>
      </div>
      <div class="field"><label>{{ $t("users.passwordOptional") }}</label><input v-model="form.password" :placeholder="$t('users.passwordPlaceholder')" /></div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.name || !form.email">{{ $t("common.add") }}</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <p v-if="notice" class="notice">{{ notice }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>{{ $t("common.name") }}</th><th>{{ $t("users.email") }}</th><th>{{ $t("users.role") }}</th><th>{{ $t("common.addedAt") }}</th><th></th></tr></thead>
      <tbody>
        <tr v-for="u in items" :key="u.id">
          <td>{{ u.name }}</td>
          <td class="muted">{{ u.email }}</td>
          <td>
            <select :value="u.role" @change="changeRole(u, $event.target.value)">
              <option v-for="r in ROLES" :key="r" :value="r">{{ r }}</option>
            </select>
          </td>
          <td class="muted">{{ new Date(u.createdAt).toLocaleDateString(locale) }}</td>
          <td>
            <button @click="resetPassword(u)">{{ $t("users.resetPassword") }}</button>
            <button class="danger" @click="remove(u)">{{ $t("common.delete") }}</button>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">{{ $t("users.empty") }}</p>
  </div>
</template>
