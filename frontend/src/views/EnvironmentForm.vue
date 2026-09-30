<script setup>
// A named apparatus, created from a paradigm template. The paradigm decides
// which physical values exist: the form reads them from the published version
// manifest rather than hard coding a list, so a new paradigm needs no change
// here. The API stores whatever values are sent, so the limits the manifest
// declares are applied as input constraints on this side.
import { computed, h, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NSelect } from "naive-ui";
import { environments, paradigms } from "../api/endpoints.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const catalogue = ref([]);
const manifest = ref(null);
const form = ref({ name: "", paradigmKey: "", notes: "" });
const values = ref({});
const error = ref("");
const saving = ref(false);
const loadingManifest = ref(false);

const parameters = computed(() => manifest.value?.apparatusParameters || []);
const chosen = computed(() => catalogue.value.find((p) => p.key === form.value.paradigmKey) || null);
// The label carries a stable test hook: Naive UI's dropdown options are not a
// native <select>, so the browser suite can no longer address them by value.
const paradigmOptions = computed(() =>
  catalogue.value.map((p) => ({ value: p.key, label: () => h("span", { "data-test": `environment-paradigm-${p.key}` }, p.name) })),
);

async function loadManifest(key) {
  manifest.value = null;
  values.value = {};
  if (!key) return;
  const summary = catalogue.value.find((p) => p.key === key);
  if (!summary) return;
  loadingManifest.value = true;
  try {
    manifest.value = await paradigms.version(key, summary.latestVersion);
    // Start from what the paradigm considers normal; the laboratory corrects
    // the ones that differ for its own apparatus.
    for (const parameter of manifest.value.apparatusParameters || []) {
      values.value[parameter.key] = parameter.default;
    }
  } catch (e) {
    error.value = e.message;
  } finally {
    loadingManifest.value = false;
  }
}

watch(() => form.value.paradigmKey, loadManifest);

async function save() {
  error.value = "";
  saving.value = true;
  try {
    const apparatus = {};
    for (const parameter of parameters.value) {
      apparatus[parameter.key] = Number(values.value[parameter.key]);
    }
    // The new environment opens on its own page, where the calibration every
    // video of it will use is entered once.
    const created = await environments.create({
      name: form.value.name.trim(),
      paradigmKey: form.value.paradigmKey,
      notes: form.value.notes.trim(),
      revision: {
        paradigmVersion: chosen.value?.latestVersion || 1,
        apparatus,
        notes: "",
      },
    });
    router.push(`/environments/${created.environment.id}`);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  crumb.set([{ label: t("environments.title"), to: "/environments" }, { label: t("environments.new") }]);
  try {
    catalogue.value = (await paradigms.list()).data || [];
    // Arriving from a paradigm page preselects it.
    const preset = route.query.paradigm;
    if (preset && catalogue.value.some((p) => p.key === preset)) form.value.paradigmKey = preset;
  } catch (e) {
    error.value = e.message;
  }
});
onUnmounted(() => crumb.clear());
</script>

<template>
  <PageHead :title="$t('environments.new')" />

  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small">
    <form class="form" data-test="environment-form" @submit.prevent="save">
      <label class="fld">
        <span>{{ $t("common.name") }}</span>
        <input
          v-model="form.name"
          data-test="environment-name"
          :placeholder="$t('environments.namePlaceholder')"
          required
        />
      </label>

      <label class="fld">
        <span>{{ $t("environments.paradigm") }}</span>
        <NSelect
          v-model:value="form.paradigmKey"
          data-test="environment-paradigm"
          :placeholder="$t('environments.pickParadigm')"
          :options="paradigmOptions"
        />
        <small class="muted">{{ $t("environments.paradigmLocked") }}</small>
      </label>

      <label class="fld wide">
        <span>{{ $t("common.notes") }}</span>
        <textarea v-model="form.notes" rows="2" data-test="environment-notes"></textarea>
      </label>

      <div class="wide" v-if="parameters.length">
        <h4>{{ $t("environments.apparatus") }}</h4>
        <div class="params">
          <label class="fld" v-for="parameter in parameters" :key="parameter.key">
            <span>{{ parameter.label }} <span class="muted">{{ parameter.unit }}</span></span>
            <input
              type="number"
              step="any"
              :min="parameter.min"
              :max="parameter.max"
              v-model="values[parameter.key]"
              :data-test="`apparatus-${parameter.key}`"
              required
            />
            <small class="muted">{{ parameter.min }} - {{ parameter.max }}</small>
          </label>
        </div>
      </div>
      <p class="muted wide" v-else-if="loadingManifest">{{ $t("common.loading") }}</p>

      <div class="actions">
        <NButton type="primary" attr-type="submit" :loading="saving" :disabled="!parameters.length" data-test="environment-save">
          {{ $t("common.save") }}
        </NButton>
        <RouterLink class="link" to="/environments">{{ $t("common.cancel") }}</RouterLink>
      </div>
    </form>
  </NCard>
</template>

<style scoped>
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; align-items: start; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.fld :deep(.n-select) { width: 100%; }
.params { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; }
.wide { grid-column: 1 / -1; }
.actions { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; }
h4 { margin: 0 0 8px; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
</style>
