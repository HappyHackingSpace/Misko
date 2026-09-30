<script setup>
// One apparatus with its measurement revisions and, for each revision, the
// calibration every video of that revision uses. It is entered once when the rig
// is set up; a single video whose camera or arena moved is calibrated by hand
// from its own test page. Reading is open to every role, calibrating needs
// apparatus:write.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDescriptions, NDescriptionsItem, NTag } from "naive-ui";
import { environments, paradigms } from "../api/endpoints.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";
import CalibrationForm from "../components/CalibrationForm.vue";

const { t } = useI18n();
const route = useRoute();
const auth = useAuth();
const crumb = useBreadcrumb();
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
const centimetres = (value) => `${Number(value).toFixed(2)} cm`;
const tagType = (status) => ({ CALIBRATED: "success", VALID: "success", WAITING_FOR_CALIBRATION: "warning", REJECTED: "error" })[status] || "default";

async function load() {
  error.value = "";
  try {
    environment.value = await environments.get(environmentId.value);
    revisions.value = (await environments.revisions(environmentId.value)).data || [];
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
    await environments.calibrate(environmentId.value, number, { ...body, supersedesId: latestOf(number)?.id || "" });
    calibrating.value = 0;
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
const revising = ref(false);
const manifest = ref(null);
const values = ref({});
const revisionNotes = ref("");
const parameters = computed(() => manifest.value?.apparatusParameters || []);
const newestFirst = computed(() => [...revisions.value].reverse());
const latest = computed(() => revisions.value[revisions.value.length - 1] || null);

function openDetails() {
  details.value = { name: environment.value.name, notes: environment.value.notes || "" };
  editing.value = true;
}

async function saveDetails() {
  saving.value = true;
  error.value = "";
  try {
    await environments.update(environmentId.value, { name: details.value.name.trim(), notes: details.value.notes.trim() });
    editing.value = false;
    await load();
  } catch (e) {
    error.value = e.message;
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
    values.value = { ...latest.value.apparatus };
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
    const apparatus = {};
    for (const parameter of parameters.value) apparatus[parameter.key] = Number(values.value[parameter.key]);
    await environments.addRevision(environmentId.value, {
      paradigmVersion: latest.value.paradigmVersion,
      apparatus,
      notes: revisionNotes.value.trim(),
    });
    revising.value = false;
    await load();
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
  <PageHead v-if="environment" :title="environment.name" :subtitle="environment.paradigmKey">
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

  <NCard v-if="editing" :bordered="true" size="small" class="edit-card">
    <form class="edit-form" data-test="environment-edit-form" @submit.prevent="saveDetails">
      <label class="fld">
        <span>{{ $t("common.name") }}</span>
        <input v-model="details.name" data-test="environment-edit-name" required />
      </label>
      <label class="fld">
        <span>{{ $t("common.notes") }}</span>
        <textarea v-model="details.notes" rows="2" data-test="environment-edit-notes"></textarea>
      </label>
      <div class="actions">
        <NButton type="primary" attr-type="submit" :loading="saving" data-test="environment-edit-save">{{ $t("common.save") }}</NButton>
        <NButton quaternary data-test="environment-edit-cancel" @click="editing = false">{{ $t("common.cancel") }}</NButton>
      </div>
    </form>
  </NCard>

  <NCard v-if="revising" :bordered="true" size="small" class="edit-card">
    <form class="edit-form" data-test="revision-form" @submit.prevent="saveRevision">
      <p class="muted">{{ $t("environments.newRevisionHint") }}</p>
      <div class="params">
        <label class="fld" v-for="parameter in parameters" :key="parameter.key">
          <span>{{ parameter.label }} <span class="muted">{{ parameter.unit }}</span></span>
          <input
            type="number"
            step="any"
            :min="parameter.min"
            :max="parameter.max"
            v-model="values[parameter.key]"
            :data-test="`revision-${parameter.key}`"
            required
          />
          <small class="muted">{{ parameter.min }} - {{ parameter.max }}</small>
        </label>
      </div>
      <label class="fld">
        <span>{{ $t("environments.revisionNotes") }}</span>
        <textarea v-model="revisionNotes" rows="2" data-test="revision-notes"></textarea>
      </label>
      <div class="actions">
        <NButton type="primary" attr-type="submit" :loading="saving" :disabled="!parameters.length" data-test="revision-save">
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
        <NDescriptionsItem v-for="(value, key) in revision.apparatus" :key="key" :label="key">{{ value }}</NDescriptionsItem>
      </NDescriptions>

      <div class="calibration" data-test="environment-calibration" :data-status="stateOf(revision.number).status">
        <h4>{{ $t("environments.calibration.title") }}</h4>
        <p class="muted" v-if="stateOf(revision.number).status === 'NOT_REQUIRED'" data-test="calibration-not-required">
          {{ $t("environments.calibration.notRequired") }}
        </p>
        <template v-else-if="stateOf(revision.number).status">
          <p class="muted">{{ $t("environments.calibration.intro") }}</p>

          <NDescriptions
            v-if="latestOf(revision.number)"
            :column="1"
            label-placement="left"
            size="small"
            bordered
            data-test="calibration-summary"
            :data-calibration-status="latestOf(revision.number).status"
          >
            <NDescriptionsItem :label="$t('calibration.camera')">{{ latestOf(revision.number).cameraId }}</NDescriptionsItem>
            <NDescriptionsItem :label="$t('calibration.fitError')">
              <span data-test="fit-error">{{ centimetres(latestOf(revision.number).fitRmsErrorCm) }}</span>
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('calibration.checkError')">
              <span data-test="check-error">{{ centimetres(latestOf(revision.number).checkRmsErrorCm) }}</span>
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('calibration.maxError')">
              {{ centimetres(latestOf(revision.number).checkMaxErrorCm) }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('calibration.tolerance')">
              <span data-test="tolerance">{{ centimetres(latestOf(revision.number).toleranceCm) }}</span>
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('tests.status')">
              <NTag size="small" round :bordered="false" :type="tagType(latestOf(revision.number).status)">
                {{ $t(`statuses.${latestOf(revision.number).status}`) }}
              </NTag>
            </NDescriptionsItem>
          </NDescriptions>
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

          <CalibrationForm
            v-if="calibrating === revision.number"
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
.calibration h4 { margin: 0 0 6px; font-size: 14px; }
.hint { margin: 8px 0 0; }
.calibrate-btn { margin-top: 10px; }
</style>
