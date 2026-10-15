import { useI18n } from "vue-i18n";

export const SPECIES = ["MOUSE", "RAT"];
export const SEXES = ["FEMALE", "MALE", "UNKNOWN"];
export const ROLE_TAG = {
  HEALTHY_CONTROL: "success",
  SHAM: "default",
  DISEASE_CONTROL: "error",
  POSITIVE_CONTROL: "warning",
  TREATMENT: "info",
};

// Words for the enums a subject carries, plus locale-aware numbers and dates.
export function useSubjectLabels() {
  const { t, n, locale } = useI18n();
  return {
    species: (value) => t(`subjects.speciesNames.${value}`),
    sex: (value) => t(`subjects.sexNames.${value}`),
    role: (value) => t(`subjects.roles.${value}`),
    grams: (value) => `${n(Number(value), { maximumFractionDigits: 3 })} g`,
    dateTime: (value) => new Intl.DateTimeFormat(locale.value, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)),
    date: (value) => new Intl.DateTimeFormat(locale.value, { dateStyle: "medium" }).format(new Date(value)),
  };
}
