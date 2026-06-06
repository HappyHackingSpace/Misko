-- AlterTable
ALTER TABLE "Subject" ADD COLUMN     "acquiredAt" TIMESTAMP(3),
ADD COLUMN     "cageId" TEXT,
ADD COLUMN     "coatColor" TEXT,
ADD COLUMN     "cohort" TEXT,
ADD COLUMN     "earTag" TEXT,
ADD COLUMN     "genotype" TEXT,
ADD COLUMN     "healthStatus" TEXT,
ADD COLUMN     "line" TEXT,
ADD COLUMN     "litter" TEXT,
ADD COLUMN     "microchipId" TEXT,
ADD COLUMN     "sacrificedAt" TIMESTAMP(3),
ADD COLUMN     "species" TEXT NOT NULL DEFAULT 'Mus musculus',
ADD COLUMN     "status" TEXT NOT NULL DEFAULT 'ALIVE',
ADD COLUMN     "strain" TEXT,
ADD COLUMN     "zygosity" TEXT;

-- CreateTable
CREATE TABLE "WeightLog" (
    "id" TEXT NOT NULL,
    "subjectId" TEXT NOT NULL,
    "measuredAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "grams" DOUBLE PRECISION NOT NULL,
    "notes" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "WeightLog_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "WeightLog_subjectId_idx" ON "WeightLog"("subjectId");

-- CreateIndex
CREATE INDEX "Subject_status_idx" ON "Subject"("status");

-- CreateIndex
CREATE INDEX "Subject_strain_idx" ON "Subject"("strain");

-- AddForeignKey
ALTER TABLE "WeightLog" ADD CONSTRAINT "WeightLog_subjectId_fkey" FOREIGN KEY ("subjectId") REFERENCES "Subject"("id") ON DELETE CASCADE ON UPDATE CASCADE;
