<script setup>
// The calibration editor shared by an environment revision's default and an
// animal's override on its test's video. Fit and check points, the crop and
// the paradigm's calibrated zones are placed on a frame of a video; every
// number can also be typed. It only shapes the request: the parent decides
// where it goes and which calibration a correction replaces.
//
// The frame comes from `videoUrl` (the test's current video) or, without one,
// from a video file chosen on this computer. That file is read in the browser
// through an object URL and never uploaded.
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NCollapse, NCollapseItem, NInput, NInputNumber, NRadioButton, NRadioGroup, NSelect, NTag } from "naive-ui";
import { selectOption } from "../../composables/selectOptions.js";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";
import CalibrationStage from "./CalibrationStage.vue";
import { clamp, complete, defaultPlane, emptyPoint, placed, presetPoints, presetSize, storedZone, zoneBody } from "./geometry.js";

const props = defineProps({
  // The paradigm version's manifest: its key and zones (labels in the panel's language).
  paradigm: { type: Object, default: null },
  // The environment revision's parameter values, for the point presets.
  parameters: { type: Object, default: () => ({}) },
  // The test's current video; without it the user picks a local file.
  videoUrl: { type: String, default: "" },
  // Values carried over from the calibration this one replaces or starts from.
  initial: { type: Object, default: null },
  // An override names the video frame its points were read from.
  withReferenceFrame: { type: Boolean, default: false },
  correcting: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  note: { type: String, default: "" },
});
const emit = defineEmits(["save", "cancel"]);
const { t } = useI18n();
const fmt = useExecutionFormat();

const PLANES = ["ARENA_FLOOR", "WATER_SURFACE", "APPARATUS_TOP"];
const ZONE_COLOURS = ["var(--warn)", "var(--good)", "var(--bad)", "var(--accent)", "var(--txt)"];
const MIN = { fit: 4, check: 3 };
const planeOptions = computed(() => PLANES.map((plane) => selectOption("plane", plane, t(`calibration.planes.${plane}`))));
const paradigmKey = computed(() => props.paradigm?.key || "");
const zones = computed(() => (props.paradigm?.zones || []).filter((z) => z.calibrated));
const zoneLabel = (key) => zones.value.find((z) => z.key === key)?.label || key;
const zoneColour = (key) => ZONE_COLOURS[Math.max(0, zones.value.findIndex((z) => z.key === key)) % ZONE_COLOURS.length];

// --- Form state --------------------------------------------------------------
const init = props.initial;
const form = ref({
  cameraId: init?.cameraId || "",
  frameWidth: init?.frameWidth || null,
  frameHeight: init?.frameHeight || null,
  crop: init?.crop ? { ...init.crop } : { x: 0, y: 0, width: null, height: null },
  measurementPlane: init?.measurementPlane || defaultPlane(paradigmKey.value),
});
const time = ref(init?.referenceFrameUs ? init.referenceFrameUs / 1_000_000 : 0);
const copyPoint = (p) => ({ ...emptyPoint(), pixelX: p.pixelX, pixelY: p.pixelY, worldX: p.worldX, worldY: p.worldY });
const preset = () => presetPoints(paradigmKey.value, props.parameters, t);
const startPoints = () => {
  if (init?.fitPoints?.length) return { fit: init.fitPoints.map(copyPoint), check: (init.checkPoints || []).map(copyPoint) };
  return preset() || { fit: Array.from({ length: 4 }, () => emptyPoint()), check: Array.from({ length: 3 }, () => emptyPoint()) };
};
const points = ref(startPoints());
let nextShape = 1;
const shapes = ref(
  (init?.zones || [])
    .map(storedZone)
    .filter((z) => z.pixels && zones.value.some((known) => known.key === z.key))
    .map((z) => ({ id: nextShape++, key: z.key, circle: z.pixels.circle ? { ...z.pixels.circle } : null, polygon: z.pixels.polygon ? z.pixels.polygon.map((p) => ({ ...p })) : null })),
);

// --- Frame source --------------------------------------------------------------
const localUrl = ref("");
const localName = ref("");
const fileInput = ref(null);
const videoError = ref(false);
const src = computed(() => props.videoUrl || localUrl.value);
const fromVideo = ref(false);

