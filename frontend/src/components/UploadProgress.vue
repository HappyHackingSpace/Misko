<script setup>
// Where a video upload stands: checking the file, sending it (with percent)
// and verifying it in storage. Wording is shared with the session video.
import { NProgress } from "naive-ui";

defineProps({
  // checksum | upload | verify
  stage: { type: String, required: true },
  // 0 to 1 while uploading.
  progress: { type: Number, default: 0 },
});
</script>

<template>
  <div class="progress" data-test="video-progress">
    <NProgress
      type="line"
      :percentage="stage === 'upload' ? Math.round(progress * 100) : stage === 'verify' ? 100 : 0"
      :height="8"
      :processing="stage !== 'upload'"
    />
    <span class="muted">{{ $t(`execution.video.stages.${stage}`, { percent: Math.round(progress * 100) }) }}</span>
  </div>
</template>

<style scoped>
.progress { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; }
</style>
