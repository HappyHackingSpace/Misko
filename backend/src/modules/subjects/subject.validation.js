import { z } from "zod";

// Subject domain enums (see docs/DOMAIN.md §2).
const ZYGOSITY = ["WT", "HET", "HOMO"];
const SEX = ["M", "F"];
const STATUS = ["ALIVE", "SACRIFICED", "DEAD"];

// An explicit blank string means "clear this field" -> map it to null so an edit
// can actually unset a value. Leave `undefined` (key absent from a partial PATCH)
// untouched so optional fields are simply omitted, not nulled out across the board.
// `.nullable()` short-circuits before `z.coerce.date()` runs, so null dates stay
// null instead of becoming the epoch.
const blankToNull = (v) => (v === "" ? null : v);

const nullableString = z.preprocess(blankToNull, z.string().nullable().optional());
const nullableDate = z.preprocess(blankToNull, z.coerce.date().nullable().optional());
const nullableEnum = (values) => z.preprocess(blankToNull, z.enum(values).nullable().optional());

// `species` is non-nullable in the schema (has a default). A blank value means
// "use the default", so drop it rather than sending null.
const optionalString = z.preprocess((v) => (v === "" ? undefined : v), z.string().optional());

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