function pickFile(event) {
  const file = event.target.files?.[0];
  if (!file) return;
  if (localUrl.value) URL.revokeObjectURL(localUrl.value);
  localUrl.value = URL.createObjectURL(file);
  localName.value = file.name;
  videoError.value = false;
  event.target.value = "";
}
onBeforeUnmount(() => localUrl.value && URL.revokeObjectURL(localUrl.value));

// The frame size is the video's own; a crop that no longer fits becomes the whole frame.
function onMetadata({ width, height }) {
  if (!width || !height) return;
  form.value.frameWidth = width;
  form.value.frameHeight = height;
  fromVideo.value = true;
  const c = form.value.crop;
  if (!(c.width > 0 && c.height > 0) || c.x + c.width > width || c.y + c.height > height) fullFrame();
}
function fullFrame() {
  form.value.crop = { x: 0, y: 0, width: form.value.frameWidth || null, height: form.value.frameHeight || null };
}

// --- Tools ---------------------------------------------------------------------
// "fit" and "check" place points, "crop" drags the crop, "zones" draws shapes.
const mode = ref("fit");
const selected = ref({ fit: 0, check: 0 });
const drawing = ref(null); // { key, tool: "polygon" | "circle", points: [] }
const draft = ref(null); // a circle being dragged out: { key, x, y, radius }
const hover = ref(null);
const drag = ref(null);
const stage = ref(null);
const scale = computed(() => stage.value?.scale || 1);
const grab = computed(() => 12 * scale.value);
const modeOptions = computed(() => [
  { value: "fit", label: t("calibration.tools.fit") },
  { value: "check", label: t("calibration.tools.check") },
  { value: "crop", label: t("calibration.tools.crop") },
  ...(zones.value.length ? [{ value: "zones", label: t("calibration.tools.zones") }] : []),
]);
watch(mode, (value) => {
  if (value === "zones") return;
  drawing.value = null;
  draft.value = null;
});

const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);
const short = (group, index) => `${t(`calibration.points.${group}Short`)}${index + 1}`;

// The point the next click places: the selected one while it has no pixel,
// else the first without one, else a new one.
function targetPoint(group) {
  const list = points.value[group];
  const chosen = list[selected.value[group]];
  if (chosen && !placed(chosen)) return selected.value[group];
  const free = list.findIndex((p) => !placed(p));
  if (free >= 0) return free;
  list.push(emptyPoint());
  return list.length - 1;
}

function setPixel(group, index, { x, y }) {
  const p = points.value[group][index];
  p.pixelX = Math.round(x * 10) / 10;
  p.pixelY = Math.round(y * 10) / 10;
}

function press(e) {
  const kind = e.target.kind;
  if (mode.value === "fit" || mode.value === "check") {
    const group = mode.value;
    let index;
    if (kind === "point" && e.target.group === group) index = Number(e.target.index);
    else {
      index = targetPoint(group);
      setPixel(group, index, e);
    }
    selected.value[group] = index;
    drag.value = { kind: "point", group, index };
    return;
  }
  if (mode.value === "crop") {
    drag.value = { kind: "crop", from: { x: e.x, y: e.y }, before: { ...form.value.crop } };
    return;
  }
  // Zones: while drawing, a click on another shape's corner or centre snaps to
  // it, so neighbouring zones share their edges exactly. Otherwise a handle is
  // dragged to adjust its shape.
  const shape = shapes.value.find((s) => s.id === Number(e.target.shape));
  const index = Number(e.target.index);
  const d = drawing.value;
  if (!d) {
    if (shape && (kind === "vertex" || kind === "centre" || kind === "edge")) {
      drag.value = { kind, shape, index, offset: shape.circle ? { x: e.x - shape.circle.x, y: e.y - shape.circle.y } : null };
    }
    return;
  }
  let at = { x: e.x, y: e.y };
  if (shape && kind === "vertex") at = { ...shape.polygon[index] };
  else if (shape && kind === "centre") at = { x: shape.circle.x, y: shape.circle.y };
  if (d.tool === "circle") {
    draft.value = { key: d.key, ...at, radius: 0 };
    drag.value = { kind: "circle" };
    return;
  }
  if (d.points.length >= 3 && distance(at, d.points[0]) <= grab.value) closePolygon();
  else d.points.push(at);
}

