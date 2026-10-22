<script setup>
// One experiment: a header with its status and one tab per part of the study.
// Tabs are separate components; each reads the experiment and asks for a
// reload through the injected study context, so tabs stay independent.
import { computed, onMounted, onUnmounted, provide, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NTabPane, NTabs, NTag } from "naive-ui";
import { api } from "../../api/client.js";
import { useBreadcrumb } from "../../stores/breadcrumb.js";
import PageHead from "../../components/PageHead.vue";
import Overview from "./tabs/Overview.vue";
import Design from "./tabs/Design.vue";
import Timeline from "./tabs/Timeline.vue";
import Endpoints from "./tabs/Endpoints.vue";
import Animals from "./tabs/Animals.vue";
import Dosing from "./tabs/Dosing.vue";
import Tests from "./tabs/Tests.vue";
import Results from "./tabs/Results.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const experiment = ref(null);
const error = ref("");
const id = computed(() => route.params.id);

async function load() {
  error.value = "";
  try {
    experiment.value = await api(`/experiments/${id.value}`);
    crumb.set([{ label: t("nav.experiments"), to: "/experiments" }, { label: experiment.value.code }]);
  } catch (e) {
    error.value = e.message;
  }
}

// Tabs read the experiment and call reload after changing its status.
provide("study", { experiment, reload: load, id });

const TABS = ["overview", "design", "timeline", "endpoints", "animals", "dosing", "tests", "results"];
const tab = computed({
  get: () => (TABS.includes(route.query.tab) ? route.query.tab : "overview"),
  set: (value) => router.replace({ query: { ...route.query, tab: value } }),
});
const statusType = (status) => ({ DRAFT: "default", RUNNING: "info", COMPLETED: "success" })[status] || "default";

onMounted(load);
watch(id, load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />
  <template v-if="experiment">
    <PageHead :title="`${experiment.code} · ${experiment.title}`">
      <template #actions>
        <NTag :type="statusType(experiment.status)" round :bordered="false" data-test="experiment-status">
          {{ $t(`experiments.status.${experiment.status}`) }}
        </NTag>
      </template>
    </PageHead>
    <NTabs v-model:value="tab" type="line" animated>
      <NTabPane name="overview" :tab="$t('experiments.tabs.overview')"><Overview /></NTabPane>
      <NTabPane name="design" :tab="$t('experiments.tabs.design')"><Design /></NTabPane>
      <NTabPane name="timeline" :tab="$t('experiments.tabs.timeline')"><Timeline /></NTabPane>
      <NTabPane name="endpoints" :tab="$t('experiments.tabs.endpoints')"><Endpoints /></NTabPane>
      <NTabPane name="animals" :tab="$t('experiments.tabs.animals')"><Animals /></NTabPane>
      <NTabPane name="dosing" :tab="$t('experiments.tabs.dosing')"><Dosing /></NTabPane>
      <NTabPane name="tests" :tab="$t('experiments.tabs.tests')"><Tests /></NTabPane>
      <NTabPane name="results" :tab="$t('experiments.tabs.results')"><Results /></NTabPane>
    </NTabs>
  </template>
</template>
