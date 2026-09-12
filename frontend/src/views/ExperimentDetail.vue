<script setup>
// One experiment: its groups, enrolled subjects and tests. Tests link to the
// workflow screen where video, calibration and analysis live.
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { environments, experiments, protocols } from "../api/endpoints.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const route = useRoute();
const auth = useAuth();
const crumb = useBreadcrumb();

const experiment = ref(null);
const groups = ref([]);
const enrollments = ref([]);
const tests = ref([]);
const protocolList = ref([]);
const revisionOptions = ref([]);
const error = ref("");

// Groups belong to the study, so writing one needs study:write.
const canWrite = computed(() => auth.can("study:write"));
const newGroup = ref({ name: "", role: "TREATMENT", targetSize: "" });
const adding = ref(false);

// A protocol pins apparatus revisions, so writing one needs apparatus:write
// rather than study:write.
const canWriteProtocol = computed(() => auth.can("apparatus:write"));
const emptyStep = () => ({ environmentRevisionId: "", trialType: "STANDARD", trials: 1 });
const newProtocol = ref({ name: "", description: "", steps: [emptyStep()] });
const savingProtocol = ref(false);

const revisionLabel = (option) =>
  t("protocols.revisionLabel", {
    name: option.environmentName,
    number: option.number,
    paradigm: option.paradigmKey,
    version: option.paradigmVersion,
  });

function addStep() {
  newProtocol.value.steps.push(emptyStep());
}

function removeStep(index) {
  newProtocol.value.steps.splice(index, 1);
}

// Every revision of every apparatus, flattened into one list. A step names a
// revision, and the paradigm it belongs to is read from that revision rather
// than asked for a second time: the API refuses a mismatch anyway.
async function loadRevisions() {
  const list = (await environments.list()).data || [];
  const perEnvironment = await Promise.all(
    list.map(async (environment) => {
      const revisions = (await environments.revisions(environment.id)).data || [];
      return revisions.map((revision) => ({
        id: revision.id,
        number: revision.number,
        paradigmKey: revision.paradigmKey,
        paradigmVersion: revision.paradigmVersion,
        environmentName: environment.name,
      }));
    }),
  );
  revisionOptions.value = perEnvironment.flat();
}

async function createProtocol() {
  error.value = "";
  savingProtocol.value = true;
  try {
    const steps = newProtocol.value.steps.map((step, index) => {
      const revision = revisionOptions.value.find((option) => option.id === step.environmentRevisionId);
      return {
        // Positions have to start at one and run without gaps.
        position: index + 1,
        paradigmKey: revision?.paradigmKey || "",
        paradigmVersion: revision?.paradigmVersion || 0,
        environmentRevisionId: step.environmentRevisionId,
        trialType: step.trialType.trim().toUpperCase(),
        trials: Number(step.trials),
        interTrialIntervalS: 0,
        session: {},
        notes: "",
      };
    });
    await protocols.create(route.params.id, {
      name: newProtocol.value.name.trim(),
      description: newProtocol.value.description.trim(),
      version: { notes: "", steps },
    });
    newProtocol.value = { name: "", description: "", steps: [emptyStep()] };
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    savingProtocol.value = false;
  }
}

const subjectOf = (test) => enrollments.value.find((e) => e.id === test.enrollmentId);

async function addGroup() {
  error.value = "";
  adding.value = true;
  try {
    await experiments.addGroup(route.params.id, {
      name: newGroup.value.name.trim(),
      role: newGroup.value.role,
      description: "",
      // An empty field means no target rather than a target of nothing.
      targetSize: Number(newGroup.value.targetSize || 0),
    });
    newGroup.value = { name: "", role: "TREATMENT", targetSize: "" };
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    adding.value = false;
  }
}