function move(e) {
  hover.value = { x: e.x, y: e.y };
  const g = drag.value;
  if (!g) return;
  if (g.kind === "point") setPixel(g.group, g.index, e);
  else if (g.kind === "crop") {
    const x = Math.round(Math.min(g.from.x, e.x));
    const y = Math.round(Math.min(g.from.y, e.y));
    form.value.crop = { x, y, width: Math.round(Math.max(g.from.x, e.x)) - x, height: Math.round(Math.max(g.from.y, e.y)) - y };
  } else if (g.kind === "circle") draft.value.radius = distance(draft.value, e);
  else if (g.kind === "vertex") Object.assign(g.shape.polygon[g.index], { x: e.x, y: e.y });
  else if (g.kind === "centre") {
    g.shape.circle.x = clamp(e.x - g.offset.x, 0, form.value.frameWidth);
    g.shape.circle.y = clamp(e.y - g.offset.y, 0, form.value.frameHeight);
  } else if (g.kind === "edge") g.shape.circle.radius = Math.max(1, distance(g.shape.circle, e));
}

function release() {
  const g = drag.value;
  drag.value = null;
  if (!g) return;
  if (g.kind === "crop" && !(form.value.crop.width >= 4 && form.value.crop.height >= 4)) form.value.crop = g.before;
  if (g.kind === "point") {
    // Move on to the next point still waiting for its place.
    const list = points.value[g.group];
    const next = list.findIndex((p) => !placed(p));
    if (next >= 0) selected.value[g.group] = next;
  }
  if (g.kind === "circle") {
    const c = draft.value;
    draft.value = null;
    if (c.radius >= 2) shapes.value.push({ id: nextShape++, key: c.key, circle: { x: c.x, y: c.y, radius: c.radius }, polygon: null });
  }
}

function startDrawing(key, tool) {
  mode.value = "zones";
  draft.value = null;
  drawing.value = { key, tool, points: [] };
}
function closePolygon() {
  const d = drawing.value;
  if (!d || d.points.length < 3) return;
  shapes.value.push({ id: nextShape++, key: d.key, circle: null, polygon: d.points });
  drawing.value = { ...d, points: [] };
}
// Stopping keeps a polygon that already has three corners.
function stopDrawing() {
  if (drawing.value?.points.length >= 3) closePolygon();
  drawing.value = null;
}
const undoVertex = () => drawing.value?.points.pop();
const removeShape = (id) => (shapes.value = shapes.value.filter((s) => s.id !== id));
function redraw(shape) {
  removeShape(shape.id);
  startDrawing(shape.key, shape.circle ? "circle" : "polygon");
}
const shapesOf = (key) => shapes.value.filter((s) => s.key === key);

function applyPreset() {
  const p = preset();
  if (!p) return;
  points.value = p;
  selected.value = { fit: 0, check: 0 };
  mode.value = "fit";
}
const presetHint = computed(() => {
  const size = presetSize(paradigmKey.value, props.parameters);
  if (!size) return "";
  if (size.kind === "tank") return t("calibration.presets.tank", { diameter: fmt.number(size.diameter, 1) });
  return t("calibration.presets.box", { width: fmt.number(size.width, 1), depth: fmt.number(size.depth, 1) });
});
const hasPreset = computed(() => !!preset());

function addPoint(group) {
  points.value[group].push(emptyPoint());
  selected.value[group] = points.value[group].length - 1;
  mode.value = group;
}
function removePoint(group, index) {
  points.value[group].splice(index, 1);
  selected.value[group] = Math.min(selected.value[group], points.value[group].length - 1);
}
function clearPixels() {
  for (const group of ["fit", "check"]) for (const p of points.value[group]) Object.assign(p, { pixelX: null, pixelY: null });
  selected.value = { fit: 0, check: 0 };
}

