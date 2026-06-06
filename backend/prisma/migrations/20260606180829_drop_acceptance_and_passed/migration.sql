/*
  Warnings:

  - You are about to drop the column `acceptance` on the `Scenario` table. All the data in the column will be lost.
  - You are about to drop the column `passed` on the `Test` table. All the data in the column will be lost.

*/
-- AlterTable
ALTER TABLE "Scenario" DROP COLUMN "acceptance";

-- AlterTable
ALTER TABLE "Test" DROP COLUMN "passed";