async function load() {
  error.value = "";
  try {
    const id = route.params.id;
    experiment.value = await experiments.get(id);
    const [groupList, enrollmentPage, testPage, protocolPage] = await Promise.all([
      experiments.groups(id),
      experiments.enrollments(id, { pageSize: 100 }),
      experiments.tests(id, { pageSize: 100 }),
      protocols.list(id),
    ]);
    groups.value = groupList.data || [];
    enrollments.value = enrollmentPage.data || [];
    tests.value = testPage.data || [];
    protocolList.value = protocolPage.data || [];
    // Only a role that may write a protocol needs the revisions to choose from.
    if (canWriteProtocol.value) await loadRevisions();
    crumb.set([
      { label: t("experiments.title"), to: "/experiments" },
      { label: experiment.value.code },
    ]);
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <p class="err" v-if="error">{{ error }}</p>

  <div class="detail" v-if="experiment">
    <div class="card">
      <div class="title">
        <h2 style="margin-top:0">{{ experiment.code }} <span class="muted">{{ experiment.title }}</span></h2>
        <RouterLink v-if="canWrite" class="link" :to="`/experiments/${experiment.id}/edit`" data-test="experiment-edit">
          {{ $t("common.edit") }}
        </RouterLink>
      </div>
      <p class="muted" v-if="experiment.description">{{ experiment.description }}</p>

      <div class="pills" v-if="groups.length">
        <!-- The role label is translated, so the raw value travels in an
             attribute for anything reading this without knowing the language. -->
        <span class="pill" v-for="group in groups" :key="group.id" data-test="group">
          {{ group.name }}
          <span class="muted" :data-role="group.role">
            · {{ group.role === "CONTROL" ? $t("experiments.control") : $t("experiments.treatment") }}
          </span>
        </span>
      </div>
      <p v-else class="muted">{{ $t("experiments.noGroups") }}</p>

      <form v-if="canWrite" class="add-group" data-test="group-form" @submit.prevent="addGroup">
        <label class="fld">
          <span>{{ $t("experiments.groupName") }}</span>
          <input v-model="newGroup.name" data-test="group-name" required />
        </label>
        <label class="fld">
          <span>{{ $t("experiments.groupRole") }}</span>
          <select v-model="newGroup.role" data-test="group-role">
            <option value="CONTROL">{{ $t("experiments.control") }}</option>
            <option value="TREATMENT">{{ $t("experiments.treatment") }}</option>
          </select>
        </label>
        <label class="fld">
          <span>{{ $t("experiments.targetSize") }}</span>
          <input type="number" min="0" v-model="newGroup.targetSize" data-test="group-target-size" />
        </label>
        <button type="submit" class="small" :disabled="adding" data-test="group-add">
          {{ $t("experiments.addGroup") }}
        </button>
      </form>
    </div>

    <div class="card">
      <h4>{{ $t("protocols.title") }}</h4>
      <ul class="protocols" v-if="protocolList.length">
        <li v-for="protocol in protocolList" :key="protocol.id" data-test="protocol">
          {{ protocol.name }}
          <span class="muted">
            · {{ $t("protocols.version") }} {{ protocol.latestVersion }}
          </span>
        </li>
      </ul>
      <p v-else class="muted">{{ $t("protocols.none") }}</p>

      <form v-if="canWriteProtocol" class="protocol-form" data-test="protocol-form" @submit.prevent="createProtocol">
        <label class="fld">
          <span>{{ $t("protocols.name") }}</span>
          <input v-model="newProtocol.name" data-test="protocol-name" required />
        </label>

        <!-- A step pins one apparatus revision. The paradigm comes with it, so
             it is never asked for separately. -->
        <div class="steps">
          <div class="step" v-for="(step, index) in newProtocol.steps" :key="index" data-test="protocol-step">
            <label class="fld grow">
              <span>{{ $t("protocols.step") }} {{ index + 1 }} · {{ $t("protocols.environment") }}</span>
              <select v-model="step.environmentRevisionId" :data-test="`step-revision-${index}`" required>
                <option value="">{{ $t("protocols.pickEnvironment") }}</option>
                <option v-for="option in revisionOptions" :key="option.id" :value="option.id">
                  {{ revisionLabel(option) }}
                </option>
              </select>
            </label>
            <label class="fld">
              <span>{{ $t("protocols.trialType") }}</span>
              <input v-model="step.trialType" :data-test="`step-trial-type-${index}`" required />
            </label>
            <label class="fld narrow">
              <span>{{ $t("protocols.trials") }}</span>
              <input type="number" min="1" max="1000" v-model="step.trials" :data-test="`step-trials-${index}`" required />
            </label>
            <button
              v-if="newProtocol.steps.length > 1"
              type="button"
              class="small"
              :data-test="`step-remove-${index}`"
              @click="removeStep(index)"
            >
              {{ $t("protocols.removeStep") }}
            </button>
          </div>
        </div>

        <div class="actions">
          <button type="button" class="small" data-test="protocol-add-step" @click="addStep">
            {{ $t("protocols.addStep") }}
          </button>
          <button type="submit" :disabled="savingProtocol" data-test="protocol-save">{{ $t("common.save") }}</button>
        </div>
      </form>
    </div>

    <div class="card">
      <h4>{{ $t("experiments.tests") }}</h4>
      <div class="table-scroll" v-if="tests.length">
        <table class="rows" data-test="experiment-tests">
          <thead>
            <tr>
              <th>{{ $t("tests.subject") }}</th>
              <th>{{ $t("tests.paradigm") }}</th>
              <th>{{ $t("tests.scheduled") }}</th>
              <th>{{ $t("tests.status") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="test in tests" :key="test.id">
              <td>
                <RouterLink class="link" :to="`/tests/${test.id}`" data-test="test-link">
                  {{ subjectOf(test)?.subjectId?.slice(0, 8) || test.id.slice(0, 8) }}
                </RouterLink>
              </td>
              <td>{{ test.paradigmKey }} <span class="muted">v{{ test.paradigmVersion }}</span></td>
              <td class="muted">{{ new Date(test.scheduledAt).toLocaleString() }}</td>
              <td>{{ $t(`statuses.${test.status}`) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="muted">{{ $t("experiments.noTests") }}</p>
    </div>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.title { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.pills { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 10px; }
.add-group { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 14px; }
.protocols { list-style: none; margin: 0 0 12px; padding: 0; display: flex; flex-direction: column; gap: 6px; }
.protocol-form { display: flex; flex-direction: column; gap: 12px; margin-top: 14px; }
.steps { display: flex; flex-direction: column; gap: 10px; }
.step { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
.grow { flex: 1; min-width: 240px; }
.narrow { max-width: 110px; }
.actions { display: flex; gap: 12px; align-items: center; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.small { padding: 4px 10px; font-size: .85rem; }
.rows { width: 100%; border-collapse: collapse; }
.rows th { text-align: left; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; padding: 6px 8px; }
.rows td { padding: 6px 8px; border-top: 1px solid var(--line); }
.link { color: var(--accent); font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
