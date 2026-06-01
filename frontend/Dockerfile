# syntax=docker/dockerfile:1

###############################################
# 1) Build aşaması
###############################################
FROM node:26-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

###############################################
# 2) Nginx ile statik sunum
###############################################
FROM nginx:1.31-alpine AS runner
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
