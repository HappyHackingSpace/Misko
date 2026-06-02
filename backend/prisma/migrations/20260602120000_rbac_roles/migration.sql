-- RBAC: eski 2 rolü (ADMIN/OPERATOR) 5'li role modeline taşı.
-- Bkz. docs/DOMAIN.md §5b ve src/config/permissions.js.

-- Mevcut kullanıcıları yeni rollere eşle:
--   ADMIN    -> SUPERADMIN  (tam kontrol, bootstrap kullanıcısı dahil)
--   OPERATOR -> RESEARCHER  (denek/test/çalışma yazma + test koşma)
UPDATE "User" SET "role" = 'SUPERADMIN' WHERE "role" = 'ADMIN';
UPDATE "User" SET "role" = 'RESEARCHER' WHERE "role" = 'OPERATOR';

-- Yeni kullanıcılar için varsayılan rolü güncelle.
ALTER TABLE "User" ALTER COLUMN "role" SET DEFAULT 'RESEARCHER';
