<script setup>
// The Results tab: the study report of this experiment. A draft has no results
// yet, so the tab says so instead of showing empty tables.
import { inject } from "vue";
import { NEmpty } from "naive-ui";
import ReportView from "../../../components/analysis/ReportView.vue";

const { experiment, id } = inject("study");
</script>

<template>
  <NEmpty v-if="experiment.status === 'DRAFT'" :description="$t('studyReport.draft')" data-test="results-draft" />
  <ReportView v-else :experiment-id="id" />
</template>
