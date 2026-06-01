<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api.js";

const items = ref([]);
const form = ref({ name: "", email: "", role: "OPERATOR", password: "" });
const err = ref("");
const notice = ref(""); // üretilen şifreyi bir kez göstermek için

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
      notice.value = `Kullanıcı oluşturuldu — ${user.email} için üretilen şifre (bir kez gösterilir): ${generatedPassword}`;
    }
    form.value = { name: "", email: "", role: "OPERATOR", password: "" };
    await load();
  } catch (e) {
    err.value = e.message;
  }
}

async function resetPassword(u) {
  err.value = "";
  notice.value = "";
  if (!confirm(`${u.email} için şifre sıfırlansın mı?`)) return;
  try {
    const { generatedPassword } = await api(`/users/${u.id}/reset-password`, { method: "POST", body: {} });
    notice.value = `${u.email} için yeni şifre (bir kez gösterilir): ${generatedPassword}`;
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
  if (!confirm(`${u.email} silinsin mi?`)) return;
  try {
    await api(`/users/${u.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    err.value = e.message;
  }
}
</script>

<template>
  <h1>Kullanıcı Yönetimi</h1>

  <div class="card">
    <p class="muted" style="margin-top:0">
      Bu internal bir uygulamadır — kayıt yok. Kullanıcıları buradan açın. Şifreyi boş bırakırsanız
      sistem güçlü bir şifre üretir ve <strong>bir kez</strong> gösterir.
    </p>
    <div class="row">
      <div class="field"><label>Ad</label><input v-model="form.name" placeholder="Ahmet Yılmaz" /></div>
      <div class="field"><label>E-posta</label><input v-model="form.email" type="email" placeholder="ahmet@fare.lab" /></div>
      <div class="field">
        <label>Rol</label>
        <select v-model="form.role">
          <option value="OPERATOR">OPERATOR</option>
          <option value="ADMIN">ADMIN</option>
        </select>
      </div>
      <div class="field"><label>Şifre (opsiyonel)</label><input v-model="form.password" placeholder="boşsa üretilir" /></div>
      <div><label>&nbsp;</label><button class="primary" @click="create" :disabled="!form.name || !form.email">Ekle</button></div>
    </div>
    <p class="err" v-if="err">{{ err }}</p>
    <p v-if="notice" class="notice">{{ notice }}</p>
  </div>

  <div class="card">
    <table>
      <thead><tr><th>Ad</th><th>E-posta</th><th>Rol</th><th>Eklendi</th><th></th></tr></thead>
      <tbody>
        <tr v-for="u in items" :key="u.id">
          <td>{{ u.name }}</td>
          <td class="muted">{{ u.email }}</td>
          <td>
            <select :value="u.role" @change="changeRole(u, $event.target.value)">
              <option value="OPERATOR">OPERATOR</option>
              <option value="ADMIN">ADMIN</option>
            </select>
          </td>
          <td class="muted">{{ new Date(u.createdAt).toLocaleDateString("tr-TR") }}</td>
          <td>
            <button @click="resetPassword(u)">Şifre sıfırla</button>
            <button class="danger" @click="remove(u)">Sil</button>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">Kullanıcı yok.</p>
  </div>
</template>
