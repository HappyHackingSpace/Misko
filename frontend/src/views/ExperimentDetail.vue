<script setup>
// One experiment: its groups, enrolled subjects and tests. Tests link to the
// workflow screen where video, calibration and analysis live.
import { computed, h, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NButton, NCard, NDataTable, NSelect, NTag } from "naive-ui";
import { environments, experiments, protocols, subjects } from "../api/endpoints.js";
import { useAuth } from "../stores/auth.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";
import { filterByText, selectOption } from "../composables/selectOptions.js";

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
const subjectList = ref([]);
const error = ref("");

// A status carries meaning through its color everywhere else in the panel;
// the tests table here uses the same map.
const STATUS_TAG = { PLANNED: "info", IN_PROGRESS: "warning", COMPLETED: "success", CANCELLED: "error" };
const tagType = (status) => STATUS_TAG[status] || "default";

// Groups belong to the study, so writing one needs study:write.
const canWrite = computed(() => auth.can("study:write"));
const newGroup = ref({ name: "", role: "TREATMENT", targetSize: "" });
const adding = ref(false);

// A protocol pins apparatus revisions, so writing one needs apparatus:write
// rather than study:write.
const canWriteProtocol = computed(() => auth.can("apparatus:write"));
const emptyStep = () => ({ environmentId: "", environmentRevisionId: "", trialType: "STANDARD", trials: 1 });
const newProtocol = ref({ name: "", description: "", steps: [emptyStep()] });
const savingProtocol = ref(false);

function addStep() {
  newProtocol.value.steps.push(emptyStep());
}

function removeStep(index) {
  newProtocol.value.steps.splice(index, 1);
}

