<script setup>
// One animal: what it is, the group it belongs to and its body weights. Weights
// are never edited once recorded; doses in mg/kg use the latest one.
import { computed, h, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import {
  NAlert, NButton, NCard, NDataTable, NDatePicker, NDescriptions, NDescriptionsItem, NEmpty, NForm, NFormItem, NInputNumber, NPopconfirm, NSpace, NTag, useMessage,
} from "naive-ui";
import { subjects } from "../../api/subjects.js";
import { useAuth } from "../../stores/auth.js";
import { useBreadcrumb } from "../../stores/breadcrumb.js";
import PageHead from "../../components/PageHead.vue";
import SubjectDrawer from "./SubjectDrawer.vue";
import { ROLE_TAG, useSubjectLabels } from "./useSubjectLabels.js";

const { t, locale } = useI18n();
const route = useRoute();
const router = useRouter();
const message = useMessage();
const auth = useAuth();
const crumb = useBreadcrumb();
const labels = useSubjectLabels();
const canWrite = computed(() => auth.can("subject:write"));
const canWeigh = computed(() => auth.can("weight:write"));

const subject = ref(null);
const weights = ref([]);
const error = ref("");
const loading = ref(true);
const id = computed(() => route.params.id);

async function load() {
  error.value = "";
  loading.value = true;
  try {
    const [s, w] = await Promise.all([subjects.get(id.value), subjects.weights(id.value)]);
    subject.value = s;
    weights.value = w.data || [];
    crumb.set([{ label: t("subjects.title"), to: "/subjects" }, { label: s.code }]);
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

const latest = computed(() => weights.value[0] || null);

// Edit and delete.
const drawer = ref(false);
const actionError = ref("");
function onSaved(saved) {
  subject.value = { ...subject.value, ...saved };
  crumb.set([{ label: t("subjects.title"), to: "/subjects" }, { label: saved.code }]);
}
async function remove() {
  actionError.value = "";
  try {
    await subjects.remove(subject.value.id);
    message.success(t("subjects.deleted", { code: subject.value.code }));
    router.push("/subjects");
  } catch (e) {
    actionError.value = e.message;
  }
}

// Weights, newest first, with the change from the weighing before.
const weightColumns = computed(() => [
  { title: t("subjects.weights.measuredAt"), key: "measuredAt", render: (row) => labels.dateTime(row.measuredAt) },
  { title: t("subjects.weights.grams"), key: "grams", render: (row) => h("strong", { "data-test": "weight-grams" }, labels.grams(row.grams)) },
  {
    title: t("subjects.weights.change"),
    key: "change",
    render: (row, index) => {
      const previous = weights.value[index + 1];
      if (!previous) return "";
      const diff = Number(row.grams) - Number(previous.grams);
      const sign = diff > 0 ? "+" : "";
      return h("span", { class: "muted" }, `${sign}${labels.grams(diff)}`);
    },
  },
]);

const weightForm = ref({ grams: null, measuredAt: Date.now() });
const weightError = ref("");
const weighing = ref(false);
const isFuture = (ts) => ts > Date.now();
const pickerFormat = computed(() => (locale.value === "tr" ? "dd.MM.yyyy HH:mm" : "yyyy-MM-dd HH:mm"));

async function recordWeight() {
  weightError.value = "";
  weighing.value = true;
  try {
    // The API takes grams as a decimal string with at most 3 decimals.
    const grams = String(Number(Number(weightForm.value.grams).toFixed(3)));
    await subjects.recordWeight(id.value, { grams, measuredAt: new Date(weightForm.value.measuredAt).toISOString() });
    message.success(t("subjects.weights.recorded", { grams: labels.grams(grams) }));
    weightForm.value = { grams: null, measuredAt: Date.now() };
    weights.value = (await subjects.weights(id.value)).data || [];
  } catch (e) {
    weightError.value = e.message;
  } finally {
    weighing.value = false;
  }
}

onMounted(load);
watch(id, (value) => value && load());
onUnmounted(() => crumb.clear());
</script>

<template>
  <NAlert v-if="error" type="error" :title="error" data-test="error" />
  <template v-else-if="subject">
    <PageHead :title="subject.code" :subtitle="`${labels.species(subject.species)}, ${labels.sex(subject.sex)}`">
      <template #actions>
        <NSpace v-if="canWrite">
          <NButton secondary data-test="subject-edit" @click="drawer = true">{{ $t("common.edit") }}</NButton>
          <NPopconfirm :positive-text="$t('common.delete')" :negative-text="$t('common.cancel')" @positive-click="remove">
            <template #trigger>
              <NButton secondary type="error" data-test="subject-delete">{{ $t("common.delete") }}</NButton>
            </template>
            {{ $t("subjects.confirmDelete", { code: subject.code }) }}
          </NPopconfirm>
        </NSpace>
      </template>
    </PageHead>
    <NAlert v-if="actionError" type="error" :title="actionError" class="alert" data-test="subject-action-error" closable @close="actionError = ''" />

    <div class="grid">
      <NCard :title="$t('subjects.details')" size="small" :bordered="true">
        <NDescriptions :column="1" label-placement="left" size="small" bordered>
          <NDescriptionsItem :label="$t('subjects.code')">{{ subject.code }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('subjects.species')">{{ labels.species(subject.species) }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('subjects.sex')">{{ labels.sex(subject.sex) }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('subjects.weights.latest')">
            <span data-test="subject-latest-weight">{{ latest ? `${labels.grams(latest.grams)} (${labels.date(latest.measuredAt)})` : $t("subjects.weights.none") }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('subjects.createdAt')">{{ labels.dateTime(subject.createdAt) }}</NDescriptionsItem>
        </NDescriptions>
      </NCard>

      <NCard :title="$t('subjects.allocation')" size="small" :bordered="true" data-test="subject-allocation-card">
        <NDescriptions v-if="subject.allocation" :column="1" label-placement="left" size="small" bordered>
          <NDescriptionsItem :label="$t('subjects.experiment')">
            <RouterLink class="link" :to="`/experiments/${subject.allocation.experimentId}`" data-test="subject-experiment">{{ subject.allocation.experimentCode }}</RouterLink>
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('subjects.group')">{{ subject.allocation.groupCode }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('subjects.role')">
            <NTag size="small" round :bordered="false" :type="ROLE_TAG[subject.allocation.groupRole]">{{ labels.role(subject.allocation.groupRole) }}</NTag>
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('subjects.status')">
            <NTag v-if="subject.allocation.excluded" size="small" round type="error" data-test="subject-excluded">{{ $t("subjects.excluded") }}</NTag>
            <span v-else>{{ $t("subjects.onStudy") }}</span>
          </NDescriptionsItem>
        </NDescriptions>
        <p v-else class="muted" data-test="subject-unallocated">{{ $t("subjects.unallocatedHint") }}</p>
      </NCard>
    </div>

    <NCard :title="$t('subjects.weights.title')" size="small" :bordered="true" class="weights" data-test="subject-weights">
      <NForm v-if="canWeigh" inline class="weigh" data-test="weight-form" @submit.prevent="recordWeight">
        <NFormItem :label="$t('subjects.weights.grams')">
          <NInputNumber v-model:value="weightForm.grams" data-test="weight-grams-input" :min="0.001" :max="5000" :step="0.1" :placeholder="$t('subjects.weights.gramsPlaceholder')">
            <template #suffix>g</template>
          </NInputNumber>
        </NFormItem>
        <NFormItem :label="$t('subjects.weights.measuredAt')">
          <NDatePicker v-model:value="weightForm.measuredAt" type="datetime" :format="pickerFormat" data-test="weight-measured-at" :is-date-disabled="isFuture" :clearable="false" />
        </NFormItem>
        <NFormItem>
          <NButton type="primary" attr-type="submit" :loading="weighing" :disabled="!weightForm.grams || !weightForm.measuredAt" data-test="weight-save">
            {{ $t("subjects.weights.record") }}
          </NButton>
        </NFormItem>
      </NForm>
      <NAlert v-if="weightError" type="error" :title="weightError" class="alert" data-test="weight-error" />
      <NDataTable
        :columns="weightColumns"
        :data="weights"
        :loading="loading"
        :row-key="(row) => row.id"
        :row-props="() => ({ 'data-test': 'weight-row' })"
        :pagination="{ pageSize: 10 }"
        size="small"
      >
        <template #empty><NEmpty :description="$t('subjects.weights.none')" /></template>
      </NDataTable>
    </NCard>

    <SubjectDrawer v-model:show="drawer" :subject="subject" @saved="onSaved" />
  </template>
</template>

<style scoped>
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; }
.weights { margin-top: 16px; }
.weigh { flex-wrap: wrap; }
.alert { margin-bottom: 12px; }
</style>
