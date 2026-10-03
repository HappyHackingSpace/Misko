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
          <label for="login-email">{{ $t("login.email") }}</label>
          <input id="login-email" v-model="email" type="email" autocomplete="username" required />
        </div>
        <div class="field">
          <label for="login-password">{{ $t("login.password") }}</label>
          <input id="login-password" v-model="password" type="password" autocomplete="current-password" required />
        </div>
        <button class="primary sign-in" data-test="sign-in" :disabled="busy" :aria-busy="busy">
          <span v-if="busy" class="spinner" role="status" :aria-label="$t('common.loading')"></span>
          <span v-else>{{ $t("login.signIn") }}</span>
        </button>
      </form>

      <p class="err" v-if="err">{{ err }}</p>
      <p class="muted" style="margin-top:14px;font-size:13px">
        {{ $t("login.noAccount") }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.sign-in { width: 100%; display: inline-flex; align-items: center; justify-content: center; min-height: 40px; }
/* Currentcolor keeps the ring readable on both the dark and the light accent. */
.spinner {
  width: 18px; height: 18px; border-radius: 50%;
  border: 2.5px solid currentColor; border-right-color: transparent;
  animation: spin 0.7s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .spinner { animation-duration: 1.6s; } }
</style>
