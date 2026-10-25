<script setup>
// One arena (environment) with its revisions and, for each revision, the
// default calibration every animal tested in it uses. It is entered once when
// the rig is set up; an animal whose camera or arena moved is calibrated by
// hand from its test. Reading is open to every role, writing needs
// apparatus:write.
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDescriptions, NDescriptionsItem, NForm, NFormItem, NInput, NModal, NSpace, NTag, useMessage } from "naive-ui";
import { environments, paradigmCatalog as paradigms, revisionParameters } from "../api/catalog.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";
import Calibrator from "../components/calibration/Calibrator.vue";
import CalibrationSummary from "../components/calibration/CalibrationSummary.vue";
import ParameterFields from "../components/ParameterFields.vue";
import { defaults, useParameters } from "../composables/useParameters.js";

const { t, locale } = useI18n();
const { format } = useParameters();
const route = useRoute();
const auth = useAuth();
const crumb = useBreadcrumb();
const message = useMessage();
const canWrite = computed(() => auth.can("apparatus:write"));

const environment = ref(null);
const revisions = ref([]);
// Per revision number: the calibration status and the chain of calibrations.
const states = ref({});
const error = ref("");
const loading = ref(true);
const calibrating = ref(0);
const saving = ref(false);

const environmentId = computed(() => route.params.id);
const stateOf = (number) => states.value[number] || { status: "", chain: [] };
// The latest calibration is the one no other calibration supersedes.
const latestOf = (number) => {
  const chain = stateOf(number).chain;
  const superseded = new Set(chain.map((c) => c.supersedesId).filter(Boolean));
  return chain.findLast((c) => !superseded.has(c.id)) || null;
};

// Paradigm manifests by "KEY/version", for labels, units and defaults.
const manifests = ref({});
const manifestOf = (revision) => manifests.value[`${revision.paradigmKey}/${revision.paradigmVersion}`] || null;
const environmentParameters = (m) => (m?.parameters || []).filter((p) => p.scope === "APPARATUS" || p.scope === "ANALYSIS");
// A revision's parameters, labelled; analysis settings it does not store run at their default.
const shownValues = (revision) =>
  environmentParameters(manifestOf(revision)).map((parameter) => ({
    parameter,
    value: revision.parameters?.[parameter.key] ?? parameter.default,
  }));
const paradigmName = computed(() => (latest.value && manifestOf(latest.value)?.name) || environment.value?.paradigmKey || "");

async function loadManifests() {
  const wanted = new Set(revisions.value.map((r) => `${r.paradigmKey}/${r.paradigmVersion}`));
  const loaded = {};
  for (const id of wanted) {
    const [key, version] = id.split("/");
    loaded[id] = await paradigms.version(key, Number(version));
  }
  manifests.value = loaded;
}

