<script setup>
// Substances: active compounds, reference drugs, vehicles and disease-inducing
// agents. The kind only guides pickers; the role a substance plays in a study
// comes from the regimen that uses it.
import { computed, h, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NDataTable, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NPopconfirm, NSelect, NSpace, NTag, useMessage } from "naive-ui";
import { substances } from "../../api/catalog.js";
import { useAuth } from "../../stores/auth.js";
import { useCatalogList } from "./useCatalogList.js";

const KINDS = ["COMPOUND", "REFERENCE_DRUG", "VEHICLE", "INDUCER"];
const KIND_TAG = { COMPOUND: "info", REFERENCE_DRUG: "success", VEHICLE: "default", INDUCER: "warning" };

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const canWrite = computed(() => auth.can("study:write"));
const { state, load } = useCatalogList(substances.list, "kind");

const kindLabel = (kind) => t(`catalog.kinds.${kind}`);
const kindOptions = computed(() => KINDS.map((value) => ({ value, label: kindLabel(value) })));

const deleteError = ref("");
async function remove(row) {
  deleteError.value = "";
  try {
    await substances.remove(row.id);
    message.success(t("catalog.deleted", { name: row.name }));
    await load();
  } catch (e) {
    deleteError.value = e.message;
  }
}

const columns = computed(() => {
  const cols = [
    { title: t("catalog.name"), key: "name", render: (row) => h("strong", { "data-test": "substance-name" }, row.name) },
    {
      title: t("catalog.substances.kind"),
      key: "kind",
      width: 170,
      render: (row) => h(NTag, { size: "small", round: true, bordered: false, type: KIND_TAG[row.kind] }, () => kindLabel(row.kind)),
    },
    { title: t("catalog.description"), key: "description", ellipsis: { tooltip: true }, render: (row) => row.description || "" },
  ];
  if (canWrite.value) {
    cols.push({
      title: "",
      key: "actions",
      width: 150,
      render: (row) =>
        h(NSpace, { size: 6, wrap: false, justify: "end" }, () => [
          h(NButton, { size: "tiny", secondary: true, "data-test": `substance-edit-${row.name}`, onClick: () => openEdit(row) }, () => t("common.edit")),
          h(
            NPopconfirm,
            { onPositiveClick: () => remove(row), positiveText: t("common.delete"), negativeText: t("common.cancel") },
            {
              trigger: () => h(NButton, { size: "tiny", secondary: true, type: "error", "data-test": `substance-delete-${row.name}` }, () => t("common.delete")),
              default: () => t("catalog.substances.confirmDelete", { name: row.name }),
            },
          ),
        ]),
    });
  }
  return cols;
});

// Create and edit share one drawer.
const drawer = ref(false);
const editing = ref(null);
const form = ref({ name: "", kind: "COMPOUND", description: "" });
const formError = ref("");
const saving = ref(false);

function openNew() {
  editing.value = null;
  form.value = { name: "", kind: state.filter || "COMPOUND", description: "" };
  formError.value = "";
  drawer.value = true;
}

function openEdit(row) {
  editing.value = row;
  form.value = { name: row.name, kind: row.kind, description: row.description || "" };
  formError.value = "";
  drawer.value = true;
}

async function save() {
  formError.value = "";
  saving.value = true;
  const body = { name: form.value.name.trim(), kind: form.value.kind, description: form.value.description.trim() };
  try {
    if (editing.value) {
      await substances.update(editing.value.id, body);
      message.success(t("catalog.updated", { name: body.name }));
    } else {
      await substances.create(body);
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
    <NInput v-model:value="state.search" class="search" clearable data-test="substance-search" :placeholder="$t('catalog.searchName')" />
    <NSelect v-model:value="state.filter" class="filter" clearable data-test="substance-kind-filter" :options="kindOptions" :placeholder="$t('catalog.substances.allKinds')" />
    <div class="spacer"></div>
    <NButton v-if="canWrite" type="primary" data-test="substance-new" @click="openNew">{{ $t("catalog.substances.new") }}</NButton>
  </div>
  <NAlert v-if="state.error || deleteError" type="error" :title="state.error || deleteError" data-test="substance-list-error" class="alert" closable @close="deleteError = ''" />
  <NDataTable
    :columns="columns"
    :data="state.rows"
    :loading="state.loading"
    :row-key="(row) => row.id"
    :row-props="() => ({ 'data-test': 'substance-row' })"
    :pagination="{ pageSize: 20 }"
    :scroll-x="640"
  >
    <template #empty>
      <span class="muted">{{ state.search || state.filter ? $t("catalog.noMatch") : $t("catalog.substances.empty") }}</span>
    </template>
  </NDataTable>

  <NDrawer v-model:show="drawer" :width="'min(480px, 100vw)'" placement="right">
    <NDrawerContent :title="editing ? $t('catalog.substances.edit') : $t('catalog.substances.new')" closable>
      <NForm data-test="substance-form" @submit.prevent="save">
        <NFormItem :label="$t('catalog.name')" required>
          <NInput v-model:value="form.name" data-test="substance-form-name" maxlength="120" :placeholder="$t('catalog.substances.namePlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('catalog.substances.kind')" required>
          <NSelect v-model:value="form.kind" data-test="substance-form-kind" :options="kindOptions" />
        </NFormItem>
        <p class="muted hint">{{ $t(`catalog.kindHints.${form.kind}`) }}</p>
        <NFormItem :label="$t('catalog.description')">
          <NInput v-model:value="form.description" type="textarea" data-test="substance-form-description" placeholder="" maxlength="5000" :autosize="{ minRows: 3, maxRows: 8 }" />
        </NFormItem>
        <NAlert v-if="formError" type="error" :title="formError" data-test="substance-form-error" class="alert" />
      </NForm>
      <template #footer>
        <NSpace>
          <NButton @click="drawer = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" :disabled="!form.name.trim()" data-test="substance-form-save" @click="save">{{ $t("common.save") }}</NButton>
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
.hint { margin: -10px 0 14px; font-size: 12px; }
@media (max-width: 520px) {
  .search, .filter { max-width: none; width: 100%; }
}
</style>
