<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const isNew = computed(() => !route.params.id);

const emptyForm = () => ({
  code: "",
  microchipId: "", earTag: "",
  species: "Mus musculus", strain: "", line: "", genotype: "", zygosity: "", sex: "M",
  birthDate: "", coatColor: "",
  cageId: "", litter: "", cohort: "",
  status: "ALIVE", acquiredAt: "", sacrificedAt: "", healthStatus: "",
  groupName: "", notes: "",
});

const form = ref(emptyForm());
const err = ref("");
const saving = ref(false);

// Prisma returns dates as ISO strings; <input type="date"> needs "YYYY-MM-DD".
const toDateInput = (v) => (v ? String(v).slice(0, 10) : "");

function syncCrumb() {
  crumb.set([
    { label: t("nav.dashboard"), to: "/" },
    { label: t("subjects.title"), to: "/subjects" },
    { label: isNew.value ? t("common.new") : form.value.code || route.params.id },
  ]);
}

async function load() {
  err.value = "";
  if (isNew.value) {
    syncCrumb();
    return;
  }
  try {
    const s = await api(`/subjects/${route.params.id}`);
    form.value = {
      code: s.code ?? "",
      microchipId: s.microchipId ?? "", earTag: s.earTag ?? "",
      species: s.species ?? "", strain: s.strain ?? "", line: s.line ?? "",
      genotype: s.genotype ?? "", zygosity: s.zygosity ?? "", sex: s.sex ?? "M",
      birthDate: toDateInput(s.birthDate), coatColor: s.coatColor ?? "",
      cageId: s.cageId ?? "", litter: s.litter ?? "", cohort: s.cohort ?? "",
      status: s.status ?? "ALIVE", acquiredAt: toDateInput(s.acquiredAt),
      sacrificedAt: toDateInput(s.sacrificedAt), healthStatus: s.healthStatus ?? "",
      groupName: s.groupName ?? "", notes: s.notes ?? "",
    };
    await Promise.all([loadWeights(), loadDiseases(), loadTreatments()]);
  } catch (e) {
    err.value = e.message;
  }
  syncCrumb();
}

async function save() {
  err.value = "";
  saving.value = true;
  try {
    if (isNew.value) {
      const created = await api("/subjects", { method: "POST", body: { ...form.value } });
      router.push(`/subjects/${created.id}`);
    } else {
      await api(`/subjects/${route.params.id}`, { method: "PATCH", body: { ...form.value } });
      router.push("/subjects");
    }
  } catch (e) {
    err.value = e.message;
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!confirm(t("subjects.confirmDelete"))) return;
  err.value = "";
  try {
    await api(`/subjects/${route.params.id}`, { method: "DELETE" });
    router.push("/subjects");
  } catch (e) {
    err.value = e.message;
  }
}

// --- Weight log panel (only when editing an existing subject) ---
const weights = ref([]);
const newWeight = ref({ grams: "", measuredAt: "", notes: "" });
const weightErr = ref("");

async function loadWeights() {
  weights.value = await api(`/subjects/${route.params.id}/weights`);
}

async function addWeight() {
  weightErr.value = "";
  try {
    await api(`/subjects/${route.params.id}/weights`, { method: "POST", body: { ...newWeight.value } });
    newWeight.value = { grams: "", measuredAt: "", notes: "" };
    await loadWeights();
  } catch (e) {
    weightErr.value = e.message;
  }
}

async function removeWeight(id) {
  if (!confirm(t("weights.confirmDelete"))) return;
  weightErr.value = "";
  try {
    await api(`/subjects/${route.params.id}/weights/${id}`, { method: "DELETE" });
    await loadWeights();
  } catch (e) {
    weightErr.value = e.message;
  }
}

const fmtDate = (v) => (v ? String(v).slice(0, 10) : "");

// --- Disease model assignments (docs/DOMAIN.md §3) ---
const diseaseCatalog = ref([]);
const diseaseLinks = ref([]);
const newDisease = ref({ diseaseModelId: "", inducedAt: "", method: "", notes: "" });
const diseaseErr = ref("");

async function loadDiseases() {
  const [cat, links] = await Promise.all([
    api("/disease-models?all=true"),
    api(`/subjects/${route.params.id}/disease-models`),
  ]);
  diseaseCatalog.value = cat.data;
  diseaseLinks.value = links;
}

