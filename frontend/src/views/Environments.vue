<script setup>
import { ref, reactive, onMounted, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";

const { locale, t } = useI18n();
const route = useRoute();

const items = ref([]); // environment instances
const paradigms = ref([]); // paradigm summaries for the dropdown
const err = ref("");

// Editor state. `editing` is null when closed, otherwise the working record.
// `spec` holds the loaded paradigm detail (apparatusParameters); `apparatus`
// is the editable value map rendered in the form.
const editing = ref(null);
const spec = ref(null);
const apparatus = reactive({});
const saving = ref(false);

function paradigmName(key) {
  const p = paradigms.value.find((x) => x.key === key);
  return p ? p.name : key;
}

async function load() {
  err.value = "";
  try {
    const [envs, paras] = await Promise.all([
      api("/environments"),
      api(`/paradigms?lang=${locale.value}`),
    ]);
    items.value = envs;
    paradigms.value = paras;
  } catch (e) {
    err.value = e.message;
  }
}

function clearApparatus() {
  for (const k of Object.keys(apparatus)) delete apparatus[k];
}

// Load a paradigm spec and seed the apparatus form. `values` overlays defaults
// (used when editing an existing environment).
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

function startCreate(key = "") {
  err.value = "";
  editing.value = { name: "", paradigmKey: key, notes: "" };
  loadSpec(key);
}

async function startEdit(env) {
  err.value = "";
  editing.value = {
    id: env.id,
    name: env.name,
    paradigmKey: env.paradigmKey,
    notes: env.notes || "",
  };
  await loadSpec(env.paradigmKey, env.config?.apparatus ?? {});
}

function onParadigmChange() {
  loadSpec(editing.value.paradigmKey);
}

function cancel() {
  editing.value = null;
  spec.value = null;
  clearApparatus();
}

async function save() {
  if (!editing.value) return;
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
  const body = { name: editing.value.name, apparatus: ap, notes: editing.value.notes || null };
  try {
    if (editing.value.id) {
      await api(`/environments/${editing.value.id}`, { method: "PATCH", body });
    } else {
      await api("/environments", {
        method: "POST",
        body: { ...body, paradigmKey: editing.value.paradigmKey },
      });
    }
    cancel();
    await load();
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove(env) {
  if (!confirm(t("environments.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/environments/${env.id}`, { method: "DELETE" });
    await load();
  } catch (e) {
    err.value = e.message;
  }
}

// Re-fetch localized paradigm labels when the language changes.
watch(locale, load);

onMounted(async () => {
  await load();
  // Deep link from a paradigm detail page: ?paradigm=KEY opens the create form.
  if (route.query.paradigm) startCreate(String(route.query.paradigm));
});
</script>

<template>
  <div class="head">
    <h1>{{ $t("environments.title") }}</h1>
    <button class="primary" v-if="!editing" @click="startCreate()">{{ $t("environments.new") }}</button>
  </div>
  <p class="muted" style="margin-top:0">{{ $t("environments.intro") }}</p>
  <p class="err" v-if="err">{{ err }}</p>

  <!-- Editor -->
  <div class="card" v-if="editing">
    <div class="row">
      <div class="field">
        <label>{{ $t("common.name") }}</label>
        <input v-model="editing.name" :placeholder="$t('environments.namePlaceholder')" />
      </div>
      <div class="field">
        <label>{{ $t("environments.paradigm") }}</label>
        <select v-model="editing.paradigmKey" :disabled="!!editing.id" @change="onParadigmChange">
          <option value="" disabled>{{ $t("environments.pickParadigm") }}</option>
          <option v-for="p in paradigms" :key="p.key" :value="p.key">{{ p.name }}</option>
        </select>
      </div>
      <div class="field" style="flex:2">
        <label>{{ $t("common.notes") }}</label>
        <input v-model="editing.notes" />
      </div>
    </div>
    <p class="muted hint" v-if="editing.id">{{ $t("environments.paradigmLocked") }}</p>

    <!-- Apparatus values for the selected paradigm -->
    <div v-if="spec">
      <h3>{{ $t("environments.apparatus") }}</h3>
      <table>
        <thead><tr><th>{{ $t("paradigms.field") }}</th><th>{{ $t("paradigms.unit") }}</th><th>{{ $t("paradigms.range") }}</th><th>{{ $t("paradigms.value") }}</th></tr></thead>
        <tbody>
          <tr v-for="f in spec.apparatusParameters" :key="f.key">
            <td>{{ f.label }} <span class="muted">{{ f.key }}</span></td>
            <td><span class="pill">{{ f.unit }}</span></td>
            <td class="muted">{{ f.min != null ? f.min + " - " + f.max : (f.options ? f.options.join(" / ") : "-") }}</td>
            <td>
              <select v-if="f.type === 'enum'" v-model="apparatus[f.key]">
                <option v-for="o in f.options" :key="o" :value="o">{{ o }}</option>
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

    <div class="actions">
      <button @click="cancel">{{ $t("common.cancel") }}</button>
      <button
        class="primary"
        :disabled="saving || !editing.name || !editing.paradigmKey"
        @click="save"
      >
        {{ $t("common.save") }}
      </button>
    </div>
  </div>

  <!-- List -->
  <div class="card">
    <table>
      <thead><tr><th>{{ $t("common.name") }}</th><th>{{ $t("environments.paradigm") }}</th><th>{{ $t("common.notes") }}</th><th></th></tr></thead>
      <tbody>
        <tr v-for="e in items" :key="e.id">
          <td>{{ e.name }}</td>
          <td><span class="pill">{{ paradigmName(e.paradigmKey) }}</span></td>
          <td class="muted">{{ e.notes }}</td>
          <td class="row-actions">
            <button @click="startEdit(e)">{{ $t("common.edit") }}</button>
            <button class="danger" @click="remove(e)">{{ $t("common.delete") }}</button>
          </td>
        </tr>
      </tbody>
    </table>
    <p v-if="!items.length" class="muted">{{ $t("environments.empty") }}</p>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.hint { margin: 4px 0 0; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
.row-actions { display: flex; gap: 8px; }
.num { width: 120px; }
h3 { margin: 16px 0 8px; }
</style>
