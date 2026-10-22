<script setup>
// Creates or edits one subject in a side drawer. The API owns the rules (code
// format, unique code); the form sends what was typed and shows its answer.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NRadioButton, NRadioGroup, NSpace, useMessage } from "naive-ui";
import { subjects } from "../../api/subjects.js";
import { SEXES, SPECIES, useSubjectLabels } from "./useSubjectLabels.js";

const props = defineProps({
  show: { type: Boolean, default: false },
  // null creates a new subject.
  subject: { type: Object, default: null },
});
const emit = defineEmits(["update:show", "saved"]);

const { t } = useI18n();
const message = useMessage();
const labels = useSubjectLabels();
const form = ref({ code: "", species: "MOUSE", sex: "UNKNOWN" });
const error = ref("");
const saving = ref(false);
const isNew = computed(() => !props.subject);

watch(
  () => props.show,
  (open) => {
    if (!open) return;
    error.value = "";
    form.value = props.subject
      ? { code: props.subject.code, species: props.subject.species, sex: props.subject.sex }
      : { code: "", species: "MOUSE", sex: "UNKNOWN" };
  },
  { immediate: true },
);

async function save() {
  error.value = "";
  saving.value = true;
  const body = { code: form.value.code.trim(), species: form.value.species, sex: form.value.sex };
  try {
    const saved = isNew.value ? await subjects.create(body) : await subjects.update(props.subject.id, body);
    message.success(t(isNew.value ? "subjects.created" : "subjects.updated", { code: saved.code }));
    emit("saved", saved);
    emit("update:show", false);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NDrawer :show="show" :width="'min(440px, 100vw)'" placement="right" @update:show="emit('update:show', $event)">
    <NDrawerContent :title="isNew ? $t('subjects.new') : $t('subjects.edit')" closable>
      <NForm data-test="subject-form" @submit.prevent="save">
        <NFormItem :label="$t('subjects.code')" required>
          <NInput v-model:value="form.code" data-test="subject-code" maxlength="64" :placeholder="$t('subjects.codePlaceholder')" @keyup.enter="save" />
        </NFormItem>
        <p class="muted hint">{{ $t("subjects.codeHint") }}</p>
        <NFormItem :label="$t('subjects.species')" required>
          <NRadioGroup v-model:value="form.species" data-test="subject-species">
            <NRadioButton v-for="value in SPECIES" :key="value" :value="value" :data-test="`subject-species-${value}`">{{ labels.species(value) }}</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem :label="$t('subjects.sex')" required>
          <NRadioGroup v-model:value="form.sex" data-test="subject-sex">
            <NRadioButton v-for="value in SEXES" :key="value" :value="value" :data-test="`subject-sex-${value}`">{{ labels.sex(value) }}</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NAlert v-if="error" type="error" :title="error" data-test="subject-form-error" />
      </NForm>
      <template #footer>
        <NSpace>
          <NButton @click="emit('update:show', false)">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" :disabled="!form.code.trim()" data-test="subject-save" @click="save">{{ $t("common.save") }}</NButton>
        </NSpace>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.hint { margin: -10px 0 14px; font-size: 12px; }
</style>