async function load() {
  error.value = "";
  try {
    environment.value = await environments.get(environmentId.value);
    revisions.value = (await environments.revisions(environmentId.value)).data || [];
    await loadManifests();
    for (const revision of revisions.value) {
      const status = await environments.calibrationStatus(environmentId.value, revision.number);
      const chain = status.status === "NOT_REQUIRED" ? [] : (await environments.calibrations(environmentId.value, revision.number)).data || [];
      states.value[revision.number] = { status: status.status, current: status.calibration, drift: status.drift, chain };
    }
    crumb.set([{ label: t("environments.title"), to: "/environments" }, { label: environment.value.name }]);
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

async function save(number, body) {
  saving.value = true;
  error.value = "";
  try {
    // A correction names the calibration it replaces, so a rejected attempt is
    // corrected rather than repeated from scratch.
    const stored = await environments.calibrate(environmentId.value, number, { ...body, supersedesId: latestOf(number)?.id || "" });
    calibrating.value = 0;
    if (stored.status === "VALID") message.success(t("environments.calibration.saved"));
    else message.warning(t("environments.calibration.rejected"));
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

// Name and notes are edited in place. Measurements never are: changing them adds
// a revision, so every test keeps the values it was run with.
const editing = ref(false);
const details = ref({ name: "", notes: "" });
const detailsError = ref("");
const revising = ref(false);
const manifest = ref(null);
const values = ref({});
const revisionNotes = ref("");
const showAnalysis = ref(false);
const apparatusParameters = computed(() => (manifest.value?.parameters || []).filter((p) => p.scope === "APPARATUS"));
const analysisParameters = computed(() => (manifest.value?.parameters || []).filter((p) => p.scope === "ANALYSIS"));
const newestFirst = computed(() => [...revisions.value].reverse());
const latest = computed(() => revisions.value[revisions.value.length - 1] || null);

function openDetails() {
  details.value = { name: environment.value.name, notes: environment.value.notes || "" };
  detailsError.value = "";
  editing.value = true;
}

async function saveDetails() {
  saving.value = true;
  detailsError.value = "";
  try {
    await environments.update(environmentId.value, { name: details.value.name.trim(), notes: details.value.notes.trim() });
    editing.value = false;
    message.success(t("environments.detailsSaved"));
    await load();
  } catch (e) {
    detailsError.value = e.message;
  } finally {
    saving.value = false;
  }
}

// The form starts from the latest measurements and the paradigm version they
// were validated against, so only what changed has to be typed.
async function openRevision() {
  error.value = "";
  try {
    manifest.value = await paradigms.version(latest.value.paradigmKey, latest.value.paradigmVersion);
    values.value = { ...defaults([...apparatusParameters.value, ...analysisParameters.value]), ...latest.value.parameters };
    revisionNotes.value = "";
    revising.value = true;
  } catch (e) {
    error.value = e.message;
  }
}

async function saveRevision() {
  saving.value = true;
  error.value = "";
  try {
    await environments.addRevision(environmentId.value, {
      paradigmVersion: latest.value.paradigmVersion,
      parameters: revisionParameters(apparatusParameters.value, analysisParameters.value, values.value),
      notes: revisionNotes.value.trim(),
    });
    revising.value = false;
    message.success(t("environments.revisionSaved"));
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(load);
watch(locale, async () => {
  try {
    await loadManifests();
    if (manifest.value) manifest.value = await paradigms.version(manifest.value.key, manifest.value.version);
  } catch (e) {
    error.value = e.message;
  }
});
onUnmounted(() => crumb.clear());
</script>

<template>
  <PageHead v-if="environment" :title="environment.name" :subtitle="paradigmName">
    <template #actions v-if="canWrite">
      <NButton size="small" secondary data-test="environment-edit" :disabled="editing" @click="openDetails">
        {{ $t("environments.editDetails") }}
      </NButton>
      <NButton size="small" secondary data-test="environment-new-revision" :disabled="revising || !latest" @click="openRevision">
        {{ $t("environments.newRevision") }}
      </NButton>
    </template>
  </PageHead>
  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />
  <p v-if="environment?.notes" class="muted notes">{{ environment.notes }}</p>

  <NModal v-model:show="editing" preset="card" :title="$t('environments.editDetails')" class="details-modal" :bordered="false">
    <NForm data-test="environment-edit-form" @submit.prevent="saveDetails">
      <NFormItem :label="$t('common.name')" required>
        <NInput v-model:value="details.name" maxlength="120" data-test="environment-edit-name" />
      </NFormItem>
      <NFormItem :label="$t('common.notes')">
        <NInput v-model:value="details.notes" type="textarea" maxlength="2000" :autosize="{ minRows: 2, maxRows: 6 }" data-test="environment-edit-notes" placeholder="" />
      </NFormItem>
      <NAlert v-if="detailsError" type="error" :title="detailsError" data-test="environment-edit-error" />
    </NForm>
    <template #footer>
      <NSpace justify="end">
        <NButton data-test="environment-edit-cancel" @click="editing = false">{{ $t("common.cancel") }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!details.name.trim()" data-test="environment-edit-save" @click="saveDetails">{{ $t("common.save") }}</NButton>
      </NSpace>
    </template>
  </NModal>

  <NCard v-if="revising" :bordered="true" size="small" class="edit-card">
    <form class="edit-form" data-test="revision-form" @submit.prevent="saveRevision">
      <p class="muted">{{ $t("environments.newRevisionHint") }}</p>
      <h4>{{ $t("environments.apparatus") }}</h4>
      <ParameterFields v-if="apparatusParameters.length" v-model="values" :parameters="apparatusParameters" test-prefix="revision" />
      <p class="muted" v-else>{{ $t("environments.noApparatus") }}</p>
      <template v-if="analysisParameters.length">
        <a href="#" class="link" data-test="revision-toggle-analysis" @click.prevent="showAnalysis = !showAnalysis">
          {{ showAnalysis ? $t("environments.hideAnalysis") : $t("environments.showAnalysis") }}
        </a>
        <div v-show="showAnalysis">
          <h4>{{ $t("environments.analysis") }}</h4>
          <p class="muted">{{ $t("environments.analysisHint") }}</p>
          <ParameterFields v-model="values" :parameters="analysisParameters" test-prefix="revision-analysis" />
        </div>
      </template>
      <label class="fld">
        <span>{{ $t("environments.revisionNotes") }}</span>
        <textarea v-model="revisionNotes" rows="2" data-test="revision-notes"></textarea>
      </label>
      <div class="actions">
        <NButton type="primary" attr-type="submit" :loading="saving" :disabled="!manifest" data-test="revision-save">
          {{ $t("environments.saveRevision") }}
        </NButton>
        <NButton quaternary data-test="revision-cancel" @click="revising = false">{{ $t("common.cancel") }}</NButton>
      </div>
    </form>
  </NCard>

  <h3 class="section" v-if="revisions.length">{{ $t("environments.revisions") }}</h3>
  <div class="revisions">
    <NCard
      v-for="revision in newestFirst"
      :key="revision.id"
      :bordered="true"
      size="small"
      data-test="environment-revision"
      :data-revision="revision.number"
    >
      <template #header>
        {{ $t("environments.revisionNumber", { number: revision.number }) }}
        <span class="muted version">v{{ revision.paradigmVersion }}</span>
        <NTag v-if="revision.number === latest?.number" size="small" round :bordered="false" type="info" class="latest">{{ $t("environments.latest") }}</NTag>
      </template>

      <NDescriptions :column="2" label-placement="left" size="small" bordered>
        <NDescriptionsItem
          v-for="item in shownValues(revision)"
          :key="item.parameter.key"
          :label="item.parameter.label"
          :data-test="`revision-value-${item.parameter.key}`"
        >
          {{ format(item.parameter, item.value) }}
        </NDescriptionsItem>
      </NDescriptions>

      <div class="calibration" data-test="environment-calibration" :data-status="stateOf(revision.number).status">
        <div class="calibration-heading">
          <h4>{{ $t("environments.calibration.title") }}</h4>
          <NTag
            v-if="stateOf(revision.number).status && stateOf(revision.number).status !== 'NOT_REQUIRED'"
            size="small"
            round
            :bordered="false"
            :type="stateOf(revision.number).status === 'CALIBRATED' && latestOf(revision.number)?.status === 'VALID'
              ? 'success'
              : latestOf(revision.number)?.status === 'REJECTED' ? 'error' : 'warning'"
            data-test="environment-default-badge"
            :data-active="stateOf(revision.number).status === 'CALIBRATED' && latestOf(revision.number)?.status === 'VALID'"
          >
            {{ stateOf(revision.number).status === 'CALIBRATED' && latestOf(revision.number)?.status === 'VALID'
              ? $t("environments.calibration.defaultActive")
              : $t("environments.calibration.defaultInactive") }}
          </NTag>
        </div>
        <p class="muted" v-if="stateOf(revision.number).status === 'NOT_REQUIRED'" data-test="calibration-not-required">
          {{ $t("environments.calibration.notRequired") }}
        </p>
        <template v-else-if="stateOf(revision.number).status">
          <p class="muted">{{ $t("environments.calibration.intro") }}</p>
          <p
            v-if="stateOf(revision.number).status === 'CALIBRATED' && latestOf(revision.number)?.status === 'VALID'"
            class="default-applied"
            data-test="environment-default-applied"
          >
            {{ $t("environments.calibration.automatic") }}
          </p>

          <CalibrationSummary
            v-if="latestOf(revision.number)"
            :calibration="latestOf(revision.number)"
            :zones="manifestOf(revision)?.zones || []"
          />
          <p v-else class="muted" data-test="calibration-waiting">{{ $t("environments.calibration.waiting") }}</p>
          <p class="err" v-if="latestOf(revision.number)?.status === 'REJECTED'" data-test="calibration-rejected">
            {{ $t("environments.calibration.rejected") }}
          </p>
          <p class="muted hint" v-if="latestOf(revision.number)">{{ $t("environments.calibration.fixedCamera") }}</p>

          <NButton
            v-if="canWrite && calibrating !== revision.number"
            size="small"
            secondary
            class="calibrate-btn"
            data-test="environment-calibrate"
            @click="calibrating = revision.number"
          >
            {{ $t(latestOf(revision.number) ? "environments.calibration.correct" : "environments.calibration.enter") }}
          </NButton>

          <Calibrator
            v-if="calibrating === revision.number"
            :paradigm="manifestOf(revision)"
            :parameters="revision.parameters || {}"
            :initial="latestOf(revision.number)"
            :correcting="!!latestOf(revision.number)"
            :note="$t('environments.calibration.note')"
            :saving="saving"
            :disabled="saving"
            @save="save(revision.number, $event)"
            @cancel="calibrating = 0"
          />
        </template>
      </div>
    </NCard>
  </div>
  <p v-if="!loading && !revisions.length && !error" class="muted">{{ $t("environments.empty") }}</p>
</template>

<style scoped>
.notes { margin: -8px 0 16px; }
.edit-card { margin-bottom: 16px; }
.details-modal { width: min(520px, calc(100vw - 32px)); }
.edit-form { display: flex; flex-direction: column; gap: 12px; }
.params { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 14px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.actions { display: flex; gap: 10px; align-items: center; }
.latest { margin-left: 8px; }
.section { margin: 8px 0 12px; font-size: 15px; }
.revisions { display: flex; flex-direction: column; gap: 16px; }
.version { margin-left: 6px; font-size: 12px; }
.calibration { margin-top: 18px; border-top: 1px solid var(--line); padding-top: 14px; }
.calibration-heading { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.calibration h4 { margin: 0; font-size: 14px; }
.default-applied { margin: 8px 0; padding: 8px 10px; border-radius: 8px; background: var(--panel2); }
.hint { margin: 8px 0 0; }
.calibrate-btn { margin-top: 10px; }
</style>
