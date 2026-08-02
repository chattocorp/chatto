FROM node:22-bookworm-slim AS web

RUN corepack enable
WORKDIR /src/authling

COPY authling/.npmrc authling/package.json authling/pnpm-lock.yaml authling/pnpm-workspace.yaml ./
COPY authling/web/package.json web/package.json
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY authling/web web
COPY authling/internal/web internal/web
RUN pnpm --dir web build

FROM golang:1.26.2-bookworm AS builder

WORKDIR /src
COPY go.work go.work.sum ./
COPY cli/go.mod cli/go.sum cli/
COPY pkg pkg
COPY authling authling
COPY --from=web /src/authling/internal/web/assets authling/internal/web/assets

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd authling && CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w" -o /out/authling ./cmd/authling

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1000 authling \
    && adduser -D -u 1000 -G authling -h /data authling \
    && mkdir -p /config /data \
    && chown authling:authling /data

COPY --from=builder /out/authling /authling
COPY LICENSE NOTICE /usr/share/doc/authling/

USER authling
WORKDIR /data
EXPOSE 8080
ENTRYPOINT ["/authling"]
