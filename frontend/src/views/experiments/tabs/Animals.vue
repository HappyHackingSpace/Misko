<script setup>
// The animals of the study: how full every group is, and each allocated
// animal with its cage and, if excluded, why. In a draft an animal can still
// be moved or taken out; once the study runs it stays in its group and can
// only be excluded with a reason, never deleted.
import { computed, h, inject, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDataTable, NDatePicker, NInput, NModal, NPopconfirm, NProgress, NSelect, NSpin, NTag, NTooltip, useMessage } from "naive-ui";
import { allocations } from "../../../api/experiments.js";
import { selectOption } from "../../../composables/selectOptions.js";
import { useAuth } from "../../../stores/auth.js";
import { EXCLUSION_REASONS, roleColor, roleTagType, useDesignFormat, useTabLink } from "../design/shared.js";
import AllocateDrawer from "../design/AllocateDrawer.vue";

const { t } = useI18n();
const message = useMessage();
const auth = useAuth();
const fmt = useDesignFormat();
const goTab = useTabLink();
const { experiment, id } = inject("study");

const status = computed(() => experiment.value?.status);
const canWrite = computed(() => auth.can("study:write") && status.value !== "COMPLETED");
const isDraft = computed(() => status.value === "DRAFT");

const rows = ref([]);
const groups = ref([]);
const loading = ref(false);
const error = ref("");
const groupFilter = ref(null);
const search = ref("");
const showExcluded = ref("all");

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const res = await allocations.list(id.value);
    rows.value = res.data || [];
    groups.value = res.groups || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

const groupById = (gid) => groups.value.find((g) => g.id === gid);
const filtered = computed(() => {
  const q = search.value.trim().toLowerCase();
  return rows.value.filter(
    (r) =>
      (!groupFilter.value || r.groupId === groupFilter.value) &&
      (showExcluded.value === "all" || (showExcluded.value === "excluded") === r.excluded) &&
      (!q || (r.subjectCode || "").toLowerCase().includes(q) || (r.cage || "").toLowerCase().includes(q)),
  );
});
const groupOptions = computed(() => groups.value.map((g) => selectOption("animals-group", g.id, `${g.code} ${g.name}`)));
const stateOptions = computed(() => [
  { value: "all", label: t("design.animals.stateAll") },
  { value: "active", label: t("design.animals.stateActive") },
  { value: "excluded", label: t("design.animals.stateExcluded") },
]);
const fillPercent = (g) => (g.targetSize ? Math.min(100, Math.round((g.allocated / g.targetSize) * 100)) : 0);

const columns = computed(() => [
  { title: t("design.animals.subject"), key: "subjectCode", sorter: (a, b) => (a.subjectCode || "").localeCompare(b.subjectCode || "") },
  {
    title: t("design.animals.group"),
    key: "groupId",
    render: (r) => {
      const g = groupById(r.groupId);
      return g ? h(NTag, { size: "small", bordered: false, type: roleTagType(g.role) }, () => `${g.code} ${g.name}`) : "-";
    },
  },
  { title: t("design.animals.cage"), key: "cage", render: (r) => r.cage || "-" },
  { title: t("design.animals.allocatedAt"), key: "allocatedAt", render: (r) => fmt.dateTime(r.allocatedAt) },
  {
    title: t("design.animals.state"),
    key: "excluded",
    render: (r) => {
      if (!r.excluded) return h(NTag, { size: "small", round: true, bordered: false, type: "success" }, () => t("design.animals.stateActive"));
      const tag = h(NTag, { size: "small", round: true, bordered: false, type: "error", "data-test": "allocation-excluded" }, () =>
        `${t("design.animals.stateExcluded")}: ${t(`design.exclusionReasons.${r.exclusionReason}`)}`,
      );
      const detail = [fmt.dateTime(r.excludedAt), r.exclusionNote].filter(Boolean).join(" · ");
      return h(NTooltip, null, { trigger: () => tag, default: () => detail });
    },
  },
  ...(canWrite.value
    ? [
        {
          title: t("common.actions"),
          key: "actions",
          width: 260,
          render: (r) =>
            h("div", { class: "row-actions" }, [
              h(NButton, { size: "tiny", quaternary: true, onClick: () => openCage(r) }, () => t("design.animals.setCage")),
              isDraft.value ? h(NButton, { size: "tiny", quaternary: true, onClick: () => openMove(r) }, () => t("design.animals.move")) : null,
              !r.excluded ? h(NButton, { size: "tiny", quaternary: true, type: "warning", "data-test": "allocation-exclude", onClick: () => openExclude(r) }, () => t("design.animals.exclude")) : null,
              isDraft.value
                ? h(
                    NPopconfirm,
                    { onPositiveClick: () => removeAllocation(r) },
                    {
                      trigger: () => h(NButton, { size: "tiny", quaternary: true, type: "error" }, () => t("design.animals.remove")),
                      default: () => t("design.animals.removeConfirm"),
                    },
                  )
                : null,
            ]),
        },
      ]
    : []),
]);

