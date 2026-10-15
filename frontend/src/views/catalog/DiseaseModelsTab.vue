<script setup>
// Disease models: how a deficit is produced and how it is confirmed. Groups of
// an experiment point to one; a model that a group uses cannot be deleted.
import { computed, h, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NDataTable, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NPopconfirm, NSelect, NSpace, NTag, useMessage } from "naive-ui";
import { diseaseModels } from "../../api/catalog.js";
import { useAuth } from "../../stores/auth.js";
import { useCatalogList } from "./useCatalogList.js";

const TYPES = ["CHEMICAL", "SURGICAL", "DIET", "STRESS", "OTHER"];

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const canWrite = computed(() => auth.can("study:write"));
const { state, load } = useCatalogList(diseaseModels.list, "inductionType");

const typeLabel = (type) => t(`catalog.inductionTypes.${type}`);
const typeOptions = computed(() => TYPES.map((value) => ({ value, label: typeLabel(value) })));

const deleteError = ref("");
async function remove(row) {
  deleteError.value = "";
  try {
    await diseaseModels.remove(row.id);
    message.success(t("catalog.deleted", { name: row.name }));
    await load();
  } catch (e) {
    deleteError.value = e.message;
  }
}

const columns = computed(() => {
  const cols = [
    { title: t("catalog.name"), key: "name", render: (row) => h("strong", { "data-test": "disease-model-name" }, row.name) },
    {
      title: t("catalog.diseaseModels.inductionType"),
      key: "inductionType",
      width: 150,
      render: (row) => h(NTag, { size: "small", round: true, bordered: false }, () => typeLabel(row.inductionType)),
    },
    { title: t("catalog.diseaseModels.confirmationCriterion"), key: "confirmationCriterion", ellipsis: { tooltip: true }, render: (row) => row.confirmationCriterion || "" },
    { title: t("catalog.description"), key: "description", ellipsis: { tooltip: true }, render: (row) => row.description || "" },
  ];
  if (canWrite.value) {
    cols.push({
      title: "",
      key: "actions",
      width: 150,
      render: (row) =>
        h(NSpace, { size: 6, wrap: false, justify: "end" }, () => [
          h(NButton, { size: "tiny", secondary: true, "data-test": `disease-model-edit-${row.name}`, onClick: () => openEdit(row) }, () => t("common.edit")),
          h(
            NPopconfirm,
            { onPositiveClick: () => remove(row), positiveText: t("common.delete"), negativeText: t("common.cancel") },
            {
              trigger: () => h(NButton, { size: "tiny", secondary: true, type: "error", "data-test": `disease-model-delete-${row.name}` }, () => t("common.delete")),
              default: () => t("catalog.diseaseModels.confirmDelete", { name: row.name }),
            },
          ),
        ]),
    });
  }
  return cols;
});

const drawer = ref(false);
const editing = ref(null);
const form = ref({ name: "", inductionType: "CHEMICAL", confirmationCriterion: "", description: "" });
const formError = ref("");
const saving = ref(false);

function openNew() {
  editing.value = null;
  form.value = { name: "", inductionType: state.filter || "CHEMICAL", confirmationCriterion: "", description: "" };
  formError.value = "";
  drawer.value = true;
}

function openEdit(row) {
  editing.value = row;
  form.value = {
    name: row.name,
    inductionType: row.inductionType,
    confirmationCriterion: row.confirmationCriterion || "",
    description: row.description || "",
  };
  formError.value = "";
  drawer.value = true;
}

async function save() {
  formError.value = "";
  saving.value = true;
  const body = {
    name: form.value.name.trim(),
    inductionType: form.value.inductionType,
    confirmationCriterion: form.value.confirmationCriterion.trim(),
    description: form.value.description.trim(),
  };
  try {
    if (editing.value) {
      await diseaseModels.update(editing.value.id, body);
      message.success(t("catalog.updated", { name: body.name }));
    } else {
      await diseaseModels.create(body);
      message.success(t("catalog.created", { name: body.name }));
    }
    drawer.value = false;
    await load();
  } catch (e) {
    formError.value = e.message;
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div class="toolbar">
    <NInput v-model:value="state.search" class="search" clearable data-test="disease-model-search" :placeholder="$t('catalog.searchName')" />
    <NSelect v-model:value="state.filter" class="filter" clearable data-test="disease-model-type-filter" :options="typeOptions" :placeholder="$t('catalog.diseaseModels.allTypes')" />
    <div class="spacer"></div>
    <NButton v-if="canWrite" type="primary" data-test="disease-model-new" @click="openNew">{{ $t("catalog.diseaseModels.new") }}</NButton>
  </div>
  <NAlert v-if="state.error || deleteError" type="error" :title="state.error || deleteError" data-test="disease-model-list-error" class="alert" closable @close="deleteError = ''" />
  <NDataTable
    :columns="columns"
    :data="state.rows"
    :loading="state.loading"
    :row-key="(row) => row.id"
    :row-props="() => ({ 'data-test': 'disease-model-row' })"
    :pagination="{ pageSize: 20 }"
    :scroll-x="760"
  >
    <template #empty>
      <span class="muted">{{ state.search || state.filter ? $t("catalog.noMatch") : $t("catalog.diseaseModels.empty") }}</span>
    </template>
  </NDataTable>

  <NDrawer v-model:show="drawer" :width="'min(480px, 100vw)'" placement="right">
    <NDrawerContent :title="editing ? $t('catalog.diseaseModels.edit') : $t('catalog.diseaseModels.new')" closable>
      <NForm data-test="disease-model-form" @submit.prevent="save">
        <NFormItem :label="$t('catalog.name')" required>
          <NInput v-model:value="form.name" data-test="disease-model-form-name" maxlength="120" :placeholder="$t('catalog.diseaseModels.namePlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('catalog.diseaseModels.inductionType')" required>
          <NSelect v-model:value="form.inductionType" data-test="disease-model-form-type" :options="typeOptions" />
        </NFormItem>
        <NFormItem :label="$t('catalog.diseaseModels.confirmationCriterion')">
          <NInput
            v-model:value="form.confirmationCriterion"
            type="textarea"
            data-test="disease-model-form-criterion"
            maxlength="2000"
            :autosize="{ minRows: 2, maxRows: 6 }"
            :placeholder="$t('catalog.diseaseModels.criterionPlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('catalog.description')">
          <NInput v-model:value="form.description" type="textarea" data-test="disease-model-form-description" placeholder="" maxlength="5000" :autosize="{ minRows: 3, maxRows: 8 }" />
        </NFormItem>
        <NAlert v-if="formError" type="error" :title="formError" data-test="disease-model-form-error" class="alert" />
      </NForm>
      <template #footer>
        <NSpace>
          <NButton @click="drawer = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" :disabled="!form.name.trim()" data-test="disease-model-form-save" @click="save">{{ $t("common.save") }}</NButton>
        </NSpace>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 12px; }
.search { max-width: 280px; }
.filter { width: 200px; }
.spacer { flex: 1; }
.alert { margin-bottom: 12px; }
@media (max-width: 520px) {
  .search, .filter { max-width: none; width: 100%; }
}
</style>