async function attachDisease() {
  diseaseErr.value = "";
  try {
    await api(`/subjects/${route.params.id}/disease-models`, { method: "POST", body: { ...newDisease.value } });
    newDisease.value = { diseaseModelId: "", inducedAt: "", method: "", notes: "" };
    await loadDiseases();
  } catch (e) {
    diseaseErr.value = e.message;
  }
}

async function detachDisease(id) {
  if (!confirm(t("subjectLinks.confirmRemove"))) return;
  diseaseErr.value = "";
  try {
    await api(`/subjects/${route.params.id}/disease-models/${id}`, { method: "DELETE" });
    await loadDiseases();
  } catch (e) {
    diseaseErr.value = e.message;
  }
}

// --- Treatment assignments (docs/DOMAIN.md §3) ---
const TREATMENT_ROUTES = ["IP", "ORAL", "SC", "IV", "IN"];
const treatmentCatalog = ref([]);
const treatmentLinks = ref([]);
const newTreatment = ref({ treatmentId: "", dose: "", unit: "", route: "", startedAt: "", endedAt: "" });
const treatmentErr = ref("");

async function loadTreatments() {
  const [cat, links] = await Promise.all([
    api("/treatments?all=true"),
    api(`/subjects/${route.params.id}/treatments`),
  ]);
  treatmentCatalog.value = cat.data;
  treatmentLinks.value = links;
}

async function attachTreatment() {
  treatmentErr.value = "";
  try {
    await api(`/subjects/${route.params.id}/treatments`, { method: "POST", body: { ...newTreatment.value } });
    newTreatment.value = { treatmentId: "", dose: "", unit: "", route: "", startedAt: "", endedAt: "" };
    await loadTreatments();
  } catch (e) {
    treatmentErr.value = e.message;
  }
}

