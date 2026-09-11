<script setup>
// One experiment: its groups, enrolled subjects and tests. Tests link to the
// workflow screen where video, calibration and analysis live.
import { onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { experiments } from "../api/endpoints.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";

const { t } = useI18n();
const route = useRoute();
const crumb = useBreadcrumb();

const experiment = ref(null);
const groups = ref([]);
const enrollments = ref([]);
const tests = ref([]);
const error = ref("");

const subjectOf = (test) => enrollments.value.find((e) => e.id === test.enrollmentId);

async function load() {
  error.value = "";
  try {
    const id = route.params.id;
    experiment.value = await experiments.get(id);
    const [groupList, enrollmentPage, testPage] = await Promise.all([
      experiments.groups(id),
      experiments.enrollments(id, { pageSize: 100 }),
      experiments.tests(id, { pageSize: 100 }),
    ]);
    groups.value = groupList.data || [];
    enrollments.value = enrollmentPage.data || [];
    tests.value = testPage.data || [];
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
      <h2 style="margin-top:0">{{ experiment.code }} <span class="muted">{{ experiment.title }}</span></h2>
      <p class="muted" v-if="experiment.description">{{ experiment.description }}</p>
      <div class="pills">
        <span class="pill" v-for="group in groups" :key="group.id">
          {{ group.name }}
          <span class="muted">· {{ group.role === "CONTROL" ? $t("experiments.control") : $t("experiments.treatment") }}</span>
        </span>
      </div>
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
.pills { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 10px; }
.rows { width: 100%; border-collapse: collapse; }
.rows th { text-align: left; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; padding: 6px 8px; }
.rows td { padding: 6px 8px; border-top: 1px solid var(--line); }
.link { color: var(--accent); font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
