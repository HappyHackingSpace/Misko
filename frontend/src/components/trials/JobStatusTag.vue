<script setup>
// The state of an analysis job in words: queued, running, succeeded, failed.
import { NTag } from "naive-ui";
import { runTag } from "../analysis/format.js";

defineProps({ status: { type: String, required: true } });
</script>

<template>
  <NTag size="small" round :bordered="false" :type="runTag(status)" data-test="job-status" :data-status="status">
    {{ $t(`runs.statuses.${status}`) }}
  </NTag>
</template>
