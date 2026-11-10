# The workflow takes these versions from mise.toml, package.json, and the installed
# frontend Playwright package. Application source is mounted when the jobs run.
ARG NODE_VERSION
ARG GO_VERSION
ARG PLAYWRIGHT_VERSION
FROM node:${NODE_VERSION}-bookworm-slim AS node
FROM golang:${GO_VERSION}-bookworm AS go
FROM mcr.microsoft.com/playwright:v${PLAYWRIGHT_VERSION}-noble

COPY --from=ghcr.io/jdx/mise:2026.9.12 /usr/local/bin/mise /usr/local/bin/mise
COPY --from=node /usr/local/ /usr/local/
COPY --from=go /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" \
    CI=1 \
    GOFLAGS=-trimpath \
    GOTOOLCHAIN=local \
    TURBO_TELEMETRY_DISABLED=1 \
    TURBO_CACHE=local:rw \
    STORYBOOK_DISABLE_TELEMETRY=1 \
    DO_NOT_TRACK=1 \
    MISE_TRUSTED_CONFIG_PATHS=/workspace \
    MISE_TASK_RUN_AUTO_INSTALL=false

ARG PNPM_VERSION
RUN apt-get update \
    && apt-get install --no-install-recommends -y build-essential git ca-certificates tini \
    && rm -rf /var/lib/apt/lists/* \
    && npm install --global "pnpm@${PNPM_VERSION}"

WORKDIR /workspace
ENTRYPOINT ["/usr/bin/tini", "-g", "--", "bash", "/repo/ci/docker/container.sh"]
