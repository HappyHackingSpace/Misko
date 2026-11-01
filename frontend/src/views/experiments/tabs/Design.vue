<script setup>
// Groups as cards: who they are (role, disease model, target size), how full
// they are, and what each group receives (its regimen). Editable only while
// the study is a draft; afterwards it is the frozen record of the design.
import { computed, inject, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { NAlert, NButton, NCard, NEmpty, NPopconfirm, NProgress, NSpin, NTag, useMessage } from "naive-ui";
import { groups as groupsApi, pickers, regimenItems } from "../../../api/experiments.js";
import { useAuth } from "../../../stores/auth.js";
import { NO_SUBSTANCE, purposeTagType, roleColor, roleTagType, useDesignFormat } from "../design/shared.js";
import GroupForm from "../design/GroupForm.vue";
import RegimenForm from "../design/RegimenForm.vue";

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const fmt = useDesignFormat();
const { experiment, id } = inject("study");
const route = useRoute();
const router = useRouter();

const groups = ref([]);
const items = ref([]);
const substances = ref([]);
const diseaseModels = ref([]);
const loading = ref(false);
const error = ref("");

const editable = computed(() => experiment.value?.status === "DRAFT" && auth.can("study:write"));

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [g, r, s, d] = await Promise.all([
      groupsApi.list(id.value),
      regimenItems.list(id.value),
      pickers.substances(),
      pickers.diseaseModels(),
    ]);
    groups.value = g.data || [];
    items.value = r.data || [];
    substances.value = s.data || [];
    diseaseModels.value = d.data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

const itemsOf = (groupId) => items.value.filter((i) => i.groupId === groupId);
const substanceName = (sid) => substances.value.find((s) => s.id === sid)?.name || "-";
const diseaseName = (did) => diseaseModels.value.find((d) => d.id === did)?.name || "-";
const fillPercent = (g) => (g.targetSize ? Math.min(100, Math.round((g.allocated / g.targetSize) * 100)) : 0);

const nextCode = computed(() => {
  const used = new Set(groups.value.map((g) => g.code.toLowerCase()));
  let n = 1;
  while (used.has(`g${n}`)) n++;
  return `G${n}`;
});

// A healthy control receives no substance at all (its items are only the
// periods without one); a sham only a vehicle. Their empty regimen and add
// button say so.
const emptyText = (g) => t(g.role === "HEALTHY_CONTROL" ? "design.regimen.emptyNaive" : "design.regimen.empty");
const addLabel = (g) => t(g.role === "SHAM" ? "design.regimen.addVehicle" : "design.regimen.add");

// Group dialog.
const groupOpen = ref(false);
const editingGroup = ref(null);
const presetRole = ref(null);
const presetDiseaseModel = ref(null);
function addGroup(role = null, diseaseModelId = null) {
  editingGroup.value = null;
  presetRole.value = role;
  presetDiseaseModel.value = diseaseModelId;
  groupOpen.value = true;
}
function editGroup(g) {
  editingGroup.value = g;
  presetRole.value = null;
  presetDiseaseModel.value = null;
  groupOpen.value = true;
}
async function deleteGroup(g) {
  error.value = "";
  try {
    await groupsApi.remove(id.value, g.id);
    message.success(t("design.groups.deleted"));
    await load();
  } catch (e) {
    error.value = e.message;
  }
}

// Regimen dialog.
const itemOpen = ref(false);
const itemGroup = ref(null);
const editingItem = ref(null);
const presetPurpose = ref(null);
function addItem(g, purpose = null) {
  itemGroup.value = g;
  editingItem.value = null;
  presetPurpose.value = purpose;
  itemOpen.value = true;
}
function editItem(g, item) {
  itemGroup.value = g;
  editingItem.value = item;
  presetPurpose.value = null;
  itemOpen.value = true;
}
async function deleteItem(item) {
  error.value = "";
  try {
    await regimenItems.remove(id.value, item.id);
    message.success(t("design.regimen.deleted"));
    await load();
  } catch (e) {
    error.value = e.message;
  }
}

// The readiness panel links here to fix one unmet rule:
// ?addGroup=[<role>][&diseaseModel=<id>], ?addItem=<groupId>[&purpose=<purpose>]
// or ?editGroup=<groupId>. The request is consumed so a reload does not reopen it.
const REQUEST_KEYS = ["addGroup", "diseaseModel", "addItem", "purpose", "editGroup"];
function openRequested() {
  const q = route.query;
  if (!REQUEST_KEYS.some((k) => k in q)) return;
  const query = { ...q };
  REQUEST_KEYS.forEach((k) => delete query[k]);
  router.replace({ query });
  if (!editable.value) return;
  const group = groups.value.find((g) => g.id === (q.addItem || q.editGroup));
  if ("addGroup" in q) addGroup(q.addGroup ? String(q.addGroup) : null, q.diseaseModel ? String(q.diseaseModel) : null);
  else if (q.addItem && group) addItem(group, q.purpose ? String(q.purpose) : null);
  else if (q.editGroup && group) editGroup(group);
}

async function onSaved(kind) {
  message.success(t(kind === "group" ? "design.groups.saved" : "design.regimen.saved"));
  await load();
}

// Requests that name a group wait for the groups to load.
onMounted(async () => {
  await load();
  openRequested();
});
watch(() => experiment.value?.status, load);
watch(() => REQUEST_KEYS.map((k) => route.query[k]), openRequested);
</script>

<template>
  <div class="design">
    <NAlert v-if="experiment && experiment.status !== 'DRAFT'" type="info" :title="$t('design.frozen')" data-test="design-frozen" />
    <NAlert v-if="error" type="error" :title="error" data-test="design-error" />

    <div class="bar">
      <p class="muted">{{ $t("design.groups.intro") }}</p>
      <NButton v-if="editable" type="primary" data-test="group-add" @click="addGroup()">{{ $t("design.groups.add") }}</NButton>
    </div>

    <NSpin :show="loading">
      <NEmpty v-if="!loading && !groups.length" :description="$t('design.groups.empty')" class="empty" />
      <div class="cards">
        <NCard
          v-for="g in groups"
          :key="g.id"
          size="small"
          class="group-card"
          :style="{ '--role': roleColor(g.role) }"
          :data-test="`group-card-${g.code}`"
        >
          <template #header>
            <div class="head">
              <span class="code">{{ g.code }}</span>
              <span class="name">{{ g.name }}</span>
            </div>
          </template>
          <template #header-extra>
            <NTag size="small" round :bordered="false" :type="roleTagType(g.role)" data-test="group-role-tag">{{ fmt.role(g.role) }}</NTag>
          </template>

          <dl class="facts">
            <dt>{{ $t("design.groups.diseaseModel") }}</dt>
            <dd>{{ g.diseaseModelId ? diseaseName(g.diseaseModelId) : $t("design.groups.noDisease") }}</dd>
            <dt>{{ $t("design.groups.fill") }}</dt>
            <dd>
              <template v-if="g.targetSize">
                <NProgress type="line" :percentage="fillPercent(g)" :show-indicator="false" :height="6" />
                <small>{{ $t("design.groups.fillText", { allocated: g.allocated, target: g.targetSize }) }}</small>
              </template>
              <small v-else class="warn">{{ $t("design.groups.noTarget", { allocated: g.allocated }) }}</small>
              <small v-if="g.excluded" class="muted"> · {{ $t("design.groups.excludedCount", { n: g.excluded }) }}</small>
            </dd>
          </dl>
          <p v-if="g.description" class="desc muted">{{ g.description }}</p>

          <div class="regimen">
            <div class="regimen-head">
              <strong>{{ $t("design.regimen.title") }}</strong>
              <NButton v-if="editable" size="tiny" secondary :data-test="`regimen-add-${g.code}`" @click="addItem(g)">{{ addLabel(g) }}</NButton>
            </div>
            <small class="muted hint" :data-test="`regimen-rule-${g.code}`">{{ $t(`design.regimen.roleRules.${g.role}`) }}</small>
            <p v-if="!itemsOf(g.id).length" class="muted small" :data-test="`regimen-empty-${g.code}`">{{ emptyText(g) }}</p>
            <ul class="items">
              <li v-for="item in itemsOf(g.id)" :key="item.id" class="item" data-test="regimen-item" :data-purpose="item.purpose">
                <div v-if="item.purpose === NO_SUBSTANCE" class="item-body" data-test="regimen-period">{{ fmt.period(item) }}</div>
                <NTag v-else size="small" :bordered="false" :type="purposeTagType(item.purpose)">{{ fmt.purpose(item.purpose) }}</NTag>
                <div v-if="item.purpose !== NO_SUBSTANCE" class="item-body">
                  <div><strong>{{ substanceName(item.substanceId) }}</strong> <span data-test="regimen-dose">{{ fmt.dose(item) }}</span></div>
                  <small class="muted">{{ fmt.route(item.route) }} · {{ fmt.frequency(item) }} · {{ fmt.days(item) }}</small>
                </div>
                <div v-if="editable" class="item-actions">
                  <NButton size="tiny" quaternary @click="editItem(g, item)">{{ $t("common.edit") }}</NButton>
                  <NPopconfirm @positive-click="deleteItem(item)">
                    <template #trigger>
                      <NButton size="tiny" quaternary type="error">{{ $t("common.delete") }}</NButton>
                    </template>
                    {{ $t("design.regimen.deleteConfirm") }}
                  </NPopconfirm>
                </div>
              </li>
            </ul>
          </div>

          <template v-if="editable" #action>
            <div class="card-actions">
              <NButton size="small" :data-test="`group-edit-${g.code}`" @click="editGroup(g)">{{ $t("common.edit") }}</NButton>
              <NPopconfirm @positive-click="deleteGroup(g)">
                <template #trigger>
                  <NButton size="small" type="error" secondary :data-test="`group-delete-${g.code}`">{{ $t("common.delete") }}</NButton>
                </template>
                {{ $t("design.groups.deleteConfirm") }}
              </NPopconfirm>
            </div>
          </template>
        </NCard>
      </div>
    </NSpin>

    <GroupForm
      v-model:show="groupOpen"
      :experiment-id="id"
      :group="editingGroup"
      :disease-models="diseaseModels"
      :next-code="nextCode"
      :role="presetRole"
      :disease-model-id="presetDiseaseModel"
      @saved="onSaved('group')"
    />
    <RegimenForm
      v-model:show="itemOpen"
      :experiment-id="id"
      :group="itemGroup"
      :item="editingItem"
      :group-items="itemGroup ? itemsOf(itemGroup.id) : []"
      :substances="substances"
      :purpose="presetPurpose"
      @saved="onSaved('item')"
    />
  </div>
</template>

<style scoped>
.design { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
.bar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.bar p { flex: 1; margin: 0; min-width: 200px; }
.empty { padding: 30px 0; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr)); gap: 14px; }
.group-card { border-left: 4px solid var(--role); min-width: 0; }
.head { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
.code { font-weight: 700; color: var(--role); }
.name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.facts { display: grid; grid-template-columns: auto 1fr; gap: 6px 12px; margin: 0 0 8px; font-size: 13px; }
.facts dt { color: var(--muted); }
.facts dd { margin: 0; min-width: 0; }
.warn { color: var(--warn); }
.desc { margin: 0 0 8px; white-space: pre-wrap; }
.regimen { border-top: 1px solid var(--line); padding-top: 8px; }
.regimen-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; }
.items { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
.item { display: flex; align-items: flex-start; gap: 8px; }
.item-body { flex: 1; min-width: 0; font-size: 13px; overflow-wrap: anywhere; }
.item-actions { display: flex; gap: 2px; flex: none; }
.small { font-size: 12px; margin: 0 0 6px; }
.hint { display: block; margin: 0 0 8px; font-size: 11px; }
.card-actions { display: flex; gap: 8px; justify-content: flex-end; }
</style>
