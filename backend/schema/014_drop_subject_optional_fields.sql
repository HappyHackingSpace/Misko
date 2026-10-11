-- Strain, birth date and notes were never used by the laboratory and are removed
-- from the subject. Existing values are dropped with the columns.
ALTER TABLE misko.subjects
    DROP COLUMN strain,
    DROP COLUMN birth_date,
    DROP COLUMN notes;
