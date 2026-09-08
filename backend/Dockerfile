# syntax=docker/dockerfile:1

###############################################
# 1) Bağımlılıklar + Prisma client (engine'ler dahil)
###############################################
FROM node:26-alpine AS deps
WORKDIR /app
RUN apk add --no-cache openssl
COPY package*.json ./
COPY prisma ./prisma
COPY prisma.config.ts ./
# Migration için prisma CLI'ye ihtiyaç var → tüm bağımlılıklar kurulur
RUN npm ci && npx prisma generate

###############################################
# 2) Çalışma imajı
###############################################
FROM node:26-alpine AS runner
ENV NODE_ENV=production
WORKDIR /app
RUN apk add --no-cache openssl

# Güvenlik: root olmayan kullanıcı
RUN addgroup -S nodejs && adduser -S nodejs -G nodejs

COPY --from=deps /app/node_modules ./node_modules
COPY package*.json ./
COPY prisma ./prisma
COPY prisma.config.ts ./
COPY src ./src
COPY docker-entrypoint.sh ./docker-entrypoint.sh
RUN sed -i 's/\r$//' ./docker-entrypoint.sh && chmod +x ./docker-entrypoint.sh && chown -R nodejs:nodejs /app

USER nodejs
EXPOSE 4000

# Migration'ları uygula, ardından sunucuyu başlat
ENTRYPOINT ["./docker-entrypoint.sh"]
CMD ["node", "src/server.js"]
