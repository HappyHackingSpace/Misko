<script setup>
import { ref, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAuth } from "../stores/auth.js";
import { useLab } from "../stores/lab.js";
import ThemeToggle from "../components/ThemeToggle.vue";
import LangSelect from "../components/LangSelect.vue";

const auth = useAuth();
const lab = useLab();
const router = useRouter();

onMounted(() => lab.load());

const email = ref("");
const password = ref("");
const err = ref("");
const busy = ref(false);

async function submit() {
  err.value = "";
  busy.value = true;
  try {
    await auth.login(email.value, password.value);
    router.push("/");
  } catch (e) {
    err.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="center">
    <div class="card authbox">
      <div class="row" style="justify-content:flex-end;gap:6px;margin-bottom:6px">
        <ThemeToggle />
        <LangSelect />
      </div>
      <div class="brand" style="font-size:22px;font-weight:800;margin-bottom:2px">{{ $t("app.name") }}</div>
      <p class="muted" v-if="lab.labName" style="margin:0 0 2px;font-weight:600">{{ lab.labName }}</p>
      <p class="muted" style="margin:0 0 10px">{{ $t("app.tagline") }}</p>
      <p class="muted">{{ $t("login.operatorLogin") }}</p>

      <form @submit.prevent="submit">
        <div class="field">
          <label>{{ $t("login.email") }}</label>
          <input v-model="email" type="email" required />
        </div>
        <div class="field">
          <label>{{ $t("login.password") }}</label>
          <input v-model="password" type="password" required />
        </div>
        <button class="primary" style="width:100%" :disabled="busy">
          {{ busy ? $t("common.loading") : $t("login.signIn") }}
        </button>
      </form>

      <p class="err" v-if="err">{{ err }}</p>
      <p class="muted" style="margin-top:14px;font-size:13px">
        {{ $t("login.noAccount") }}
      </p>
    </div>
  </div>
</template>
