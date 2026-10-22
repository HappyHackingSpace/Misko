<script setup>
// Starts a paradigm trial: a name, the paradigm and the arena (an environment
// revision of that paradigm, which fixes the paradigm version) and notes.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NSelect, NSpace, useMessage } from "naive-ui";
import { trials } from "../../api/trials.js";
import { lookups } from "../../api/tests.js";
import { selectOption } from "../../composables/selectOptions.js";

const props = defineProps({ show: { type: Boolean, default: false } });
const emit = defineEmits(["update:show", "saved"]);
const { t } = useI18n();
const message = useMessage();

const paradigms = ref([]);
const revisions = ref([]);
const form = ref({ name: "", paradigmKey: null, revisionId: null, notes: "" });
const error = ref("");
const saving = ref(false);

const paradigmOptions = computed(() => paradigms.value.map((p) => selectOption("trial-paradigm", p.key, p.name)));
// Every revision of the paradigm's arenas, newest first within an arena.
const arenas = computed(() =>
  revisions.value
    .filter((r) => r.paradigmKey === form.value.paradigmKey)
    .sort((a, b) => a.environmentName.localeCompare(b.environmentName) || b.number - a.number),
);
const arenaOptions = computed(() =>
  arenas.value.map((r) =>
    selectOption("trial-arena", r.id, `${r.environmentName} · ${t("trials.revision", { n: r.number })} · ${t("trials.version", { n: r.paradigmVersion })}`),
  ),
);
const arena = computed(() => arenas.value.find((r) => r.id === form.value.revisionId) || null);
const ready = computed(() => form.value.name.trim() && arena.value);

async function open() {
  form.value = { name: "", paradigmKey: null, revisionId: null, notes: "" };
  error.value = "";
  try {
    const [list, all] = await Promise.all([lookups.paradigms(), lookups.environmentRevisions()]);
    paradigms.value = list.data || [];
    revisions.value = all.data || [];
  } catch (e) {
    error.value = e.message;
  }
}
watch(() => props.show, (shown) => shown && open(), { immediate: true });
// A new paradigm offers its newest arena revision first.
watch(
  () => form.value.paradigmKey,
  () => (form.value.revisionId = arenas.value[0]?.id || null),
);

async function save() {
  saving.value = true;
  error.value = "";
  try {
    const body = {
      name: form.value.name.trim(),
      paradigmKey: arena.value.paradigmKey,
      paradigmVersion: arena.value.paradigmVersion,
      environmentRevisionId: arena.value.id,
    };
    if (form.value.notes.trim()) body.notes = form.value.notes.trim();
    const trial = await trials.create(body);
    message.success(t("trials.created", { name: trial.name }));
    emit("update:show", false);
    emit("saved", trial);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NDrawer :show="show" :width="'min(480px, 100vw)'" placement="right" @update:show="emit('update:show', $event)">
    <NDrawerContent :title="$t('trials.new')" closable>
      <NForm data-test="trial-form" @submit.prevent="save">
        <NFormItem :label="$t('trials.name')" required>
          <NInput v-model:value="form.name" maxlength="120" data-test="trial-form-name" :placeholder="$t('trials.namePlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('trials.paradigm')" required>
          <NSelect v-model:value="form.paradigmKey" :options="paradigmOptions" data-test="trial-form-paradigm" :placeholder="$t('trials.choose')" />
        </NFormItem>
        <NFormItem :label="$t('trials.arena')" required>
          <NSelect
            v-model:value="form.revisionId"
            :options="arenaOptions"
            :disabled="!form.paradigmKey"
            data-test="trial-form-arena"
            :placeholder="$t('trials.choose')"
          />
        </NFormItem>
        <p v-if="form.paradigmKey && !arenas.length" class="muted hint" data-test="trial-form-no-arena">{{ $t("trials.noArena") }}</p>
        <NFormItem :label="$t('trials.notes')">
          <NInput v-model:value="form.notes" type="textarea" maxlength="2000" :autosize="{ minRows: 2, maxRows: 6 }" data-test="trial-form-notes" />
        </NFormItem>
        <NAlert v-if="error" type="error" :title="error" data-test="trial-form-error" />
      </NForm>
      <template #footer>
        <NSpace>
          <NButton @click="emit('update:show', false)">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" :disabled="!ready" data-test="trial-form-save" @click="save">{{ $t("trials.create") }}</NButton>
        </NSpace>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.hint { margin: -12px 0 14px; font-size: 12px; }
</style>