// Every revision of every apparatus, in one request. A step names an environment
// first and then one of its revisions; the paradigm comes with the revision, so it
// is never asked for separately (the API refuses a mismatch anyway).
async function loadRevisions() {
  const all = (await environments.allRevisions()).data || [];
  // The highest number of an environment is the one to use now.
  const newest = {};
  for (const revision of all) newest[revision.environmentId] = Math.max(newest[revision.environmentId] || 0, revision.number);
  revisionOptions.value = all.map((revision) => ({ ...revision, latest: revision.number === newest[revision.environmentId] }));
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

// Planning a test writes it, so it needs test:write. Running one later needs
// test:run, which is a different permission and a different screen.
const canPlanTest = computed(() => auth.can("test:write"));
const protocolVersions = ref([]);
const planning = ref(false);

// The datetime field speaks local time and holds whole minutes. The default is the
// next whole minute: rounding down would put it before a subject enrolled a moment
// ago, which the API refuses because a test cannot precede the enrollment.
function localNow() {
  const next = new Date(Math.ceil(Date.now() / 60_000) * 60_000);
  next.setMinutes(next.getMinutes() - next.getTimezoneOffset());
  return next.toISOString().slice(0, 16);
}

const newTest = ref({ enrollmentId: "", protocolId: "", protocolVersionId: "", stepPosition: "", scheduledAt: localNow(), notes: "" });

// An enrollment names a subject by id only, so the code comes from the subjects
// the laboratory holds. It is what a person reads on both the picker and the list.
const subjectCode = (subjectId) => subjectList.value.find((s) => s.id === subjectId)?.code || t("tests.subjectUnknown");

const selectedVersion = computed(() => protocolVersions.value.find((v) => v.id === newTest.value.protocolVersionId));

const versionLabel = (version) => t("tests.versionLabel", { number: version.number });

const stepLabel = (step) =>
  t("tests.stepLabel", {
    position: step.position,
    paradigm: step.paradigmKey,
    type: step.trialType,
    trials: step.trials,
  });

// A test is planned against one step of one protocol version, so choosing a
// protocol has to bring its versions along. The newest is the default, since
// that is the procedure the study is running now.
async function loadVersions() {
  newTest.value.protocolVersionId = "";
  newTest.value.stepPosition = "";
  protocolVersions.value = [];
  if (!newTest.value.protocolId) return;
  const page = await protocols.versions(route.params.id, newTest.value.protocolId);
  protocolVersions.value = (page.data || []).slice().sort((a, b) => b.number - a.number);
  newTest.value.protocolVersionId = protocolVersions.value[0]?.id || "";
  pickVersion();
}

function pickVersion() {
  newTest.value.stepPosition = selectedVersion.value?.steps?.[0]?.position || "";
}

function chooseProtocol(id) {
  newTest.value.protocolId = id || "";
  return loadVersions();
}

function chooseVersion(id) {
  newTest.value.protocolVersionId = id || "";
  pickVersion();
}

async function planTest() {
  error.value = "";
  planning.value = true;
  try {
    await experiments.planTest(route.params.id, {
      enrollmentId: newTest.value.enrollmentId,
      // Phases are not written from the panel yet, and the API treats an empty
      // phase as "not part of a phase" rather than as a missing field.
      phaseId: "",
      protocolVersionId: newTest.value.protocolVersionId,
      stepPosition: Number(newTest.value.stepPosition),
      // The field holds local time; the API stores an instant.
      scheduledAt: new Date(newTest.value.scheduledAt).toISOString(),
      notes: newTest.value.notes.trim(),
    });
    newTest.value.notes = "";
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    planning.value = false;
  }
}

// Enrolling a subject is study:write too, same as adding a group; it just
// names a subject (and optionally a starting group) rather than a role.
const newEnrollment = ref({ subjectId: "", groupId: "" });
const enrolling = ref(false);

// A subject already enrolled would only fail the API's one-per-experiment
// rule, so it is left out of the picker instead of offered and rejected.
const availableSubjects = computed(() => subjectList.value.filter((s) => !enrollments.value.some((e) => e.subjectId === s.id)));

const groupName = (groupId) => groups.value.find((g) => g.id === groupId)?.name || "";

async function enrollSubject() {
  error.value = "";
  enrolling.value = true;
  try {
    await experiments.enroll(route.params.id, {
      subjectId: newEnrollment.value.subjectId,
      groupId: newEnrollment.value.groupId || "",
      // Enrollment starts now; the panel does not offer a past start yet.
      enrolledAt: new Date().toISOString(),
    });
    newEnrollment.value = { subjectId: "", groupId: "" };
    // The default time was read when the page opened, before this enrollment.
    newTest.value.scheduledAt = localNow();
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    enrolling.value = false;
  }
}

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

const columns = computed(() => [
  {
    title: t("tests.subject"),
    key: "subjectId",
    render: (row) =>
      h(RouterLink, { class: "link", to: `/tests/${row.id}`, "data-test": "test-link" }, () => subjectCode(row.subjectId)),
  },
  {
    title: t("tests.paradigm"),
    key: "paradigmKey",
    render: (row) => [
      h(NTag, { size: "small", round: true, bordered: false }, () => row.paradigmKey),
      h("span", { class: "muted version" }, `v${row.paradigmVersion}`),
    ],
  },
  { title: t("tests.scheduled"), key: "scheduledAt", render: (row) => new Date(row.scheduledAt).toLocaleString() },
  {
    title: t("tests.status"),
    key: "status",
    render: (row) => h(NTag, { size: "small", round: true, type: tagType(row.status) }, () => t(`statuses.${row.status}`)),
  },
]);

async function load() {
  error.value = "";
  try {
    const id = route.params.id;
    experiment.value = await experiments.get(id);
    const [groupList, enrollmentPage, testPage, protocolPage, subjectPage] = await Promise.all([
      experiments.groups(id),
      experiments.enrollments(id, { pageSize: 100 }),
      experiments.tests(id, { pageSize: 100 }),
      protocols.list(id),
      subjects.list({ pageSize: 100 }),
    ]);
    groups.value = groupList.data || [];
    enrollments.value = enrollmentPage.data || [];
    tests.value = testPage.data || [];
    protocolList.value = protocolPage.data || [];
    subjectList.value = subjectPage.data || [];
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

// The choices of every dropdown. A test hook rides on each option, see selectOptions.js.
const groupRoleOptions = computed(() => [
  selectOption("group-role", "CONTROL", t("experiments.control")),
  selectOption("group-role", "TREATMENT", t("experiments.treatment")),
]);
const subjectOptions = computed(() => availableSubjects.value.map((s) => selectOption("enroll-subject", s.id, s.code)));
const enrollGroupOptions = computed(() => groups.value.map((g) => selectOption("enroll-group", g.id, g.name)));
const stepEnvironmentOptions = computed(() => {
  const seen = new Map(revisionOptions.value.map((o) => [o.environmentId, o.environmentName]));
  return [...seen].map(([id, name]) => selectOption("step-environment", id, name));
});
// The revisions of the environment a step has chosen, newest first.
const stepRevisionOptions = (step) =>
  revisionOptions.value
    .filter((o) => o.environmentId === step.environmentId)
    .toReversed()
    .map((o) =>
      selectOption(
        "step-revision",
        o.id,
        `${t("environments.revisionNumber", { number: o.number })} (${o.paradigmKey} v${o.paradigmVersion})`,
        o.latest ? t("environments.latest") : "",
      ),
    );

// Choosing another environment empties the revision: it belonged to the old one.
function chooseEnvironment(step, id) {
  step.environmentId = id || "";
  step.environmentRevisionId = "";
}
const planEnrollmentOptions = computed(() =>
  enrollments.value.map((e) => selectOption("plan-enrollment", e.id, subjectCode(e.subjectId))),
);
const planProtocolOptions = computed(() => protocolList.value.map((p) => selectOption("plan-protocol", p.id, p.name)));
const planVersionOptions = computed(() => protocolVersions.value.map((v) => selectOption("plan-version", v.id, versionLabel(v))));
const planStepOptions = computed(() =>
  (selectedVersion.value?.steps || []).map((s) => selectOption("plan-step", s.position, stepLabel(s))),
);

// A dropdown has no native "required", so what a form cannot be saved without is
// checked here and the button waits for it.
const protocolReady = computed(() => !!newProtocol.value.name.trim() && newProtocol.value.steps.every((s) => s.environmentRevisionId));
const planReady = computed(() => !!(newTest.value.enrollmentId && newTest.value.protocolVersionId && newTest.value.stepPosition));

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />

  <div class="detail" v-if="experiment">
    <PageHead :title="`${experiment.code} · ${experiment.title}`" :subtitle="experiment.description">
      <template #actions>
        <NButton v-if="canWrite" secondary data-test="experiment-edit" @click="$router.push(`/experiments/${experiment.id}/edit`)">
          {{ $t("common.edit") }}
        </NButton>
      </template>
    </PageHead>

    <div class="grid-2">
      <div class="col">
        <NCard :bordered="true" size="small">
          <template #header>{{ $t("experiments.groups") }}</template>
          <div class="pills" v-if="groups.length">
            <!-- The role label is translated, so the raw value travels in an
                 attribute for anything reading this without knowing the language. -->
            <NTag v-for="group in groups" :key="group.id" round :bordered="false" data-test="group">
              {{ group.name }}
              <span class="muted" :data-role="group.role">
                · {{ group.role === "CONTROL" ? $t("experiments.control") : $t("experiments.treatment") }}
              </span>
            </NTag>
          </div>
          <p v-else class="muted empty">{{ $t("experiments.noGroups") }}</p>

          <form v-if="canWrite" class="add-group" data-test="group-form" @submit.prevent="addGroup">
            <label class="fld">
              <span>{{ $t("experiments.groupName") }}</span>
              <input v-model="newGroup.name" data-test="group-name" required />
            </label>
            <label class="fld">
              <span>{{ $t("experiments.groupRole") }}</span>
              <NSelect v-model:value="newGroup.role" data-test="group-role" :options="groupRoleOptions" />
            </label>
            <label class="fld narrow">
              <span>{{ $t("experiments.targetSize") }}</span>
              <input type="number" min="0" v-model="newGroup.targetSize" data-test="group-target-size" />
            </label>
            <NButton attr-type="submit" :loading="adding" :disabled="adding" data-test="group-add">
              {{ $t("experiments.addGroup") }}
            </NButton>
          </form>
        </NCard>

        <NCard :bordered="true" size="small">
          <template #header>{{ $t("experiments.enrollments") }}</template>
          <ul class="rows-list" v-if="enrollments.length" data-test="enrollments">
            <li v-for="enrollment in enrollments" :key="enrollment.id" data-test="enrollment">
              <span>{{ subjectCode(enrollment.subjectId) }}</span>
              <span class="muted" v-if="enrollment.currentGroupId">{{ groupName(enrollment.currentGroupId) }}</span>
            </li>
          </ul>
          <p v-else class="muted empty">{{ $t("experiments.noEnrollments") }}</p>

          <form v-if="canWrite && availableSubjects.length" class="add-group" data-test="enroll-form" @submit.prevent="enrollSubject">
            <label class="fld grow">
              <span>{{ $t("tests.subject") }}</span>
              <NSelect
                v-model:value="newEnrollment.subjectId"
                data-test="enroll-subject"
                filterable
                :filter="filterByText"
                :placeholder="$t('experiments.pickSubject')"
                :options="subjectOptions"
              />
            </label>
            <label class="fld">
              <span>{{ $t("experiments.assignGroup") }}</span>
              <NSelect
                v-model:value="newEnrollment.groupId"
                data-test="enroll-group"
                clearable
                :placeholder="$t('experiments.noGroup')"
                :options="enrollGroupOptions"
              />
            </label>
            <NButton attr-type="submit" :loading="enrolling" :disabled="enrolling || !newEnrollment.subjectId" data-test="enroll-save">
              {{ $t("experiments.enroll") }}
            </NButton>
          </form>
          <p v-else-if="canWrite && subjectList.length" class="muted empty" data-test="enroll-none-left">
            {{ $t("experiments.noSubjectsToEnroll") }}
          </p>
        </NCard>
      </div>

      <NCard :bordered="true" size="small">
        <template #header>{{ $t("protocols.title") }}</template>
        <ul class="rows-list" v-if="protocolList.length">
          <li v-for="protocol in protocolList" :key="protocol.id" data-test="protocol">
            <span>{{ protocol.name }}</span>
            <span class="muted">{{ $t("protocols.version") }} {{ protocol.latestVersion }}</span>
          </li>
        </ul>
        <p v-else class="muted empty">{{ $t("protocols.none") }}</p>

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
                <NSelect
                  :value="step.environmentId"
                  :data-test="`step-environment-${index}`"
                  filterable
                  :filter="filterByText"
                  :placeholder="$t('protocols.pickEnvironment')"
                  :options="stepEnvironmentOptions"
                  @update:value="chooseEnvironment(step, $event)"
                />
              </label>
              <!-- The revisions belong to the environment, so the dropdown stays shut
                   until one is chosen. -->
              <label class="fld grow">
                <span>{{ $t("protocols.revision") }}</span>
                <NSelect
                  v-model:value="step.environmentRevisionId"
                  :data-test="`step-revision-${index}`"
                  :disabled="!step.environmentId"
                  :placeholder="$t(step.environmentId ? 'protocols.pickRevision' : 'protocols.pickEnvironmentFirst')"
                  :options="stepRevisionOptions(step)"
                />
              </label>
              <label class="fld">
                <span>{{ $t("protocols.trialType") }}</span>
                <input v-model="step.trialType" :data-test="`step-trial-type-${index}`" required />
              </label>
              <label class="fld narrow">
                <span>{{ $t("protocols.trials") }}</span>
                <input type="number" min="1" max="1000" v-model="step.trials" :data-test="`step-trials-${index}`" required />
              </label>
              <NButton
                v-if="newProtocol.steps.length > 1"
                size="tiny"
                quaternary
                :data-test="`step-remove-${index}`"
                @click="removeStep(index)"
              >
                {{ $t("protocols.removeStep") }}
              </NButton>
            </div>
          </div>

          <div class="actions">
            <NButton dashed size="small" data-test="protocol-add-step" @click="addStep">
              + {{ $t("protocols.addStep") }}
            </NButton>
            <NButton type="primary" attr-type="submit" :loading="savingProtocol" :disabled="savingProtocol || !protocolReady" data-test="protocol-save">
              {{ $t("common.save") }}
            </NButton>
          </div>
        </form>
      </NCard>
    </div>

    <NCard :bordered="true" size="small">
      <template #header>{{ $t("experiments.tests") }}</template>
      <NDataTable
        v-if="tests.length"
        data-test="experiment-tests"
        :columns="columns"
        :data="tests"
        :row-key="(row) => row.id"
        :row-props="(row) => ({ 'data-test': 'test-row', 'data-scheduled': row.scheduledAt })"
      />
      <p v-else class="muted empty">{{ $t("experiments.noTests") }}</p>

      <form v-if="canPlanTest" class="plan-form" data-test="plan-form" @submit.prevent="planTest">
        <p v-if="!enrollments.length" class="muted" data-test="plan-needs-enrollment">{{ $t("tests.needsEnrollment") }}</p>
        <p v-else-if="!protocolList.length" class="muted" data-test="plan-needs-protocol">{{ $t("tests.needsProtocol") }}</p>
        <template v-else>
          <label class="fld">
            <span>{{ $t("tests.enrollment") }}</span>
            <NSelect
              v-model:value="newTest.enrollmentId"
              data-test="plan-enrollment"
              filterable
              :filter="filterByText"
              :placeholder="$t('tests.pickEnrollment')"
              :options="planEnrollmentOptions"
            />
          </label>
          <label class="fld">
            <span>{{ $t("tests.protocol") }}</span>
            <NSelect
              :value="newTest.protocolId"
              data-test="plan-protocol"
              filterable
              :filter="filterByText"
              :placeholder="$t('tests.pickProtocol')"
              :options="planProtocolOptions"
              @update:value="chooseProtocol"
            />
          </label>
          <label class="fld">
            <span>{{ $t("tests.version") }}</span>
            <NSelect
              :value="newTest.protocolVersionId"
              data-test="plan-version"
              :disabled="!protocolVersions.length"
              :options="planVersionOptions"
              @update:value="chooseVersion"
            />
          </label>
          <label class="fld grow">
            <span>{{ $t("tests.step") }}</span>
            <NSelect
              v-model:value="newTest.stepPosition"
              data-test="plan-step"
              :disabled="!planStepOptions.length"
              :options="planStepOptions"
            />
          </label>
          <label class="fld">
            <span>{{ $t("tests.scheduledAt") }}</span>
            <input type="datetime-local" v-model="newTest.scheduledAt" data-test="plan-scheduled" required />
          </label>
          <NButton type="primary" attr-type="submit" :loading="planning" :disabled="planning || !planReady" data-test="plan-save">
            {{ $t("tests.plan") }}
          </NButton>
        </template>
      </form>
    </NCard>
  </div>
</template>

<style scoped>
.detail { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; align-items: start; }
.grid-2 .col { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
@media (max-width: 860px) { .grid-2 { grid-template-columns: 1fr; } }

.pills { display: flex; gap: 8px; flex-wrap: wrap; }
.add-group { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 14px; }
.rows-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.rows-list li { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 8px 10px; border-radius: 8px; }
.rows-list li:hover { background: var(--panel2); }
.empty { margin: 0; }
.protocol-form { display: flex; flex-direction: column; gap: 12px; margin-top: 14px; }
.plan-form { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 14px; }
.steps { display: flex; flex-direction: column; gap: 8px; }
.step { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; padding: 10px; border-radius: 9px; background: var(--panel2); }
.grow { flex: 1; min-width: 220px; }
.narrow { max-width: 110px; }
.actions { display: flex; gap: 12px; align-items: center; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.fld :deep(.n-select) { width: 100%; min-width: 190px; }
.grow :deep(.n-select) { min-width: 220px; }
.version { margin-left: 6px; }
:deep(.link) { color: inherit; text-decoration: none; }
</style>
