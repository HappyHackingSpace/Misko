/*
  Warnings:

  - You are about to drop the column `deviceId` on the `Test` table. All the data in the column will be lost.
  - You are about to drop the `Device` table. If the table is not empty, all the data it contains will be lost.

*/
-- DropForeignKey
ALTER TABLE "Test" DROP CONSTRAINT "Test_deviceId_fkey";

-- DropIndex
DROP INDEX "Test_deviceId_idx";

-- AlterTable
ALTER TABLE "Test" DROP COLUMN "deviceId";

-- DropTable
DROP TABLE "Device";
