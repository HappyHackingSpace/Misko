<script setup>
// One experiment, created or edited. The API owns the rules: the code shape,
// the title and description lengths, and whether a design that requires a
// control group has one. This form sends what was typed and shows what the API
// answers rather than repeating those rules here.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NCheckbox } from "naive-ui";
import { experiments } from "../api/endpoints.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";

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
  <PageHead :title="isNew ? $t('experiments.new') : $t('experiments.edit')" />

  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small">
    <form class="form" data-test="experiment-form" @submit.prevent="save">
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

      <div class="wide">
        <NCheckbox v-model:checked="form.requiresControl" data-test="experiment-requires-control">
          {{ $t("experiments.requiresControl") }}
        </NCheckbox>
        <p class="muted hint">{{ $t("experiments.requiresControlHint") }}</p>
      </div>

      <div class="actions">
        <NButton type="primary" attr-type="submit" :loading="saving" :disabled="saving" data-test="experiment-save">
          {{ $t("common.save") }}
        </NButton>
        <RouterLink class="link" to="/experiments">{{ $t("common.cancel") }}</RouterLink>
      </div>
    </form>
  </NCard>
</template>

<style scoped>
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; align-items: start; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.wide { grid-column: 1 / -1; }
.hint { margin: 6px 0 0 24px; font-size: 12px; }
.actions { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; }
</style>
