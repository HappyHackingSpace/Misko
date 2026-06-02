-- CreateTable
CREATE TABLE "Environment" (
    "id" TEXT NOT NULL,
    "laboratoryId" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "paradigmKey" TEXT NOT NULL,
    "config" JSONB NOT NULL,
    "notes" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Environment_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "Environment_laboratoryId_idx" ON "Environment"("laboratoryId");

-- CreateIndex
CREATE INDEX "Environment_paradigmKey_idx" ON "Environment"("paradigmKey");

-- AddForeignKey
ALTER TABLE "Environment" ADD CONSTRAINT "Environment_laboratoryId_fkey" FOREIGN KEY ("laboratoryId") REFERENCES "Laboratory"("id") ON DELETE CASCADE ON UPDATE CASCADE;
