<script setup>
// Adding a video is its own small task, so it has its own dialog instead of
// sitting among the recordings. The clip is optional: most videos are analyzed
// whole.
import { ref } from "vue";
import { NButton, NModal, NProgress } from "naive-ui";

defineProps({
  busy: { type: String, default: "" },
  progress: { type: Number, default: 0 },
});
// `clip` holds the seconds of the recording to analyze; an empty end means to the end.
const clip = defineModel("clip", { type: Object, required: true });
const show = defineModel("show", { type: Boolean, default: false });
const emit = defineEmits(["upload"]);
const showClip = ref(false);
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="$t('tests.addVideo')" style="width: 440px; max-width: 94vw" :mask-closable="!busy" :closable="!busy" data-test="upload-dialog">
    <label class="drop">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="17 8 12 3 7 8" /><line x1="12" y1="3" x2="12" y2="15" /></svg>
      <span>{{ $t("tests.chooseVideo") }}</span>
      <input type="file" accept="video/*" data-test="upload-input" :disabled="!!busy" @change="emit('upload', $event)" />
    </label>

    <div class="progress" v-if="busy === 'upload'" data-test="upload-progress">
      <NProgress type="line" :percentage="Math.round(progress * 100)" :height="6" />
      <span class="muted">{{ $t("tests.uploading") }} {{ Math.round(progress * 100) }}%</span>
    </div>
    <p class="muted" v-else-if="busy === 'finalize'">{{ $t("tests.finalizing") }}</p>

    <NButton text class="clip-toggle" @click="showClip = !showClip">
      {{ showClip ? "−" : "+" }} {{ $t("tests.clipOptional") }}
    </NButton>
    <div v-show="showClip" class="clip">
      <label class="fld">
        <span>{{ $t("tests.clipStart") }}</span>
        <input type="number" min="0" step="0.1" v-model="clip.startS" />
      </label>
      <label class="fld">
        <span>{{ $t("tests.clipEnd") }}</span>
        <input type="number" min="0" step="0.1" v-model="clip.endS" />
      </label>
    </div>
  </NModal>
</template>

<style scoped>
.drop {
  display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 26px 16px; border: 1.5px dashed var(--line);
  border-radius: 12px; color: var(--muted); cursor: pointer; text-align: center;
}
.drop:hover { border-color: var(--accent); color: var(--txt); }
.drop input { font-size: 12px; max-width: 100%; }
.progress { display: flex; flex-direction: column; gap: 4px; margin-top: 12px; }
.clip-toggle { margin-top: 14px; }
.clip { display: flex; gap: 10px; margin-top: 10px; }
.fld { display: flex; flex-direction: column; gap: 4px; flex: 1; }
.fld span { font-size: 12px; color: var(--muted); }
</style>
