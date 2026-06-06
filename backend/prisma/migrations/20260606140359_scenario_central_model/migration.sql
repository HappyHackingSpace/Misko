/*
  Warnings:

  - You are about to drop the column `config` on the `Scenario` table. All the data in the column will be lost.
  - You are about to drop the column `type` on the `Scenario` table. All the data in the column will be lost.
  - You are about to drop the column `acceptanceCriteria` on the `Test` table. All the data in the column will be lost.
  - The `result` column on the `Test` table would be dropped and recreated. This will lead to data loss if there is data in the column.
  - Added the required column `updatedAt` to the `Scenario` table without a default value. This is not possible if the table is not empty.

*/
-- AlterTable
ALTER TABLE "Scenario" DROP COLUMN "config",
DROP COLUMN "type",
ADD COLUMN     "acceptance" JSONB,
ADD COLUMN     "sessionParams" JSONB,
ADD COLUMN     "updatedAt" TIMESTAMP(3) NOT NULL;

-- AlterTable
ALTER TABLE "Test" DROP COLUMN "acceptanceCriteria",
DROP COLUMN "result",
ADD COLUMN     "result" JSONB;

-- CreateTable
CREATE TABLE "_EnvironmentToScenario" (
    "A" TEXT NOT NULL,
    "B" TEXT NOT NULL,

    CONSTRAINT "_EnvironmentToScenario_AB_pkey" PRIMARY KEY ("A","B")
);

-- CreateIndex
CREATE INDEX "_EnvironmentToScenario_B_index" ON "_EnvironmentToScenario"("B");

-- AddForeignKey
ALTER TABLE "_EnvironmentToScenario" ADD CONSTRAINT "_EnvironmentToScenario_A_fkey" FOREIGN KEY ("A") REFERENCES "Environment"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "_EnvironmentToScenario" ADD CONSTRAINT "_EnvironmentToScenario_B_fkey" FOREIGN KEY ("B") REFERENCES "Scenario"("id") ON DELETE CASCADE ON UPDATE CASCADE;
