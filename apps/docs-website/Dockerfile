FROM node:24-alpine AS build

WORKDIR /app

RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/docs-website/package.json apps/docs-website/package.json
RUN pnpm install --frozen-lockfile --filter docs-website...

COPY apps/docs-website apps/docs-website
WORKDIR /app/apps/docs-website
RUN pnpm build

FROM nginxinc/nginx-unprivileged:1.29-alpine

COPY apps/docs-website/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/apps/docs-website/dist /usr/share/nginx/html
COPY LICENSES/Apache-2.0.txt /usr/share/doc/chatto-docs/LICENSE
COPY NOTICE /usr/share/doc/chatto-docs/NOTICE

EXPOSE 8080