async function detachTreatment(id) {
  if (!confirm(t("subjectLinks.confirmRemove"))) return;
  treatmentErr.value = "";
  try {
    await api(`/subjects/${route.params.id}/treatments/${id}`, { method: "DELETE" });
    await loadTreatments();
  } catch (e) {
    treatmentErr.value = e.message;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <div class="card">
    <h2 style="margin-top:0">{{ isNew ? $t("common.new") : form.code }}</h2>

    <h3 class="section">{{ $t("subjects.sectionIdentity") }}</h3>
    <div class="row">
      <div class="field"><label>{{ $t("subjects.code") }}</label><input v-model="form.code" :placeholder="$t('subjects.codePlaceholder')" /></div>
      <div class="field"><label>{{ $t("subjects.microchipId") }}</label><input v-model="form.microchipId" /></div>
      <div class="field"><label>{{ $t("subjects.earTag") }}</label><input v-model="form.earTag" /></div>
    </div>

    <h3 class="section">{{ $t("subjects.sectionBiological") }}</h3>
    <div class="row">
      <div class="field"><label>{{ $t("subjects.species") }}</label><input v-model="form.species" /></div>
      <div class="field"><label>{{ $t("subjects.strain") }}</label><input v-model="form.strain" placeholder="C57BL/6J" /></div>
      <div class="field"><label>{{ $t("subjects.line") }}</label><input v-model="form.line" placeholder="5xFAD" /></div>
      <div class="field"><label>{{ $t("subjects.genotype") }}</label><input v-model="form.genotype" /></div>
    </div>
    <div class="row">
      <div class="field"><label>{{ $t("subjects.zygosity") }}</label>
        <select v-model="form.zygosity">
          <option value="">{{ $t("subjects.none") }}</option>
          <option value="WT">{{ $t("subjects.zygosityWT") }}</option>
          <option value="HET">{{ $t("subjects.zygosityHET") }}</option>
          <option value="HOMO">{{ $t("subjects.zygosityHOMO") }}</option>
        </select>
      </div>
      <div class="field"><label>{{ $t("subjects.sex") }}</label>
        <select v-model="form.sex"><option value="M">{{ $t("subjects.male") }}</option><option value="F">{{ $t("subjects.female") }}</option></select>
      </div>
      <div class="field"><label>{{ $t("subjects.coatColor") }}</label><input v-model="form.coatColor" /></div>
      <div class="field"><label>{{ $t("subjects.birthDate") }}</label><input type="date" v-model="form.birthDate" /></div>
    </div>

    <h3 class="section">{{ $t("subjects.sectionHousing") }}</h3>
    <div class="row">
      <div class="field"><label>{{ $t("subjects.cageId") }}</label><input v-model="form.cageId" /></div>
      <div class="field"><label>{{ $t("subjects.litter") }}</label><input v-model="form.litter" /></div>
      <div class="field"><label>{{ $t("subjects.cohort") }}</label><input v-model="form.cohort" /></div>
      <div class="field"><label>{{ $t("subjects.group") }}</label><input v-model="form.groupName" :placeholder="$t('subjects.groupPlaceholder')" /></div>
    </div>

    <h3 class="section">{{ $t("subjects.sectionLifecycle") }}</h3>
    <div class="row">
      <div class="field"><label>{{ $t("subjects.status") }}</label>
        <select v-model="form.status">
          <option value="ALIVE">{{ $t("subjects.statusALIVE") }}</option>
          <option value="SACRIFICED">{{ $t("subjects.statusSACRIFICED") }}</option>
          <option value="DEAD">{{ $t("subjects.statusDEAD") }}</option>
        </select>
      </div>
      <div class="field"><label>{{ $t("subjects.acquiredAt") }}</label><input type="date" v-model="form.acquiredAt" /></div>
      <div class="field"><label>{{ $t("subjects.sacrificedAt") }}</label><input type="date" v-model="form.sacrificedAt" /></div>
      <div class="field"><label>{{ $t("subjects.healthStatus") }}</label><input v-model="form.healthStatus" /></div>
    </div>
    <div class="row">
      <div class="field" style="flex:1"><label>{{ $t("common.notes") }}</label><input v-model="form.notes" /></div>
    </div>

    <p class="err" v-if="err">{{ err }}</p>
    <div class="actions">
      <button @click="router.push('/subjects')">{{ $t("common.back") }}</button>
      <button v-if="!isNew" class="danger" @click="remove">{{ $t("common.delete") }}</button>
      <button class="primary" :disabled="saving || !form.code" @click="save">{{ isNew ? $t("common.create") : $t("common.save") }}</button>
    </div>
  </div>

  <!-- Weight time series: available once the subject exists -->
  <div class="card" v-if="!isNew">
    <h3 class="section" style="margin-top:0">{{ $t("weights.title") }}</h3>
    <table class="weights">
      <thead>
        <tr><th>{{ $t("weights.measuredAt") }}</th><th>{{ $t("weights.grams") }}</th><th>{{ $t("common.notes") }}</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="w in weights" :key="w.id">
          <td>{{ fmtDate(w.measuredAt) }}</td>
          <td>{{ w.grams }}</td>
          <td class="muted">{{ w.notes }}</td>
          <td><button class="danger small" @click="removeWeight(w.id)">{{ $t("common.delete") }}</button></td>
        </tr>
        <tr v-if="!weights.length"><td colspan="4" class="muted">{{ $t("weights.empty") }}</td></tr>
      </tbody>
      <tfoot>
        <tr>
          <td><input type="date" v-model="newWeight.measuredAt" /></td>
          <td><input type="number" step="0.1" min="0" v-model="newWeight.grams" :placeholder="$t('weights.gramsPlaceholder')" /></td>
          <td><input v-model="newWeight.notes" /></td>
          <td><button class="primary small" :disabled="!newWeight.grams" @click="addWeight">{{ $t("weights.add") }}</button></td>
        </tr>
      </tfoot>
    </table>
    <p class="err" v-if="weightErr">{{ weightErr }}</p>
  </div>

  <!-- Disease model assignments -->
  <div class="card" v-if="!isNew">
    <h3 class="section" style="margin-top:0">{{ $t("subjectLinks.diseaseModels") }}</h3>
    <table class="links">
      <thead>
        <tr><th>{{ $t("diseaseModels.title") }}</th><th>{{ $t("subjectLinks.inducedAt") }}</th><th>{{ $t("subjectLinks.method") }}</th><th>{{ $t("common.notes") }}</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="l in diseaseLinks" :key="l.id">
          <td>{{ l.diseaseModel.name }} <span class="muted">({{ l.diseaseModel.key }})</span></td>
          <td>{{ fmtDate(l.inducedAt) }}</td>
          <td class="muted">{{ l.method }}</td>
          <td class="muted">{{ l.notes }}</td>
          <td><button class="danger small" @click="detachDisease(l.id)">{{ $t("common.delete") }}</button></td>
        </tr>
        <tr v-if="!diseaseLinks.length"><td colspan="5" class="muted">{{ $t("subjectLinks.emptyDisease") }}</td></tr>
      </tbody>
      <tfoot>
        <tr>
          <td>
            <select v-model="newDisease.diseaseModelId">
              <option value="">{{ $t("subjectLinks.selectDisease") }}</option>
              <option v-for="d in diseaseCatalog" :key="d.id" :value="d.id">{{ d.name }} ({{ d.key }})</option>
            </select>
          </td>
          <td><input type="date" v-model="newDisease.inducedAt" /></td>
          <td><input v-model="newDisease.method" /></td>
          <td><input v-model="newDisease.notes" /></td>
          <td><button class="primary small" :disabled="!newDisease.diseaseModelId" @click="attachDisease">{{ $t("subjectLinks.add") }}</button></td>
        </tr>
      </tfoot>
    </table>
    <p class="err" v-if="diseaseErr">{{ diseaseErr }}</p>
  </div>

  <!-- Treatment assignments -->
  <div class="card" v-if="!isNew">
    <h3 class="section" style="margin-top:0">{{ $t("subjectLinks.treatments") }}</h3>
    <table class="links">
      <thead>
        <tr><th>{{ $t("treatments.title") }}</th><th>{{ $t("treatments.defaultDose") }}</th><th>{{ $t("treatments.route") }}</th><th>{{ $t("subjectLinks.startedAt") }}</th><th>{{ $t("subjectLinks.endedAt") }}</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="l in treatmentLinks" :key="l.id">
          <td>{{ l.treatment.name }} <span class="muted">({{ l.treatment.key }})</span></td>
          <td>{{ l.dose != null ? l.dose : "" }} <span class="muted">{{ l.unit }}</span></td>
          <td class="muted">{{ l.route }}</td>
          <td>{{ fmtDate(l.startedAt) }}</td>
          <td>{{ fmtDate(l.endedAt) }}</td>
          <td><button class="danger small" @click="detachTreatment(l.id)">{{ $t("common.delete") }}</button></td>
        </tr>
        <tr v-if="!treatmentLinks.length"><td colspan="6" class="muted">{{ $t("subjectLinks.emptyTreatment") }}</td></tr>
      </tbody>
      <tfoot>
        <tr>
          <td>
            <select v-model="newTreatment.treatmentId">
              <option value="">{{ $t("subjectLinks.selectTreatment") }}</option>
              <option v-for="tx in treatmentCatalog" :key="tx.id" :value="tx.id">{{ tx.name }} ({{ tx.key }})</option>
            </select>
          </td>
          <td><input type="number" step="0.01" min="0" v-model="newTreatment.dose" /></td>
          <td>
            <select v-model="newTreatment.route">
              <option value="">{{ $t("treatments.none") }}</option>
              <option v-for="r in TREATMENT_ROUTES" :key="r" :value="r">{{ r }}</option>
            </select>
          </td>
          <td><input type="date" v-model="newTreatment.startedAt" /></td>
          <td><input type="date" v-model="newTreatment.endedAt" /></td>
          <td><button class="primary small" :disabled="!newTreatment.treatmentId" @click="attachTreatment">{{ $t("subjectLinks.add") }}</button></td>
        </tr>
      </tfoot>
    </table>
    <p class="err" v-if="treatmentErr">{{ treatmentErr }}</p>
  </div>
</template>

<style scoped>
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 14px; }
.section { font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); margin: 18px 0 8px; }
.weights, .links { width: 100%; border-collapse: collapse; }
.weights th, .weights td, .links th, .links td { padding: 6px 8px; text-align: left; border-bottom: 1px solid var(--line); }
.weights input, .links input, .links select { width: 100%; }
.small { padding: 4px 10px; font-size: 0.85rem; }
</style>
