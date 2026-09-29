<script setup>
// One subject, created or edited. The API owns the rules: the code must be
// short and printable, the species and sex come from a fixed set, and a birth
// date cannot be in the future. This form sends what was typed and shows what
// the API says rather than guessing the rules a second time.
import { computed, h, onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NSelect } from "naive-ui";
import { subjects } from "../api/endpoints.js";
import { useBreadcrumb } from "../stores/breadcrumb.js";
import PageHead from "../components/PageHead.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const crumb = useBreadcrumb();

const SPECIES = ["MOUSE", "RAT"];
const SEXES = ["FEMALE", "MALE", "UNKNOWN"];

// The label carries a stable, locale-independent test hook (Naive UI's
// dropdown options are not native <select> elements, so the browser suite
// cannot address them by value the way it used to).
const speciesOptions = SPECIES.map((value) => ({
  value,
  label: () => h("span", { "data-test": `subject-species-${value}` }, t(`subjects.species${value.charAt(0)}${value.slice(1).toLowerCase()}`)),
}));
const sexOptions = SEXES.map((value) => ({
  value,
  label: () => h("span", { "data-test": `subject-sex-${value}` }, t(`subjects.sex${value.charAt(0)}${value.slice(1).toLowerCase()}`)),
}));

const isNew = computed(() => !route.params.id);
const form = ref({ code: "", species: "MOUSE", sex: "UNKNOWN", strain: "", birthDate: "", notes: "" });
const error = ref("");
const saving = ref(false);

function syncCrumb() {
  crumb.set([
    { label: t("subjects.title"), to: "/subjects" },
    { label: isNew.value ? t("subjects.new") : form.value.code || route.params.id },
  ]);
}

async function load() {
  error.value = "";
  if (!isNew.value) {
    try {
      const s = await subjects.get(route.params.id);
      form.value = {
        code: s.code,
        species: s.species,
        sex: s.sex,
        strain: s.strain || "",
        birthDate: s.birthDate || "",
        notes: s.notes || "",
      };
    } catch (e) {
      error.value = e.message;
    }
  }
  syncCrumb();
}

async function save() {
  error.value = "";
  saving.value = true;
  try {
    // Optional fields are sent as empty strings, which the API reads as "not set".
    const body = {
      code: form.value.code.trim(),
      species: form.value.species,
      sex: form.value.sex,
      strain: form.value.strain.trim(),
      birthDate: form.value.birthDate,
      notes: form.value.notes.trim(),
    };
    if (isNew.value) await subjects.create(body);
    else await subjects.update(route.params.id, body);
    router.push("/subjects");
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(load);
onUnmounted(() => crumb.clear());
</script>

<template>
  <PageHead :title="isNew ? $t('subjects.new') : $t('subjects.edit')" />

  <NAlert v-if="error" type="error" :title="error" data-test="error" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small">
    <form class="form" data-test="subject-form" @submit.prevent="save">
      <label class="fld">
        <span>{{ $t("subjects.code") }}</span>
        <input v-model="form.code" data-test="subject-code" :placeholder="$t('subjects.codePlaceholder')" required />
      </label>

      <label class="fld">
        <span>{{ $t("subjects.species") }}</span>
        <NSelect v-model:value="form.species" data-test="subject-species" :options="speciesOptions" />
      </label>

      <label class="fld">
        <span>{{ $t("subjects.sex") }}</span>
        <NSelect v-model:value="form.sex" data-test="subject-sex" :options="sexOptions" />
      </label>

      <label class="fld">
        <span>{{ $t("subjects.strain") }}</span>
        <input v-model="form.strain" data-test="subject-strain" :placeholder="$t('subjects.strainPlaceholder')" />
      </label>

      <label class="fld">
        <span>{{ $t("subjects.birthDate") }}</span>
        <input type="date" v-model="form.birthDate" data-test="subject-birth-date" />
      </label>

      <label class="fld wide">
        <span>{{ $t("common.notes") }}</span>
        <textarea v-model="form.notes" rows="3" data-test="subject-notes"></textarea>
      </label>

      <div class="actions">
        <NButton type="primary" attr-type="submit" :loading="saving" data-test="subject-save">{{ $t("common.save") }}</NButton>
        <RouterLink class="link" to="/subjects">{{ $t("common.cancel") }}</RouterLink>
      </div>
    </form>
  </NCard>
</template>

<style scoped>
.form { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; align-items: start; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.fld :deep(.n-select) { width: 100%; }
.wide { grid-column: 1 / -1; }
.actions { grid-column: 1 / -1; display: flex; align-items: center; gap: 12px; }
</style>
