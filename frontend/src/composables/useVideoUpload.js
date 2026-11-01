// State of one video upload for a screen: the stage (checksum, upload,
// verify), the upload progress and the error, around an upload function such
// as uploadVideo or uploadTrialVideo that takes { onStage, onProgress }.
import { computed, ref } from "vue";

export function useVideoUpload(send) {
  const stage = ref("");
  const progress = ref(0);
  const error = ref("");
  const busy = computed(() => !!stage.value);

  // Resolves to what `send` returned, or null when it failed (see error).
  async function start(file) {
    error.value = "";
    progress.value = 0;
    try {
      return await send(file, { onStage: (value) => (stage.value = value), onProgress: (value) => (progress.value = value) });
    } catch (e) {
      error.value = e.message;
      return null;
    } finally {
      stage.value = "";
    }
  }

  return { stage, progress, error, busy, start };
}
