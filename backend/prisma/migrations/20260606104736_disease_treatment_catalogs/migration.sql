-- CreateTable
CREATE TABLE "DiseaseModel" (
    "id" TEXT NOT NULL,
    "key" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "category" TEXT,
    "description" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "DiseaseModel_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "SubjectDiseaseModel" (
    "id" TEXT NOT NULL,
    "subjectId" TEXT NOT NULL,
    "diseaseModelId" TEXT NOT NULL,
    "inducedAt" TIMESTAMP(3),
    "method" TEXT,
    "notes" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "SubjectDiseaseModel_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Treatment" (
    "id" TEXT NOT NULL,
    "key" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "defaultDose" DOUBLE PRECISION,
    "unit" TEXT,
    "route" TEXT,
    "notes" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "Treatment_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "SubjectTreatment" (
    "id" TEXT NOT NULL,
    "subjectId" TEXT NOT NULL,
    "treatmentId" TEXT NOT NULL,
    "dose" DOUBLE PRECISION,
    "unit" TEXT,
    "route" TEXT,
    "schedule" JSONB,
    "startedAt" TIMESTAMP(3),
    "endedAt" TIMESTAMP(3),
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "SubjectTreatment_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "DiseaseModel_key_key" ON "DiseaseModel"("key");

-- CreateIndex
CREATE INDEX "SubjectDiseaseModel_subjectId_idx" ON "SubjectDiseaseModel"("subjectId");

-- CreateIndex
CREATE INDEX "SubjectDiseaseModel_diseaseModelId_idx" ON "SubjectDiseaseModel"("diseaseModelId");

-- CreateIndex
CREATE UNIQUE INDEX "SubjectDiseaseModel_subjectId_diseaseModelId_key" ON "SubjectDiseaseModel"("subjectId", "diseaseModelId");

-- CreateIndex
CREATE UNIQUE INDEX "Treatment_key_key" ON "Treatment"("key");

-- CreateIndex
CREATE INDEX "SubjectTreatment_subjectId_idx" ON "SubjectTreatment"("subjectId");

-- CreateIndex
CREATE INDEX "SubjectTreatment_treatmentId_idx" ON "SubjectTreatment"("treatmentId");

-- CreateIndex
CREATE UNIQUE INDEX "SubjectTreatment_subjectId_treatmentId_key" ON "SubjectTreatment"("subjectId", "treatmentId");

-- AddForeignKey
ALTER TABLE "SubjectDiseaseModel" ADD CONSTRAINT "SubjectDiseaseModel_subjectId_fkey" FOREIGN KEY ("subjectId") REFERENCES "Subject"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "SubjectDiseaseModel" ADD CONSTRAINT "SubjectDiseaseModel_diseaseModelId_fkey" FOREIGN KEY ("diseaseModelId") REFERENCES "DiseaseModel"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "SubjectTreatment" ADD CONSTRAINT "SubjectTreatment_subjectId_fkey" FOREIGN KEY ("subjectId") REFERENCES "Subject"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "SubjectTreatment" ADD CONSTRAINT "SubjectTreatment_treatmentId_fkey" FOREIGN KEY ("treatmentId") REFERENCES "Treatment"("id") ON DELETE CASCADE ON UPDATE CASCADE;
