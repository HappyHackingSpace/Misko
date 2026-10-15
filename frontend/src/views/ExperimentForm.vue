<script setup>
// One experiment, created or edited: its identity and the study question. The
// API owns the rules (code shape, lengths, what may change once the study
// runs); the form sends what changed and shows the API's answer.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NInput, NSwitch, useMessage } from "naive-ui";
import { experiments } from "../api/experiments.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();
const message = useMessage();

const isNew = computed(() => !route.params.id);
const empty = () => ({ code: "", title: "", description: "", hypothesis: "", randomisation: "", blinded: false });
const form = ref(empty());
const original = ref(empty());
const status = ref("DRAFT");
const error = ref("");
const saving = ref(false);

// Hypothesis, randomisation and blinding are part of the design: fixed once
// the study runs.
const designFrozen = computed(() => status.value !== "DRAFT");

function syncCrumb() {
  crumb.set([
    { label: t("nav.experiments"), to: "/experiments" },
    ...(isNew.value ? [] : [{ label: original.value.code || route.params.id, to: `/experiments/${route.params.id}` }]),
    { label: isNew.value ? t("design.form.new") : t("design.form.edit") },
  ]);
}

async function load() {
  error.value = "";
  if (!isNew.value) {
    try {
      const e = await experiments.get(route.params.id);
      const values = {
        code: e.code,
        title: e.title,
        description: e.description || "",
        hypothesis: e.hypothesis || "",
        randomisation: e.randomisation || "",
        blinded: !!e.blinded,
      };
      form.value = { ...values };
      original.value = { ...values };
      status.value = e.status;
    } catch (e) {
      error.value = e.message;
    }
  }
  syncCrumb();
}

function body() {
  const values = {
    code: form.value.code.trim(),
    title: form.value.title.trim(),
    description: form.value.description.trim(),
    hypothesis: form.value.hypothesis.trim(),
    randomisation: form.value.randomisation.trim(),
    blinded: form.value.blinded,
  };
  if (isNew.value) {
    // Optional text left empty is simply not sent.
    return Object.fromEntries(Object.entries(values).filter(([, v]) => v !== ""));
  }
  // An edit sends only what changed; an emptied text clears the field.
  return Object.fromEntries(Object.entries(values).filter(([k, v]) => v !== original.value[k]));
}

async function save() {
  error.value = "";
  const payload = body();
  if (!isNew.value && !Object.keys(payload).length) {
    router.push(`/experiments/${route.params.id}`);
    return;
  }
  saving.value = true;
  try {
    if (isNew.value) {
      const created = await experiments.create(payload);
      message.success(t("design.form.created"));
      // A new study continues with its groups.
      router.push({ path: `/experiments/${created.id}`, query: { tab: "design" } });
      return;
    }
    await experiments.update(route.params.id, payload);
    message.success(t("design.form.saved"));
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
  <PageHead :title="isNew ? $t('design.form.new') : $t('design.form.edit')" />

  <NCard :bordered="true" size="small">
    <form class="form" data-test="experiment-form" @submit.prevent="save">
      <label class="fld">
        <span>{{ $t("design.form.code") }}</span>
        <NInput v-model:value="form.code" :maxlength="64" :placeholder="$t('design.form.codePlaceholder')" :input-props="{ 'data-test': 'experiment-code', required: true }" />
      </label>

      <label class="fld">
        <span>{{ $t("design.form.title") }}</span>
        <NInput v-model:value="form.title" :maxlength="200" :input-props="{ 'data-test': 'experiment-title', required: true }" />
      </label>

      <label class="fld wide">
        <span>{{ $t("design.form.hypothesis") }}</span>
        <NInput
          v-model:value="form.hypothesis"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 6 }"
          :maxlength="2000"
          :disabled="designFrozen"
          :placeholder="$t('design.form.hypothesisPlaceholder')"
          :input-props="{ 'data-test': 'experiment-hypothesis' }"
        />
      </label>

      <label class="fld wide">
        <span>{{ $t("design.form.randomisation") }}</span>
        <NInput
          v-model:value="form.randomisation"
          :maxlength="500"
          :disabled="designFrozen"
          :placeholder="$t('design.form.randomisationPlaceholder')"
          :input-props="{ 'data-test': 'experiment-randomisation' }"
        />
      </label>

      <div class="fld wide">
        <span>{{ $t("design.form.blinded") }}</span>
        <div class="switch">
          <NSwitch v-model:value="form.blinded" :disabled="designFrozen" data-test="experiment-blinded" />
          <small class="muted">{{ $t("design.form.blindedHint") }}</small>
        </div>
      </div>

      <label class="fld wide">
        <span>{{ $t("common.description") }}</span>
        <NInput v-model:value="form.description" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }" :maxlength="5000" :input-props="{ 'data-test': 'experiment-description' }" />
      </label>

      <NAlert v-if="designFrozen" class="wide" type="info" :title="$t('design.form.frozenHint')" />
      <NAlert v-if="error" class="wide" type="error" :title="error" data-test="error" />

      <div class="actions">
        <NButton type="primary" attr-type="submit" :loading="saving" :disabled="saving" data-test="experiment-save">
          {{ $t("common.save") }}
        </NButton>
        <RouterLink class="link" :to="isNew ? '/experiments' : `/experiments/${route.params.id}`">{{ $t("common.cancel") }}</RouterLink>
      </div>
    </form>
  </NCard>
</template>

<style scoped>
.form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; align-items: start; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.fld > span { font-size: 12px; color: var(--muted); }
.wide { grid-column: 1 / -1; }
.switch { display: flex; align-items: center; gap: 10px; min-height: 30px; }
.actions { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; }
@media (max-width: 640px) {
  .form { grid-template-columns: minmax(0, 1fr); }
}
</style>
