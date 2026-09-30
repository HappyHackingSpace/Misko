<script setup>
// The calibration form shared by the environment page and by a recording's
// manual override. It only collects and shapes the request; the parent decides
// where it goes and which calibration a correction replaces.
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NSelect } from "naive-ui";
import { selectOption } from "../composables/selectOptions.js";

const props = defineProps({
  // Values carried over from the calibration this one replaces or is based on,
  // so the camera and the frame are not asked for again.
  initial: { type: Object, default: null },
  // A recording's calibration names the video frame its points were read from;
  // an environment has no video, so it does not ask.
  withReferenceFrame: { type: Boolean, default: false },
  // Set while entering a correction, to say so above the form.
  correcting: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  // A sentence shown above the form, for what the calibration is about to do.
  note: { type: String, default: "" },
});
const emit = defineEmits(["save", "cancel"]);

const PLANES = ["ARENA_FLOOR", "WATER_SURFACE", "APPARATUS_TOP"];
const { t } = useI18n();
const planeOptions = computed(() => PLANES.map((plane) => selectOption("plane", plane, t(`calibration.planes.${plane}`))));
const emptyPoint = () => ({ pixelX: "", pixelY: "", worldX: "", worldY: "" });

// The API asks for at least four fit points and three check points.
const form = ref({
  cameraId: props.initial?.cameraId || "",
  frameWidth: props.initial?.frameWidth || "",
  frameHeight: props.initial?.frameHeight || "",
  crop: props.initial?.crop ? { ...props.initial.crop } : { x: 0, y: 0, width: "", height: "" },
  referenceFrameS: 0,
  measurementPlane: props.initial?.measurementPlane || "ARENA_FLOOR",
  fitPoints: [emptyPoint(), emptyPoint(), emptyPoint(), emptyPoint()],
  checkPoints: [emptyPoint(), emptyPoint(), emptyPoint()],
});

const point = (p) => ({
  pixelX: Number(p.pixelX),
  pixelY: Number(p.pixelY),
  worldX: Number(p.worldX),
  worldY: Number(p.worldY),
});

function submit() {
  const body = {
    cameraId: form.value.cameraId.trim(),
    frameWidth: Number(form.value.frameWidth),
    frameHeight: Number(form.value.frameHeight),
    crop: {
      x: Number(form.value.crop.x),
      y: Number(form.value.crop.y),
      width: Number(form.value.crop.width),
      height: Number(form.value.crop.height),
    },
    measurementPlane: form.value.measurementPlane,
    fitPoints: form.value.fitPoints.map(point),
    checkPoints: form.value.checkPoints.map(point),
  };
  // The field speaks seconds; the API counts microseconds from the start.
  if (props.withReferenceFrame) body.referenceFrameUs = Math.round(Number(form.value.referenceFrameS || 0) * 1_000_000);
  emit("save", body);
}
</script>

<template>
  <form class="calibration-form" data-test="calibration-form" @submit.prevent="submit">
    <p class="muted" v-if="correcting" data-test="calibration-correcting">
      {{ $t("calibration.correcting") }}
    </p>
    <p class="muted" v-if="note" data-test="calibration-note">{{ note }}</p>
    <div class="setup">
      <label class="fld">
        <span>{{ $t("calibration.camera") }}</span>
        <input v-model="form.cameraId" data-test="camera-id" required />
      </label>
      <label class="fld narrow">
        <span>{{ $t("calibration.frame") }}</span>
        <input type="number" min="1" v-model="form.frameWidth" data-test="frame-width" required />
      </label>
      <label class="fld narrow">
        <span>&nbsp;</span>
        <input type="number" min="1" v-model="form.frameHeight" data-test="frame-height" required />
      </label>
      <label class="fld narrow">
        <span>{{ $t("calibration.crop") }}</span>
        <input type="number" min="0" v-model="form.crop.width" data-test="crop-width" required />
      </label>
      <label class="fld narrow">
        <span>&nbsp;</span>
        <input type="number" min="0" v-model="form.crop.height" data-test="crop-height" required />
      </label>
      <label class="fld">
        <span>{{ $t("calibration.plane") }}</span>
        <NSelect v-model:value="form.measurementPlane" data-test="plane" :options="planeOptions" />
      </label>
      <label class="fld narrow" v-if="withReferenceFrame">
        <span>{{ $t("calibration.referenceFrame") }}</span>
        <input type="number" min="0" step="0.001" v-model="form.referenceFrameS" data-test="reference-frame" />
      </label>
    </div>

    <p class="muted hint">{{ $t("calibration.pointsHint") }}</p>

    <template v-for="group in [{ kind: 'fit', key: 'fitPoints', min: 4 }, { kind: 'check', key: 'checkPoints', min: 3 }]" :key="group.kind">
      <h5>{{ $t(`calibration.${group.key}`) }}</h5>
      <div class="points">
        <div class="point" v-for="(p, index) in form[group.key]" :key="`${group.kind}-${index}`" :data-test="`${group.kind}-point`">
          <label class="fld narrow">
            <span>{{ $t("calibration.pixel") }} X</span>
            <input type="number" step="any" v-model="p.pixelX" :data-test="`${group.kind}-${index}-pixel-x`" required />
          </label>
          <label class="fld narrow">
            <span>{{ $t("calibration.pixel") }} Y</span>
            <input type="number" step="any" v-model="p.pixelY" :data-test="`${group.kind}-${index}-pixel-y`" required />
          </label>
          <label class="fld narrow">
            <span>{{ $t("calibration.plane_") }} X</span>
            <input type="number" step="any" v-model="p.worldX" :data-test="`${group.kind}-${index}-world-x`" required />
          </label>
          <label class="fld narrow">
            <span>{{ $t("calibration.plane_") }} Y</span>
            <input type="number" step="any" v-model="p.worldY" :data-test="`${group.kind}-${index}-world-y`" required />
          </label>
          <NButton
            v-if="form[group.key].length > group.min"
            size="tiny"
            quaternary
            :data-test="`${group.kind}-${index}-remove`"
            @click="form[group.key].splice(index, 1)"
          >
            {{ $t("calibration.removePoint") }}
          </NButton>
        </div>
      </div>
      <NButton size="small" dashed :data-test="`add-${group.kind}-point`" @click="form[group.key].push(emptyPoint())">
        + {{ $t("calibration.addPoint") }}
      </NButton>
    </template>

    <div class="actions">
      <NButton type="primary" attr-type="submit" :loading="saving" :disabled="disabled" data-test="calibration-save">
        {{ $t("calibration.save") }}
      </NButton>
      <NButton quaternary data-test="calibration-cancel" @click="emit('cancel')">
        {{ $t("common.cancel") }}
      </NButton>
    </div>
  </form>
</template>

<style scoped>
.calibration-form { display: flex; flex-direction: column; gap: 10px; margin-top: 12px; }
.calibration-form h5 { margin: 6px 0 0; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.hint { margin: 8px 0 0; }
.setup { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
.points { display: flex; flex-direction: column; gap: 8px; }
.point {
  display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap;
  padding: 10px; border-radius: 9px; background: var(--panel2);
}
.narrow { max-width: 110px; }
.fld :deep(.n-select) { min-width: 170px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-top: 4px; }
</style>
