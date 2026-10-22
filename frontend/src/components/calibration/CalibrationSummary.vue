<script setup>
// A stored calibration as the API kept it: how well its fit and check points
// agree, against the paradigm's tolerance, and the zone shapes in pixels and
// centimetres.
import { computed } from "vue";
import { NDescriptions, NDescriptionsItem, NTag } from "naive-ui";
import { tagType, useExecutionFormat } from "../../composables/useExecutionFormat.js";
import { shapeArea, storedZone } from "./geometry.js";

const props = defineProps({
  calibration: { type: Object, required: true },
  // The paradigm's zones, for their labels in the panel's language.
  zones: { type: Array, default: () => [] },
});
const fmt = useExecutionFormat();

const cm = (value) => `${fmt.number(value, 2)} cm`;
const label = (key) => props.zones.find((z) => z.key === key)?.label || key;
const stored = computed(() => (props.calibration.zones || []).map(storedZone));
// Zones the paradigm marks as required that this calibration has no shape for.
const missing = computed(() => props.zones.filter((z) => z.calibrated && z.required && !stored.value.some((s) => s.key === z.key)));
</script>

<template>
  <div class="summary" data-test="calibration-summary" :data-calibration-status="calibration.status">
    <NDescriptions :column="1" label-placement="left" size="small" bordered>
      <NDescriptionsItem :label="$t('calibration.camera')">{{ calibration.cameraId }}</NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.frame')">
        <span data-test="summary-frame">{{ calibration.frameWidth }} × {{ calibration.frameHeight }}</span>
      </NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.crop')">
        {{ $t("calibration.summary.crop", calibration.crop) }}
      </NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.plane')">{{ $t(`calibration.planes.${calibration.measurementPlane}`) }}</NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.fitError')">
        <span data-test="fit-error">{{ cm(calibration.fitRmsErrorCm) }}</span>
        <span class="muted"> · {{ $t("calibration.summary.points", { count: calibration.fitPoints?.length || 0 }) }}</span>
      </NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.checkError')">
        <span data-test="check-error">{{ cm(calibration.checkRmsErrorCm) }}</span>
        <span class="muted"> · {{ $t("calibration.summary.points", { count: calibration.checkPoints?.length || 0 }) }}</span>
      </NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.maxError')">
        <span data-test="max-error">{{ cm(calibration.checkMaxErrorCm) }}</span>
      </NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.tolerance')">
        <span data-test="tolerance">{{ cm(calibration.toleranceCm) }}</span>
      </NDescriptionsItem>
      <NDescriptionsItem :label="$t('calibration.summary.status')">
        <NTag size="small" round :bordered="false" :type="tagType(calibration.status)" data-test="summary-status">
          {{ $t(`statuses.${calibration.status}`) }}
        </NTag>
      </NDescriptionsItem>
    </NDescriptions>

    <div v-if="stored.length || missing.length" class="zones" data-test="summary-zones">
      <h5>{{ $t("calibration.zones.title") }}</h5>
      <div v-for="(zone, index) in stored" :key="index" class="zone" data-test="summary-zone" :data-zone="zone.key">
        <strong>{{ label(zone.key) }}</strong>
        <span class="muted" v-if="zone.pixels?.circle">
          {{ $t("calibration.summary.circlePx", { x: fmt.number(zone.pixels.circle.x, 1), y: fmt.number(zone.pixels.circle.y, 1), radius: fmt.number(zone.pixels.circle.radius, 1) }) }}
        </span>
        <span class="muted" v-else-if="zone.pixels?.polygon">
          {{ $t("calibration.summary.polygonPx", { count: zone.pixels.polygon.length }) }}
        </span>
        <span v-if="zone.centimetres?.circle" data-test="summary-zone-cm">
          {{ $t("calibration.summary.circleCm", { radius: fmt.number(zone.centimetres.circle.radius, 2), area: fmt.number(shapeArea(zone.centimetres), 1) }) }}
        </span>
        <span v-else-if="zone.centimetres?.polygon" data-test="summary-zone-cm">
          {{ $t("calibration.summary.polygonCm", { area: fmt.number(shapeArea(zone.centimetres), 1) }) }}
        </span>
      </div>
      <p v-if="missing.length" class="err" data-test="summary-missing-zones">
        {{ $t("calibration.summary.missingZones", { zones: missing.map((z) => z.label).join(", ") }) }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.summary { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.zones { display: flex; flex-direction: column; gap: 4px; }
.zones h5 { margin: 0 0 2px; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.zone { display: flex; gap: 8px; flex-wrap: wrap; font-size: 13px; }
.err { margin: 4px 0 0; color: var(--bad); font-size: 13px; }
</style>
