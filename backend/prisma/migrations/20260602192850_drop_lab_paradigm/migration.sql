-- Drop the per-laboratory paradigm enable/disable table. Replaced by named
-- Environment instances (see Environment model); a paradigm is now a read-only
-- template and is no longer toggled on/off per lab.

-- DropForeignKey
ALTER TABLE "LabParadigm" DROP CONSTRAINT "LabParadigm_laboratoryId_fkey";

-- DropTable
DROP TABLE "LabParadigm";