// --- Overlay geometry ------------------------------------------------------------
const W = computed(() => Number(form.value.frameWidth) || 0);
const H = computed(() => Number(form.value.frameHeight) || 0);
const cropPath = computed(() => {
  const c = form.value.crop;
  if (!(c.width > 0 && c.height > 0)) return "";
  return `M0 0H${W.value}V${H.value}H0Z M${c.x} ${c.y}h${c.width}v${c.height}h${-c.width}Z`;
});
const polygonPoints = (list) => list.map((p) => `${p.x},${p.y}`).join(" ");
const labelAt = (shape) => (shape.circle ? { x: shape.circle.x, y: shape.circle.y } : shape.polygon[0]);
const drawingPath = computed(() => {
  const d = drawing.value;
  if (!d || d.tool !== "polygon" || !d.points.length) return "";
  const list = hover.value ? [...d.points, hover.value] : d.points;
  return polygonPoints(list);
});
const cursor = computed(() => {
  if (mode.value === "zones" && !drawing.value) return "default";
  return "crosshair";
});

// --- Readiness and the request ---------------------------------------------------
const problems = computed(() => {
  const list = [];
  if (!form.value.cameraId.trim()) list.push(t("calibration.missing.camera"));
  if (!(W.value > 0 && H.value > 0)) list.push(t("calibration.missing.frame"));
  const c = form.value.crop;
  if (!(c.width > 0 && c.height > 0)) list.push(t("calibration.missing.crop"));
  for (const group of ["fit", "check"]) {
    const own = points.value[group];
    if (own.length < MIN[group]) list.push(t(`calibration.missing.${group}Count`, { count: MIN[group] }));
    const incomplete = own.map((p, i) => (complete(p) ? null : short(group, i))).filter(Boolean);
    if (incomplete.length) list.push(t("calibration.missing.points", { points: incomplete.join(", ") }));
  }
  for (const zone of zones.value) {
    if (zone.required && !shapesOf(zone.key).length) list.push(t("calibration.missing.zone", { zone: zone.label }));
  }
  if (drawing.value?.points?.length) list.push(t("calibration.missing.openPolygon"));
  return list;
});

const asPoint = (p) => ({ pixelX: Number(p.pixelX), pixelY: Number(p.pixelY), worldX: Number(p.worldX), worldY: Number(p.worldY) });

function submit() {
  if (problems.value.length) return;
  const c = form.value.crop;
  const body = {
    cameraId: form.value.cameraId.trim(),
    frameWidth: Math.round(W.value),
    frameHeight: Math.round(H.value),
    crop: { x: Math.round(Number(c.x) || 0), y: Math.round(Number(c.y) || 0), width: Math.round(Number(c.width)), height: Math.round(Number(c.height)) },
    measurementPlane: form.value.measurementPlane,
    fitPoints: points.value.fit.map(asPoint),
    checkPoints: points.value.check.map(asPoint),
  };
  // Only paradigms with calibrated zones send them.
  if (shapes.value.length) body.zones = shapes.value.map(zoneBody);
  // The API counts microseconds from the start of the video.
  if (props.withReferenceFrame) body.referenceFrameUs = Math.round(Number(time.value || 0) * 1_000_000);
  emit("save", body);
}

const referenceSeconds = computed({
  get: () => time.value,
  set: (value) => (time.value = Number(value) || 0),
});
</script>

