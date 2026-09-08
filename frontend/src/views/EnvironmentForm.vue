<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { locale, t, te } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();
// Localize a parameter enum value (N/E/S/W, opaque/clear, ...); raw fallback.
const tVal = (value) => (te(`paramValues.${value}`) ? t(`paramValues.${value}`) : value);

const isNew = computed(() => !route.params.id);
const paradigms = ref([]); // paradigm summaries for the dropdown and label lookup
const spec = ref(null); // loaded paradigm spec (apparatus parameters)
const apparatus = reactive({});
const form = ref({ name: "", paradigmKey: "", notes: "" });
const err = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("environments.title"), to: "/environments" },
    { label: isNew.value ? t("common.new") : form.value.name || route.params.id },
  ]);
}

function clearApparatus() {
  for (const k of Object.keys(apparatus)) delete apparatus[k];
}

// Load a paradigm spec and seed the apparatus form. `values` overlays defaults.
async function loadSpec(key, values = {}) {
  spec.value = null;
  clearApparatus();
  if (!key) return;
  try {
    spec.value = await api(`/paradigms/${key}?lang=${locale.value}`);
    for (const f of spec.value.apparatusParameters) {
      const v = values[f.key];
      apparatus[f.key] = v ?? f.default ?? "";
    }
  } catch (e) {
    err.value = e.message;
  }
}

async function loadParadigms() {
  try {
    const res = await api(`/paradigms?all=true&lang=${locale.value}`);
    paradigms.value = res.data;
  } catch (e) {
    err.value = e.message;
  }
}

async function load() {
  err.value = "";
  await loadParadigms();
  if (isNew.value) {
    const key = route.query.paradigm ? String(route.query.paradigm) : "";
    form.value = { name: "", paradigmKey: key, notes: "" };
    await loadSpec(key);
  } else {
    try {
      const env = await api(`/environments/${route.params.id}`);
      form.value = { name: env.name, paradigmKey: env.paradigmKey, notes: env.notes || "" };
      await loadSpec(env.paradigmKey, env.config?.apparatus ?? {});
    } catch (e) {
      err.value = e.message;
    }
  }
  syncCrumb();
}

function onParadigmChange() {
  loadSpec(form.value.paradigmKey);
}

async function save() {
  err.value = "";
  saving.value = true;
  // Send only apparatus keys defined by the spec, dropping empty values.
  const ap = {};
  if (spec.value) {
    for (const f of spec.value.apparatusParameters) {
      const v = apparatus[f.key];
      if (v !== "" && v != null) ap[f.key] = v;
    }
  }
  const body = { name: form.value.name, apparatus: ap, notes: form.value.notes || null };
  try {
    if (isNew.value) {
      await api("/environments", { method: "POST", body: { ...body, paradigmKey: form.value.paradigmKey } });
    } else {
      await api(`/environments/${route.params.id}`, { method: "PATCH", body });
    }
    router.push("/environments");
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!confirm(t("environments.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/environments/${route.params.id}`, { method: "DELETE" });
    router.push("/environments");
  } catch (e) {
    err.value = e.message;
  }
}

// Re-fetch localized paradigm dropdown labels when the language changes.
watch(locale, loadParadigms);

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <div class="card">
    <h2 style="margin-top:0">{{ isNew ? $t("common.new") : form.name }}</h2>
    <div class="row">
      <div class="field">
        <label>{{ $t("common.name") }}</label>
        <input v-model="form.name" :placeholder="$t('environments.namePlaceholder')" />
      </div>
      <div class="field">
        <label>{{ $t("environments.paradigm") }}</label>
        <select v-model="form.paradigmKey" :disabled="!isNew" @change="onParadigmChange">
          <option value="" disabled>{{ $t("environments.pickParadigm") }}</option>
          <option v-for="p in paradigms" :key="p.key" :value="p.key">{{ p.name }}</option>
        </select>
      </div>
      <div class="field" style="flex:2">
        <label>{{ $t("common.notes") }}</label>
        <input v-model="form.notes" />
      </div>
    </div>
    <p class="muted hint" v-if="!isNew">{{ $t("environments.paradigmLocked") }}</p>

    <!-- Apparatus values for the selected paradigm -->
    <div v-if="spec">
      <h3>{{ $t("environments.apparatus") }}</h3>
      <div class="table-scroll">
        <table>
          <thead><tr><th>{{ $t("paradigms.field") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.range") }}</th><th>{{ $t("paradigms.value") }}</th></tr></thead>
          <tbody>
            <tr v-for="f in spec.apparatusParameters" :key="f.key">
              <td>{{ f.label }} <span class="muted">{{ f.key }}</span></td>
              <td><span class="pill">{{ f.unit }}</span></td>
              <td class="muted">{{ f.min != null ? f.min + " - " + f.max : (f.options ? f.options.map(tVal).join(" / ") : "-") }}</td>
              <td>
                <select v-if="f.type === 'enum'" v-model="apparatus[f.key]">
                  <option v-for="o in f.options" :key="o" :value="o">{{ tVal(o) }}</option>
                </select>
                <input
                  v-else
                  type="number"
                  class="num"
                  :min="f.min ?? undefined"
                  :max="f.max ?? undefined"
                  v-model.number="apparatus[f.key]"
                />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/environments')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.name || !form.paradigmKey" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>
</template>

<style scoped>
.hint { margin: 4px 0 0; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; flex-wrap: wrap; }
.num { width: 120px; }
h3 { margin: 16px 0 8px; }
</style>
