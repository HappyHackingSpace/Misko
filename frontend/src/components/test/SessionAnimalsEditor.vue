<script setup>
// The animals of a planned session: remove one (the last stays; cancel the
// session instead) or add one in a free arena of the session's paradigm.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NPopconfirm, NSelect, NTag, useMessage } from "naive-ui";
import { tests } from "../../api/tests.js";
import { studyDesign } from "../../api/dosing.js";
import { useArenas } from "../../composables/useArenas.js";
import { filterByText, selectOption } from "../../composables/selectOptions.js";

const props = defineProps({
  test: { type: Object, required: true },
});
const emit = defineEmits(["updated"]);
const { t } = useI18n();
const message = useMessage();
const arenas = useArenas();

const allocations = ref([]);
const adding = ref({ allocationId: null, environmentRevisionId: null });
const busy = ref("");
const error = ref("");

const inTest = computed(() => new Set(props.test.subjects.map((s) => s.allocationId)));
const usedArenas = computed(() => new Set(props.test.subjects.map((s) => s.environmentRevisionId)));
const animalOptions = computed(() =>
  allocations.value.filter((a) => !a.excluded && !inTest.value.has(a.id)).map((a) => selectOption("add-animal", a.id, a.subjectCode)),
);
const arenaOptions = computed(() =>
  arenas
    .forParadigm(props.test.paradigmKey, props.test.paradigmVersion)
    .filter((r) => !usedArenas.value.has(r.id))
    .map((r) => selectOption("add-arena", r.id, `${r.environmentName} · ${t("execution.tests.revisionN", { n: r.number })}`)),
);
const full = computed(() => props.test.subjects.length >= 12);

async function act(name, run) {
  busy.value = name;
  error.value = "";
  try {
    const updated = await run();
    emit("updated", updated);
    return true;
  } catch (e) {
    error.value = e.message;
    return false;
  } finally {
    busy.value = "";
  }
}

async function add() {
  const ok = await act("add", () => tests.addSubject(props.test.id, { ...adding.value }));
  if (ok) {
    message.success(t("execution.tests.animalAdded"));
    adding.value = { allocationId: null, environmentRevisionId: null };
  }
}

async function remove(subject) {
  if (await act(subject.id, () => tests.removeSubject(props.test.id, subject.id))) message.success(t("execution.tests.animalRemoved"));
}

onMounted(async () => {
  try {
    await arenas.load();
    allocations.value = (await studyDesign.allocations(props.test.experimentId)).data || [];
  } catch (e) {
    error.value = e.message;
  }
});
</script>

<template>
  <div class="editor" data-test="session-animals-editor">
    <div v-for="subject in test.subjects" :key="subject.id" class="line" data-test="session-animal" :data-animal="subject.subjectCode">
      <strong>{{ subject.subjectCode }}</strong>
      <span class="muted">{{ subject.environmentName }}</span>
      <NTag v-if="subject.excluded" size="small" type="error" :bordered="false">{{ $t("execution.excluded") }}</NTag>
      <span class="spacer" />
      <NPopconfirm @positive-click="remove(subject)">
        <template #trigger>
          <NButton size="tiny" quaternary type="error" :disabled="test.subjects.length === 1 || !!busy" :loading="busy === subject.id" data-test="remove-animal">
            {{ $t("execution.tests.remove") }}
          </NButton>
        </template>
        {{ $t("execution.tests.removeConfirm", { animal: subject.subjectCode }) }}
      </NPopconfirm>
    </div>
    <p v-if="test.subjects.length === 1" class="muted small">{{ $t("execution.tests.lastAnimalHint") }}</p>

    <div v-if="!full" class="add">
      <NSelect
        v-model:value="adding.allocationId"
        filterable
        :filter="filterByText"
        size="small"
        :placeholder="$t('execution.animal')"
        :options="animalOptions"
        data-test="add-animal"
      />
      <NSelect v-model:value="adding.environmentRevisionId" size="small" :placeholder="$t('execution.arena')" :options="arenaOptions" data-test="add-arena" />
      <NButton
        size="small"
        type="primary"
        secondary
        :disabled="!adding.allocationId || !adding.environmentRevisionId || !!busy"
        :loading="busy === 'add'"
        data-test="add-animal-save"
        @click="add"
      >
        {{ $t("execution.tests.addAnimal") }}
      </NButton>
    </div>
    <NAlert v-if="error" type="error" :title="error" data-test="animals-error" />
  </div>
</template>

<style scoped>
.editor { display: flex; flex-direction: column; gap: 8px; }
.line { display: flex; align-items: center; gap: 10px; padding: 6px 0; border-bottom: 1px solid var(--line); }
.spacer { flex: 1; }
.small { font-size: 12px; margin: 0; }
.add { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) auto; gap: 8px; align-items: center; margin-top: 6px; }
</style>