<template>
  <form class="calibrator" data-test="calibration-form" @submit.prevent="submit">
    <p class="muted" v-if="correcting" data-test="calibration-correcting">{{ $t("calibration.correcting") }}</p>
    <p class="muted" v-if="note" data-test="calibration-note">{{ note }}</p>

    <div class="setup">
      <label class="fld">
        <span>{{ $t("calibration.camera") }}</span>
        <NInput v-model:value="form.cameraId" placeholder="" size="small" maxlength="120" data-test="camera-id" />
      </label>
      <label class="fld">
        <span>{{ $t("calibration.plane") }}</span>
        <NSelect v-model:value="form.measurementPlane" size="small" data-test="plane" :options="planeOptions" />
      </label>
      <div class="fld" v-if="!videoUrl">
        <span>{{ $t("calibration.source.local") }}</span>
        <div class="file">
          <NButton size="small" secondary data-test="calibration-pick-video" @click="fileInput.click()">
            {{ localName ? $t("calibration.source.change") : $t("calibration.source.pick") }}
          </NButton>
          <span class="muted name" v-if="localName" data-test="calibration-video-name">{{ localName }}</span>
          <input ref="fileInput" type="file" accept="video/*" class="hidden" data-test="calibration-video-file" @change="pickFile" />
        </div>
      </div>
    </div>
    <p class="muted hint" v-if="!videoUrl">{{ $t("calibration.source.localHint") }}</p>
    <p class="err" v-if="videoError" data-test="calibration-video-error">{{ $t("calibration.source.error") }}</p>

    <div class="tools">
      <NRadioGroup v-model:value="mode" size="small" data-test="calibration-mode">
        <NRadioButton v-for="option in modeOptions" :key="option.value" :value="option.value" :data-test="`mode-${option.value}`">
          {{ option.label }}
        </NRadioButton>
      </NRadioGroup>
      <span class="muted hint" data-test="calibration-tool-hint">{{ $t(`calibration.toolHints.${mode}`) }}</span>
    </div>

    <CalibrationStage
      ref="stage"
      v-model:time="time"
      :src="src"
      :frame-width="W"
      :frame-height="H"
      :cursor="cursor"
      @metadata="onMetadata"
      @error="videoError = true"
      @press="press"
      @move="move"
      @release="release"
    >
      <template #default>
        <path v-if="cropPath" :d="cropPath" fill-rule="evenodd" class="shade" data-test="crop-shade" />
        <rect
          v-if="cropPath"
          :x="form.crop.x" :y="form.crop.y" :width="form.crop.width" :height="form.crop.height"
          class="crop" vector-effect="non-scaling-stroke" data-test="crop-rect"
        />

        <g v-for="shape in shapes" :key="shape.id" :style="{ color: zoneColour(shape.key) }" data-test="zone-shape" :data-zone="shape.key">
          <circle v-if="shape.circle" :cx="shape.circle.x" :cy="shape.circle.y" :r="shape.circle.radius" class="zone" vector-effect="non-scaling-stroke" />
          <polygon v-else :points="polygonPoints(shape.polygon)" class="zone" vector-effect="non-scaling-stroke" />
          <text :x="labelAt(shape).x" :y="labelAt(shape).y - 8 * scale" :font-size="12 * scale" class="zone-label" text-anchor="middle">
            {{ zoneLabel(shape.key) }}
          </text>
          <template v-if="mode === 'zones'">
            <template v-if="shape.circle">
              <circle :cx="shape.circle.x" :cy="shape.circle.y" :r="6 * scale" class="handle" data-kind="centre" :data-shape="shape.id" vector-effect="non-scaling-stroke" />
              <circle :cx="shape.circle.x + shape.circle.radius" :cy="shape.circle.y" :r="6 * scale" class="handle" data-kind="edge" :data-shape="shape.id" vector-effect="non-scaling-stroke" />
            </template>
            <circle
              v-for="(p, index) in shape.polygon || []"
              :key="index"
              :cx="p.x" :cy="p.y" :r="6 * scale"
              class="handle" data-kind="vertex" :data-shape="shape.id" :data-index="index" vector-effect="non-scaling-stroke"
            />
          </template>
        </g>

        <g v-if="drawing" :style="{ color: zoneColour(drawing.key) }" data-test="zone-drawing">
          <polyline v-if="drawingPath" :points="drawingPath" class="zone open" vector-effect="non-scaling-stroke" />
          <circle
            v-for="(p, index) in drawing.points"
            :key="index"
            :cx="p.x" :cy="p.y" :r="(index === 0 && drawing.points.length >= 3 ? 9 : 5) * scale"
            :class="['handle', { closer: index === 0 && drawing.points.length >= 3 }]"
            vector-effect="non-scaling-stroke"
          />
        </g>
        <circle v-if="draft" :cx="draft.x" :cy="draft.y" :r="draft.radius" class="zone" :style="{ color: zoneColour(draft.key) }" vector-effect="non-scaling-stroke" />

        <template v-for="group in ['fit', 'check']" :key="group">
          <g
            v-for="(p, index) in points[group]"
            v-show="p.pixelX !== null && p.pixelY !== null && p.pixelX !== '' && p.pixelY !== ''"
            :key="`${group}-${index}`"
            :class="['point', group, { dim: mode !== group, active: mode === group && selected[group] === index }]"
            :data-test="`${group}-marker`"
          >
            <circle :cx="p.pixelX" :cy="p.pixelY" :r="7 * scale" data-kind="point" :data-group="group" :data-index="index" vector-effect="non-scaling-stroke" />
            <text :x="Number(p.pixelX) + 10 * scale" :y="Number(p.pixelY) - 8 * scale" :font-size="12 * scale">{{ short(group, index) }}</text>
          </g>
        </template>
      </template>
      <template #empty>
        <span v-if="videoUrl">{{ $t("calibration.source.loading") }}</span>
        <span v-else>{{ $t("calibration.source.empty") }}</span>
      </template>
    </CalibrationStage>

    <p class="muted small" v-if="W && H" data-test="calibration-frame-size">
      {{ $t("calibration.stage.frameSize", { width: W, height: H }) }}
      <template v-if="fromVideo"> · {{ $t("calibration.stage.fromVideo") }}</template>
      <template v-if="withReferenceFrame"> · {{ $t("calibration.stage.reference", { time: fmt.number(time, 3) }) }}</template>
    </p>

    <!-- Points -->
    <section v-if="mode === 'fit' || mode === 'check'" class="panel" data-test="calibration-points">
      <div class="row-head">
        <h5>{{ $t(`calibration.${mode}Points`) }}</h5>
        <div class="row-actions">
          <NButton v-if="hasPreset" size="tiny" secondary data-test="calibration-preset" @click="applyPreset">{{ $t("calibration.presets.apply") }}</NButton>
          <NButton size="tiny" quaternary data-test="calibration-clear-pixels" @click="clearPixels">{{ $t("calibration.points.clearPixels") }}</NButton>
        </div>
      </div>
      <p class="muted hint">{{ presetHint || $t("calibration.presets.free") }}</p>
      <div
        v-for="(p, index) in points[mode]"
        :key="`${mode}-${index}`"
        :class="['point-row', { active: selected[mode] === index }]"
        :data-test="`${mode}-point`"
        @click="selected[mode] = index"
      >
        <NTag size="small" round :bordered="false" :type="placed(p) ? (mode === 'fit' ? 'info' : 'success') : 'default'">{{ short(mode, index) }}</NTag>
        <span class="label muted" v-if="p.label">{{ p.label }}</span>
        <label class="num">
          <span>{{ $t("calibration.pixel") }} X</span>
          <NInputNumber placeholder="" v-model:value="p.pixelX" size="small" :show-button="false" :data-test="`${mode}-${index}-pixel-x`" />
        </label>
        <label class="num">
          <span>{{ $t("calibration.pixel") }} Y</span>
          <NInputNumber placeholder="" v-model:value="p.pixelY" size="small" :show-button="false" :data-test="`${mode}-${index}-pixel-y`" />
        </label>
        <label class="num">
          <span>{{ $t("calibration.plane_") }} X</span>
          <NInputNumber placeholder="" v-model:value="p.worldX" size="small" :show-button="false" :data-test="`${mode}-${index}-world-x`" />
        </label>
        <label class="num">
          <span>{{ $t("calibration.plane_") }} Y</span>
          <NInputNumber placeholder="" v-model:value="p.worldY" size="small" :show-button="false" :data-test="`${mode}-${index}-world-y`" />
        </label>
        <NButton
          v-if="points[mode].length > MIN[mode]"
          size="tiny"
          quaternary
          :data-test="`${mode}-${index}-remove`"
          @click.stop="removePoint(mode, index)"
        >
          {{ $t("calibration.removePoint") }}
        </NButton>
      </div>
      <NButton size="small" dashed :data-test="`add-${mode}-point`" @click="addPoint(mode)">+ {{ $t("calibration.addPoint") }}</NButton>
    </section>

    <!-- Crop -->
    <section v-if="mode === 'crop'" class="panel" data-test="calibration-crop">
      <div class="row-head">
        <h5>{{ $t("calibration.crop") }}</h5>
        <NButton size="tiny" secondary :disabled="!W" data-test="crop-full" @click="fullFrame">{{ $t("calibration.cropFull") }}</NButton>
      </div>
      <div class="nums">
        <label class="num" v-for="field in ['x', 'y', 'width', 'height']" :key="field">
          <span>{{ $t(`calibration.cropFields.${field}`) }}</span>
          <NInputNumber placeholder="" v-model:value="form.crop[field]" size="small" :min="0" :precision="0" :show-button="false" :data-test="`crop-${field}`" />
        </label>
      </div>
    </section>

    <!-- Zones -->
    <section v-if="mode === 'zones'" class="panel" data-test="calibration-zones">
      <h5>{{ $t("calibration.zones.title") }}</h5>
      <p class="muted hint">{{ $t("calibration.zones.hint") }}</p>
      <div v-for="zone in zones" :key="zone.key" class="zone-row" data-test="zone-row" :data-zone="zone.key">
        <div class="zone-head">
          <span class="swatch" :style="{ background: zoneColour(zone.key) }"></span>
          <strong>{{ zone.label }}</strong>
          <NTag size="small" round :bordered="false" :type="zone.required ? (shapesOf(zone.key).length ? 'success' : 'warning') : 'default'" data-test="zone-required">
            {{ zone.required ? $t("calibration.zones.required") : $t("calibration.zones.optional") }}
          </NTag>
          <span class="spacer"></span>
          <NButton size="tiny" secondary :type="drawing?.key === zone.key && drawing.tool === 'polygon' ? 'primary' : 'default'" data-test="zone-draw-polygon" @click="startDrawing(zone.key, 'polygon')">
            {{ $t("calibration.zones.polygon") }}
          </NButton>
          <NButton size="tiny" secondary :type="drawing?.key === zone.key && drawing.tool === 'circle' ? 'primary' : 'default'" data-test="zone-draw-circle" @click="startDrawing(zone.key, 'circle')">
            {{ $t("calibration.zones.circle") }}
          </NButton>
        </div>
        <div class="shapes" v-if="shapesOf(zone.key).length">
          <div v-for="(shape, index) in shapesOf(zone.key)" :key="shape.id" class="shape" data-test="zone-shape-row">
            <span class="muted">
              {{ shape.circle
                ? $t("calibration.zones.circleShape", { n: index + 1, radius: fmt.number(shape.circle.radius, 1) })
                : $t("calibration.zones.polygonShape", { n: index + 1, count: shape.polygon.length }) }}
            </span>
            <NButton size="tiny" quaternary data-test="zone-redraw" @click="redraw(shape)">{{ $t("calibration.zones.redraw") }}</NButton>
            <NButton size="tiny" quaternary type="error" data-test="zone-delete" @click="removeShape(shape.id)">{{ $t("calibration.zones.delete") }}</NButton>
          </div>
        </div>
      </div>
      <div v-if="drawing" class="drawing" data-test="zone-drawing-bar">
        <span class="muted">
          {{ $t(`calibration.zones.drawing.${drawing.tool}`, { zone: zoneLabel(drawing.key), count: drawing.points.length }) }}
        </span>
        <NButton v-if="drawing.tool === 'polygon'" size="tiny" secondary :disabled="!drawing.points.length" data-test="zone-undo" @click="undoVertex">
          {{ $t("calibration.zones.undo") }}
        </NButton>
        <NButton v-if="drawing.tool === 'polygon'" size="tiny" secondary :disabled="drawing.points.length < 3" data-test="zone-close" @click="closePolygon">
          {{ $t("calibration.zones.close") }}
        </NButton>
        <NButton size="tiny" quaternary data-test="zone-stop" @click="stopDrawing">{{ $t("calibration.zones.stop") }}</NButton>
      </div>
    </section>

    <NCollapse class="manual" data-test="calibration-manual">
      <NCollapseItem :title="$t('calibration.manual.title')" name="manual">
        <p class="muted hint">{{ $t("calibration.manual.hint") }}</p>
        <div class="nums">
          <label class="num">
            <span>{{ $t("calibration.manual.frameWidth") }}</span>
            <NInputNumber placeholder="" v-model:value="form.frameWidth" size="small" :min="1" :max="16384" :precision="0" :show-button="false" :disabled="fromVideo" data-test="frame-width" />
          </label>
          <label class="num">
            <span>{{ $t("calibration.manual.frameHeight") }}</span>
            <NInputNumber placeholder="" v-model:value="form.frameHeight" size="small" :min="1" :max="16384" :precision="0" :show-button="false" :disabled="fromVideo" data-test="frame-height" />
          </label>
          <label class="num" v-for="field in ['x', 'y', 'width', 'height']" :key="field">
            <span>{{ $t("calibration.crop") }} {{ $t(`calibration.cropFields.${field}`) }}</span>
            <NInputNumber placeholder="" v-model:value="form.crop[field]" size="small" :min="0" :precision="0" :show-button="false" :data-test="`manual-crop-${field}`" />
          </label>
          <label class="num" v-if="withReferenceFrame">
            <span>{{ $t("calibration.referenceFrame") }}</span>
            <NInputNumber placeholder="" v-model:value="referenceSeconds" size="small" :min="0" :precision="3" :show-button="false" data-test="reference-frame" />
          </label>
        </div>
      </NCollapseItem>
    </NCollapse>

    <ul v-if="problems.length" class="problems muted" data-test="calibration-problems">
      <li v-for="problem in problems" :key="problem">{{ problem }}</li>
    </ul>

    <div class="actions">
      <NButton type="primary" attr-type="submit" :loading="saving" :disabled="disabled || problems.length > 0" data-test="calibration-save">
        {{ $t("calibration.save") }}
      </NButton>
      <NButton quaternary data-test="calibration-cancel" @click="emit('cancel')">{{ $t("common.cancel") }}</NButton>
    </div>
  </form>
