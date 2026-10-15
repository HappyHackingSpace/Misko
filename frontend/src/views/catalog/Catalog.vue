<script setup>
// The lab-wide catalog every study draws from: substances and disease models.
// The open tab is kept in the address so a link opens the same list.
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { NCard, NTabPane, NTabs } from "naive-ui";
import PageHead from "../../components/PageHead.vue";
import SubstancesTab from "./SubstancesTab.vue";
import DiseaseModelsTab from "./DiseaseModelsTab.vue";

const TABS = ["substances", "disease-models"];
const route = useRoute();
const router = useRouter();
const tab = computed({
  get: () => (TABS.includes(route.query.tab) ? route.query.tab : TABS[0]),
  set: (value) => router.replace({ query: { ...route.query, tab: value } }),
});
</script>

<template>
  <PageHead :title="$t('catalog.title')" :subtitle="$t('catalog.subtitle')" />
  <NCard :bordered="true" size="small">
    <NTabs v-model:value="tab" type="line" animated>
      <NTabPane name="substances" :tab="$t('catalog.tabs.substances')" display-directive="show:lazy">
        <SubstancesTab />
      </NTabPane>
      <NTabPane name="disease-models" :tab="$t('catalog.tabs.diseaseModels')" display-directive="show:lazy">
        <DiseaseModelsTab />
      </NTabPane>
    </NTabs>
  </NCard>
</template>