// Allocation drawer.
const drawerOpen = ref(false);
async function onAllocated() {
  await load();
}

// Cage, move, exclude dialogs share one modal.
const dialog = ref(null); // "cage" | "move" | "exclude"
const current = ref(null);
const dialogForm = ref({});
const dialogError = ref("");
const dialogBusy = ref(false);
function openCage(r) {
  current.value = r;
  dialogForm.value = { cage: r.cage || "" };
  dialogError.value = "";
  dialog.value = "cage";
}
function openMove(r) {
  current.value = r;
  dialogForm.value = { groupId: r.groupId };
  dialogError.value = "";
  dialog.value = "move";
}
function openExclude(r) {
  current.value = r;
  dialogForm.value = { reason: null, note: "", excludedAt: null };
  dialogError.value = "";
  dialog.value = "exclude";
}
const dialogOpen = computed({
  get: () => !!dialog.value,
  set: (v) => {
    if (!v) dialog.value = null;
  },
});
const reasonOptions = computed(() => EXCLUSION_REASONS.map((r) => selectOption("exclude-reason", r, t(`design.exclusionReasons.${r}`))));

async function submitDialog() {
  dialogError.value = "";
  dialogBusy.value = true;
  const r = current.value;
  try {
    if (dialog.value === "cage") {
      await allocations.update(id.value, r.id, { cage: dialogForm.value.cage.trim() });
      message.success(t("design.animals.cageSaved"));
    } else if (dialog.value === "move") {
      await allocations.update(id.value, r.id, { groupId: dialogForm.value.groupId });
      message.success(t("design.animals.moved"));
    } else {
      const body = { reason: dialogForm.value.reason };
      if (dialogForm.value.note.trim()) body.note = dialogForm.value.note.trim();
      if (dialogForm.value.excludedAt) body.excludedAt = new Date(dialogForm.value.excludedAt).toISOString();
      await allocations.exclude(id.value, r.id, body);
      message.success(t("design.animals.excluded"));
    }
    dialog.value = null;
    await load();
  } catch (e) {
    dialogError.value = e.message;
  } finally {
    dialogBusy.value = false;
  }
}
const dialogTitle = computed(() => {
  const code = current.value?.subjectCode || "";
  return { cage: t("design.animals.setCage"), move: t("design.animals.move"), exclude: t("design.animals.exclude") }[dialog.value] + (code ? ` · ${code}` : "");
});
const dialogReady = computed(() => (dialog.value === "exclude" ? !!dialogForm.value.reason : dialog.value === "move" ? !!dialogForm.value.groupId : true));

