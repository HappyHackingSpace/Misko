#!/bin/sh
set -e

echo "→ Veritabanı migration'ları uygulanıyor (prisma migrate deploy)..."
npx prisma migrate deploy

echo "→ Superadmin bootstrap kontrol ediliyor..."
node prisma/bootstrap-admin.js

echo "→ Uygulama başlatılıyor..."
exec "$@"
