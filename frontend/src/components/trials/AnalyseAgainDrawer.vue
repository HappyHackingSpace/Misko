<script setup>
// Analyses the trial's current video again with changed ANALYSIS thresholds.
// The form starts from the arena's values; only what differs from them is
// sent, as overrides of this one analysis. The arena itself never changes.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NDrawer, NDrawerContent, NSpace, useMessage } from "naive-ui";
import { trials } from "../../api/trials.js";
import ParameterFields from "../ParameterFields.vue";

const props = defineProps({
  show: { type: Boolean, default: false },
  trial: { type: Object, required: true },
  // The paradigm version's ANALYSIS parameters.
  parameters: { type: Array, default: () => [] },
});
const emit = defineEmits(["update:show", "submitted"]);
const { t } = useI18n();
const message = useMessage();

const values = ref({});
const error = ref("");
const saving = ref(false);

// The arena's value of a parameter, else the paradigm default.
const arenaValue = (parameter) => props.trial.arena.parameters?.[parameter.key] ?? parameter.default;
const overrides = computed(() =>
  Object.fromEntries(
    props.parameters.filter((p) => values.value[p.key] !== "" && Number(values.value[p.key]) !== arenaValue(p)).map((p) => [p.key, Number(values.value[p.key])]),
  ),
);
const changed = computed(() => Object.keys(overrides.value).length);
const complete = computed(() => props.parameters.every((p) => values.value[p.key] !== "" && values.value[p.key] != null));

watch(
  () => props.show,
  (shown) => {
    if (!shown) return;
    values.value = Object.fromEntries(props.parameters.map((p) => [p.key, arenaValue(p)]));
    error.value = "";
  },
  { immediate: true },
);

async function submit() {
  saving.value = true;
  error.value = "";
  try {
    const analysis = await trials.analyse(props.trial.id, overrides.value);
    message.success(t("trials.again.queued"));
    emit("update:show", false);
    emit("submitted", analysis);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NDrawer :show="show" :width="'min(560px, 100vw)'" placement="right" @update:show="emit('update:show', $event)">
    <NDrawerContent :title="$t('trials.again.title')" closable>
      <p class="muted intro">{{ $t("trials.again.intro") }}</p>
      <div data-test="analyse-again-form">
        <ParameterFields v-if="parameters.length" v-model="values" :parameters="parameters" test-prefix="again" />
        <p v-else class="muted">{{ $t("trials.again.noParameters") }}</p>
      </div>
      <p class="muted changed" data-test="analyse-again-changed">{{ $t("trials.again.changed", { count: changed }) }}</p>
      <NAlert v-if="error" type="error" :title="error" data-test="analyse-again-error" />
      <template #footer>
        <NSpace>
          <NButton @click="emit('update:show', false)">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" :disabled="!complete" data-test="analyse-again-submit" @click="submit">{{ $t("trials.again.submit") }}</NButton>
        </NSpace>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.intro { margin: 0 0 16px; font-size: 13px; }
.changed { margin: 14px 0; font-size: 12.5px; }
</style>
