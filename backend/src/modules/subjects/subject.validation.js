import { z } from "zod";
import {
  nullableString, nullableDate, nullableEnum, nullableNumber, nullableJson, optionalString,
} from "../../common/zodFields.js";

// Subject domain enums (see docs/DOMAIN.md §2, §3).
const ZYGOSITY = ["WT", "HET", "HOMO"];
const SEX = ["M", "F"];
const STATUS = ["ALIVE", "SACRIFICED", "DEAD"];
const ROUTES = ["IP", "ORAL", "SC", "IV", "IN"];

// Shared, optional fields (everything except the unique `code`).
const subjectFields = {
  microchipId: nullableString,
  earTag: nullableString,
  species: optionalString,
  strain: nullableString,
  line: nullableString,
  genotype: nullableString,
  zygosity: nullableEnum(ZYGOSITY),
  sex: nullableEnum(SEX),
  birthDate: nullableDate,
  coatColor: nullableString,
  healthStatus: nullableString,
  notes: nullableString,
  cageId: nullableString,
  litter: nullableString,
  cohort: nullableString,
  status: nullableEnum(STATUS),
  acquiredAt: nullableDate,
  sacrificedAt: nullableDate,
  groupName: nullableString,
};

export const createSubjectSchema = z.object({
  code: z.string().min(1, "Code is required"),
  ...subjectFields,
});

export const updateSubjectSchema = z.object({
  code: z.string().min(1).optional(),
  ...subjectFields,
});

export const weightLogSchema = z.object({
  grams: z.coerce.number().positive("Weight must be a positive number"),
  measuredAt: nullableDate,
  notes: nullableString,
});

export const diseaseModelLinkSchema = z.object({
  diseaseModelId: z.string().min(1, "diseaseModelId is required"),
  inducedAt: nullableDate,
  method: nullableString,
  notes: nullableString,
});

export const treatmentLinkSchema = z.object({
  treatmentId: z.string().min(1, "treatmentId is required"),
  dose: nullableNumber,
  unit: nullableString,
  route: nullableEnum(ROUTES),
  schedule: nullableJson,
  startedAt: nullableDate,
  endedAt: nullableDate,
});