async function removeAllocation(r) {
  error.value = "";
  try {
    await allocations.remove(id.value, r.id);
    message.success(t("design.animals.removed"));
    await load();
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(load);
watch(status, load);
</script>

<template>
  <div class="animals">
    <NAlert v-if="error" type="error" :title="error" data-test="animals-error" />
    <NAlert v-if="status === 'RUNNING'" type="info" :title="$t('design.animals.runningHint')" />

    <NSpin :show="loading">
      <div class="stack">
        <NCard size="small" :title="$t('design.animals.fillTitle')">
          <template #header-extra>
            <NButton v-if="canWrite && groups.length" type="primary" size="small" data-test="allocate-open" @click="drawerOpen = true">{{ $t("design.animals.allocateTitle") }}</NButton>
          </template>
          <p v-if="!groups.length && !loading" class="muted">
            {{ $t("design.animals.noGroups") }}
            <a href="#" class="link" @click.prevent="goTab('design')">{{ $t("experiments.tabs.design") }}</a>
          </p>
          <div class="fills">
            <button
              v-for="g in groups"
              :key="g.id"
              type="button"
              class="fill"
              :class="{ active: groupFilter === g.id }"
              :style="{ '--role': roleColor(g.role) }"
              :data-test="`fill-${g.code}`"
              @click="groupFilter = groupFilter === g.id ? null : g.id"
            >
              <span class="fill-head"><strong>{{ g.code }}</strong> {{ g.name }}</span>
              <small class="muted">{{ fmt.role(g.role) }}</small>
              <NProgress v-if="g.targetSize" type="line" :percentage="fillPercent(g)" :show-indicator="false" :height="6" :status="g.allocated >= g.targetSize ? 'success' : 'default'" />
              <small>
                {{ g.targetSize ? $t("design.groups.fillText", { allocated: g.allocated, target: g.targetSize }) : $t("design.groups.noTarget", { allocated: g.allocated }) }}
                <template v-if="g.excluded"> · {{ $t("design.groups.excludedCount", { n: g.excluded }) }}</template>
              </small>
            </button>
          </div>
        </NCard>

        <NCard size="small" :title="$t('design.animals.title')">
          <div class="filters">
            <NInput v-model:value="search" clearable class="search" :placeholder="$t('datatable.searchPlaceholder')" :input-props="{ 'data-test': 'animals-search' }" />
            <NSelect v-model:value="groupFilter" clearable class="filter" :options="groupOptions" :placeholder="$t('design.animals.allGroups')" data-test="animals-group-filter" />
            <NSelect v-model:value="showExcluded" class="filter" :options="stateOptions" data-test="animals-state-filter" />
          </div>
          <NDataTable
            :columns="columns"
            :data="filtered"
            :row-key="(r) => r.id"
            size="small"
            :scroll-x="860"
            :pagination="{ pageSize: 20 }"
            data-test="allocations-table"
          >
            <template #empty><span class="muted">{{ rows.length ? $t("datatable.noResults") : $t("design.animals.empty") }}</span></template>
          </NDataTable>
        </NCard>
      </div>
    </NSpin>

    <AllocateDrawer v-model:show="drawerOpen" :experiment-id="id" :groups="groups" @done="onAllocated" />

    <NModal v-model:show="dialogOpen" preset="card" :title="dialogTitle" style="width: 460px; max-width: 94vw">
      <div class="form" data-test="allocation-dialog">
        <label v-if="dialog === 'cage'" class="fld">
          <span>{{ $t("design.animals.cage") }}</span>
          <NInput v-model:value="dialogForm.cage" :maxlength="32" clearable :input-props="{ 'data-test': 'allocation-cage' }" />
          <small class="muted">{{ $t("design.animals.cageHint") }}</small>
        </label>
        <div v-else-if="dialog === 'move'" class="fld">
          <span>{{ $t("design.animals.group") }}</span>
          <NSelect v-model:value="dialogForm.groupId" :options="groupOptions" data-test="allocation-move-group" />
        </div>
        <template v-else-if="dialog === 'exclude'">
          <p class="muted">{{ $t("design.animals.excludeHint") }}</p>
          <div class="fld">
            <span>{{ $t("design.animals.reason") }}</span>
            <NSelect v-model:value="dialogForm.reason" :options="reasonOptions" data-test="exclude-reason" />
          </div>
          <label class="fld">
            <span>{{ $t("common.notes") }}</span>
            <NInput v-model:value="dialogForm.note" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" :maxlength="2000" :input-props="{ 'data-test': 'exclude-note' }" />
          </label>
          <label class="fld">
            <span>{{ $t("design.animals.excludedAt") }}</span>
            <NDatePicker v-model:value="dialogForm.excludedAt" type="datetime" clearable :is-date-disabled="(ts) => ts > Date.now()" :placeholder="$t('design.animals.now')" />
          </label>
        </template>
        <NAlert v-if="dialogError" type="error" :title="dialogError" data-test="allocation-dialog-error" />
      </div>
      <template #footer>
        <div class="dialog-actions">
          <NButton @click="dialog = null">{{ $t("common.cancel") }}</NButton>
          <NButton :type="dialog === 'exclude' ? 'warning' : 'primary'" :loading="dialogBusy" :disabled="!dialogReady" data-test="allocation-dialog-save" @click="submitDialog">
            {{ dialog === "exclude" ? $t("design.animals.exclude") : $t("common.save") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.animals, .stack { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.fills { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 200px), 1fr)); gap: 10px; }
.fill {
  display: flex; flex-direction: column; gap: 4px; text-align: left; cursor: pointer;
  border: 1px solid var(--line); border-left: 4px solid var(--role); border-radius: 8px;
  padding: 8px 10px; background: transparent; color: inherit; font: inherit; min-width: 0;
}
.fill.active { border-color: var(--role); background: var(--active-bg); }
.fill-head { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.filters { display: flex; gap: 10px; flex-wrap: wrap; margin-bottom: 12px; }
.search { max-width: 280px; }
.filter { width: 200px; }
:deep(.row-actions) { display: flex; gap: 4px; flex-wrap: wrap; }
.form { display: flex; flex-direction: column; gap: 12px; }
.form p { margin: 0; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld > span { font-size: 12px; color: var(--muted); }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
@media (max-width: 520px) {
  .search, .filter { max-width: none; width: 100%; }
}
</style>
