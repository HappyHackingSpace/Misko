import { z } from "zod";

/**
 * Shared Zod field builders for form-backed write payloads.
 *
 * Forms submit every field as a string, so an explicit blank means "clear this
 * field" (-> null). `undefined` (key absent from a partial PATCH) is left alone
 * so optional fields are simply omitted rather than nulled out across the board.
 * `.nullable()` short-circuits before `z.coerce.*` runs, so null values stay null
 * instead of being coerced (e.g. a null date becoming the epoch).
 */
const blankToNull = (v) => (v === "" ? null : v);
const blankToUndefined = (v) => (v === "" ? undefined : v);

export const nullableString = z.preprocess(blankToNull, z.string().nullable().optional());
export const nullableDate = z.preprocess(blankToNull, z.coerce.date().nullable().optional());
export const nullableNumber = z.preprocess(blankToNull, z.coerce.number().nullable().optional());
export const nullableEnum = (values) => z.preprocess(blankToNull, z.enum(values).nullable().optional());

// For non-nullable columns with a DB default: a blank means "use the default",
// so drop it rather than sending null.
export const optionalString = z.preprocess(blankToUndefined, z.string().optional());

export const nullableJson = z.preprocess(blankToNull, z.unknown().nullable().optional());
