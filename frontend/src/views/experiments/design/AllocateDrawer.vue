<script setup>
// Choose animals that belong to no group yet, then either put them all in one
// group or let the API randomise them over the groups up to their targets.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NDataTable, NDrawer, NDrawerContent, NInput, NRadioButton, NRadioGroup, NSelect, useMessage } from "naive-ui";
import { allocations, pickers } from "../../../api/experiments.js";
import { selectOption } from "../../../composables/selectOptions.js";
import { useDesignFormat } from "./shared.js";

const props = defineProps({
  show: { type: Boolean, default: false },
  experimentId: { type: String, required: true },
  groups: { type: Array, default: () => [] },
});
const emit = defineEmits(["update:show", "done"]);
const { t, te } = useI18n();
const message = useMessage();
const fmt = useDesignFormat();

const rows = ref([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const search = ref("");
const loading = ref(false);
const selected = ref([]);
const mode = ref("random");
const groupId = ref(null);
const randomGroups = ref([]);
const cage = ref("");
const error = ref("");
const busy = ref(false);

let timer = null;
async function fetchPage() {
  loading.value = true;
  try {
    const res = await pickers.unallocatedSubjects({ page: page.value, pageSize: pageSize.value, search: search.value, sort: "code", order: "asc" });
    rows.value = res.data || [];
    total.value = res.total || 0;
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}
function onPage(value) {
  page.value = value;
  fetchPage();
}
function onSearch(value) {
  search.value = value;
  page.value = 1;
  clearTimeout(timer);
  timer = setTimeout(fetchPage, 250);
}

watch(
  () => props.show,
  (open) => {
    if (!open) return;
    selected.value = [];
    error.value = "";
    cage.value = "";
    mode.value = "random";
    groupId.value = null;
    randomGroups.value = [];
    page.value = 1;
    search.value = "";
    fetchPage();
  },
);

const label = (key, value) => (te(key) ? t(key) : value);
const columns = computed(() => [
  { type: "selection" },
  { title: t("design.animals.subject"), key: "code" },
  { title: t("design.animals.species"), key: "species", render: (r) => label(`design.species.${r.species}`, r.species) },
  { title: t("design.animals.sex"), key: "sex", render: (r) => label(`design.sex.${r.sex}`, r.sex) },
]);

const groupOptions = computed(() =>
  props.groups.map((g) =>
    selectOption("allocate-group", g.id, `${g.code} ${g.name} · ${fmt.role(g.role)}${g.targetSize ? ` (${g.allocated}/${g.targetSize})` : ""}`),
  ),
);
const freePlaces = computed(() =>
  props.groups
    .filter((g) => !randomGroups.value.length || randomGroups.value.includes(g.id))
    .reduce((sum, g) => sum + Math.max(0, (g.targetSize || 0) - g.allocated), 0),
);

async function submit() {
  error.value = "";
  busy.value = true;
  const extra = cage.value.trim() ? { cage: cage.value.trim() } : {};
  try {
    if (mode.value === "random") {
      const body = { subjectIds: selected.value, ...extra };
      if (randomGroups.value.length) body.groupIds = randomGroups.value;
      const res = await allocations.randomise(props.experimentId, body);
      message.success(t("design.animals.randomised", { n: (res.data || []).length }));
    } else {
      // One request per animal; stop at the first refusal and say which animal.
      let done = 0;
      for (const subjectId of selected.value) {
        try {
          await allocations.allocate(props.experimentId, { subjectId, groupId: groupId.value, ...extra });
          done++;
        } catch (e) {
          const code = rows.value.find((r) => r.id === subjectId)?.code || "";
          if (done) emit("done");
          throw new Error(code ? `${code}: ${e.message}` : e.message, { cause: e });
        }
      }
      message.success(t("design.animals.allocated", { n: done }));
    }
    emit("done");
    emit("update:show", false);
  } catch (e) {
    error.value = e.message;
    fetchPage();
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <NDrawer :show="show" width="min(560px, 100vw)" placement="right" @update:show="emit('update:show', $event)">
    <NDrawerContent :title="$t('design.animals.allocateTitle')" closable>
      <div class="drawer" data-test="allocate-drawer">
        <p class="muted">{{ $t("design.animals.allocateHint") }}</p>
        <NInput :value="search" clearable :placeholder="$t('datatable.searchPlaceholder')" :input-props="{ 'data-test': 'allocate-search' }" @update:value="onSearch" />
        <NDataTable
          remote
          size="small"
          :columns="columns"
          :data="rows"
          :loading="loading"
          :row-key="(r) => r.id"
          v-model:checked-row-keys="selected"
          :pagination="{
            page,
            pageSize,
            itemCount: total,
            onUpdatePage: onPage,
          }"
          data-test="allocate-subjects"
        >
          <template #empty><span class="muted">{{ $t("design.animals.noUnallocated") }}</span></template>
        </NDataTable>
        <strong>{{ $t("design.animals.selectedCount", { n: selected.length }) }}</strong>

        <NRadioGroup v-model:value="mode">
          <NRadioButton value="random" data-test="allocate-mode-random">{{ $t("design.animals.modeRandom") }}</NRadioButton>
          <NRadioButton value="group" data-test="allocate-mode-group">{{ $t("design.animals.modeGroup") }}</NRadioButton>
        </NRadioGroup>

        <div v-if="mode === 'random'" class="fld">
          <span>{{ $t("design.animals.randomGroups") }}</span>
          <NSelect v-model:value="randomGroups" multiple clearable :options="groupOptions" :placeholder="$t('design.animals.allGroups')" data-test="allocate-random-groups" />
          <small class="muted">{{ $t("design.animals.randomHint", { n: freePlaces }) }}</small>
        </div>
        <div v-else class="fld">
          <span>{{ $t("design.animals.group") }}</span>
          <NSelect v-model:value="groupId" :options="groupOptions" data-test="allocate-group" />
        </div>
        <label class="fld">
          <span>{{ $t("design.animals.cageOptional") }}</span>
          <NInput v-model:value="cage" :maxlength="32" :input-props="{ 'data-test': 'allocate-cage' }" />
        </label>
        <NAlert v-if="error" type="error" :title="error" data-test="allocate-error" />
      </div>
      <template #footer>
        <div class="dialog-actions">
          <NButton @click="emit('update:show', false)">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="busy" :disabled="!selected.length || (mode === 'group' && !groupId)" data-test="allocate-submit" @click="submit">
            {{ mode === "random" ? $t("design.animals.randomise") : $t("design.animals.allocate") }}
          </NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.drawer { display: flex; flex-direction: column; gap: 12px; }
.drawer p { margin: 0; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld > span { font-size: 12px; color: var(--muted); }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