</template>

<style scoped>
.calibrator { display: flex; flex-direction: column; gap: 10px; margin-top: 12px; min-width: 0; }
.calibrator h5 { margin: 0; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.hint { margin: 0; font-size: 12px; }
.small { margin: 0; font-size: 12px; }
.err { margin: 0; color: var(--bad); font-size: 13px; }
.setup { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.fld > span { font-size: 12px; color: var(--muted); }
.fld :deep(.n-select) { min-width: 170px; }
.file { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; min-width: 0; }
.name { font-size: 12px; overflow-wrap: anywhere; }
.hidden { display: none; }
.tools { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.panel { display: flex; flex-direction: column; gap: 8px; padding: 10px; border-radius: 9px; background: var(--panel2); }
.row-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.row-actions { display: flex; gap: 6px; flex-wrap: wrap; }
.point-row {
  display: flex; gap: 10px; align-items: flex-end; flex-wrap: wrap;
  padding: 8px; border-radius: 8px; border: 1px solid transparent; cursor: pointer;
}
.point-row.active { border-color: var(--accent); background: var(--panel); }
.point-row .label { align-self: center; font-size: 12px; min-width: 90px; }
.nums { display: flex; gap: 10px; flex-wrap: wrap; }
.num { display: flex; flex-direction: column; gap: 4px; width: 104px; }
.num > span { font-size: 12px; color: var(--muted); }
.zone-row { display: flex; flex-direction: column; gap: 6px; padding: 8px; border-radius: 8px; background: var(--panel); }
.zone-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.spacer { flex: 1; }
.swatch { width: 12px; height: 12px; border-radius: 3px; flex: none; }
.shapes { display: flex; flex-direction: column; gap: 2px; padding-left: 20px; }
.shape { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px; }
.drawing { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 12px; }
.problems { margin: 0; padding-left: 18px; font-size: 12px; }
.actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }

/* Drawing layer */
.shade { fill: var(--bg); opacity: .45; pointer-events: none; }
.crop { fill: none; stroke: var(--txt); stroke-width: 1.5; stroke-dasharray: 6 4; pointer-events: none; }
.zone { fill: currentColor; fill-opacity: .18; stroke: currentColor; stroke-width: 2; pointer-events: none; }
.zone.open { fill: none; stroke-dasharray: 5 4; }
.zone-label { fill: currentColor; font-weight: 600; pointer-events: none; paint-order: stroke; stroke: var(--bg); stroke-width: 3px; }
.handle { fill: var(--bg); stroke: currentColor; stroke-width: 2; cursor: move; }
.handle.closer { fill: currentColor; cursor: pointer; }
.point circle { stroke: var(--bg); stroke-width: 2; cursor: move; }
.point text { font-weight: 700; pointer-events: none; paint-order: stroke; stroke: var(--bg); stroke-width: 3px; }
.point.fit circle, .point.fit text { fill: var(--accent); }
.point.check circle, .point.check text { fill: var(--good); }
.point.dim { opacity: .45; }
.point.dim circle { pointer-events: none; }
.point.active circle { stroke: var(--txt); stroke-width: 3; }
</style>
