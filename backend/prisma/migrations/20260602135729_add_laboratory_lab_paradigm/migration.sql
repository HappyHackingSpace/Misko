-- CreateTable
CREATE TABLE "Laboratory" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "code" TEXT,
    "timezone" TEXT NOT NULL DEFAULT 'Europe/Istanbul',
    "settings" JSONB NOT NULL DEFAULT '{}',
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "Laboratory_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "LabParadigm" (
    "id" TEXT NOT NULL,
    "laboratoryId" TEXT NOT NULL,
    "paradigmKey" TEXT NOT NULL,
    "enabled" BOOLEAN NOT NULL DEFAULT true,
    "labDefaults" JSONB,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "LabParadigm_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "Laboratory_code_key" ON "Laboratory"("code");

-- CreateIndex
CREATE INDEX "LabParadigm_laboratoryId_idx" ON "LabParadigm"("laboratoryId");

-- CreateIndex
CREATE UNIQUE INDEX "LabParadigm_laboratoryId_paradigmKey_key" ON "LabParadigm"("laboratoryId", "paradigmKey");

-- AddForeignKey
ALTER TABLE "LabParadigm" ADD CONSTRAINT "LabParadigm_laboratoryId_fkey" FOREIGN KEY ("laboratoryId") REFERENCES "Laboratory"("id") ON DELETE CASCADE ON UPDATE CASCADE;
