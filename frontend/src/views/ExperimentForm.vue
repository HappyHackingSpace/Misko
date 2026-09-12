<script setup>
// One experiment, created or edited. The API owns the rules: the code shape,
// the title and description lengths, and whether a design that requires a
// control group has one. This form sends what was typed and shows what the API
// answers rather than repeating those rules here.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { experiments } from "../api/endpoints.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const isNew = computed(() => !route.params.id);
const form = ref({ code: "", title: "", description: "", requiresControl: false });
const error = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("experiments.title"), to: "/experiments" },
    { label: isNew.value ? t("experiments.new") : form.value.code || route.params.id },
  ]);
}

async function load() {
  error.value = "";
  if (!isNew.value) {
    try {
      const e = await experiments.get(route.params.id);
      form.value = {
        code: e.code,
        title: e.title,
        description: e.description || "",
        requiresControl: !!e.requiresControl,
      };
    } catch (e) {
      error.value = e.message;
    }
  }
  syncCrumb();
}

async function save() {
  error.value = "";
  saving.value = true;
  try {
    const body = {
      code: form.value.code.trim(),
      title: form.value.title.trim(),
      description: form.value.description.trim(),
      requiresControl: form.value.requiresControl,
    };
    if (isNew.value) {
      const created = await experiments.create(body);
      // Straight to the detail screen: a new experiment still needs its groups
      // before anything can be planned in it.
      router.push(`/experiments/${created.id}`);
      return;
    }
    await experiments.update(route.params.id, body);
    router.push(`/experiments/${route.params.id}`);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <div class="head">
    <h1>{{ isNew ? $t("experiments.new") : $t("experiments.edit") }}</h1>
  </div>

  <p class="err" v-if="error" data-test="error">{{ error }}</p>

  <form class="card form" data-test="experiment-form" @submit.prevent="save">
    <label class="fld">
      <span>{{ $t("experiments.code") }}</span>
      <input v-model="form.code" data-test="experiment-code" required />
    </label>

    <label class="fld">
      <span>{{ $t("experiments.name") }}</span>
      <input v-model="form.title" data-test="experiment-title" required />
    </label>

    <label class="fld wide">
      <span>{{ $t("common.description") }}</span>
      <textarea v-model="form.description" rows="3" data-test="experiment-description"></textarea>
    </label>

    <label class="check wide">
      <input type="checkbox" v-model="form.requiresControl" data-test="experiment-requires-control" />
      <span>{{ $t("experiments.requiresControl") }}</span>
    </label>
    <p class="muted wide">{{ $t("experiments.requiresControlHint") }}</p>

    <div class="actions">
      <button type="submit" :disabled="saving" data-test="experiment-save">{{ $t("common.save") }}</button>
      <RouterLink class="link" to="/experiments">{{ $t("common.cancel") }}</RouterLink>
    </div>
  </form>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; align-items: start; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.check { display: flex; align-items: center; gap: 8px; }
.wide { grid-column: 1 / -1; }
.muted { margin: 0; font-size: 12px; }
.actions { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; }
</style>
