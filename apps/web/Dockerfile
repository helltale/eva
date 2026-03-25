# Build from repository root: docker build -f apps/web/Dockerfile .
FROM node:20-alpine AS build
WORKDIR /workspace/apps/web
COPY apps/web/package.json apps/web/package-lock.json* ./
RUN npm install
COPY apps/web/. .
COPY packages/ts-client-generated /workspace/packages/ts-client-generated
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=build /workspace/apps/web/dist /usr/share/nginx/html
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
