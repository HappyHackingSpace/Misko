<script setup>
// A named apparatus, created from a paradigm template. The paradigm decides
// which physical values exist: the form reads them from the published version
// manifest rather than hard coding a list, so a new paradigm needs no change
// here. The API stores whatever values are sent, so the limits the manifest
// declares are applied as input constraints on this side.
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { environments, paradigms } from "../api/endpoints.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

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
    void created;
    router.push("/environments");
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
  <div class="head">
    <h1>{{ $t("environments.new") }}</h1>
  </div>

  <p class="err" v-if="error" data-test="error">{{ error }}</p>

  <form class="card form" data-test="environment-form" @submit.prevent="save">
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
      <select v-model="form.paradigmKey" data-test="environment-paradigm" required>
        <option value="">{{ $t("environments.pickParadigm") }}</option>
        <option v-for="p in catalogue" :key="p.key" :value="p.key">{{ p.name }}</option>
      </select>
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
      <button type="submit" :disabled="saving || !parameters.length" data-test="environment-save">
        {{ $t("common.save") }}
      </button>
      <RouterLink class="link" to="/environments">{{ $t("common.cancel") }}</RouterLink>
    </div>
  </form>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; align-items: start; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.params { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; }
.wide { grid-column: 1 / -1; }
.actions { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; }
h4 { margin: 0 0 8px; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
</style>
