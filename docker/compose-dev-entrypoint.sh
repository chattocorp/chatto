#!/usr/bin/env bash
# Run bind-mounted Chatto development services with container-native
# dependencies and polling file watchers.
set -Eeuo pipefail

readonly workspace=/workspace
readonly pnpm_store=/pnpm-store
declare -a child_pids=()

cd "$workspace"

stop_children() {
    if ((${#child_pids[@]} == 0)); then
        return
    fi

    kill -TERM "${child_pids[@]}" 2>/dev/null || true
    wait "${child_pids[@]}" 2>/dev/null || true
}

wait_for_children() {
    local status=0

    trap stop_children EXIT HUP INT TERM
    wait -n "${child_pids[@]}" || status=$?
    return "$status"
}

install_chatto_web_dependencies() {
    pnpm install \
        --frozen-lockfile \
        --store-dir "$pnpm_store" \
        --filter chatto-frontend...
    pnpm --filter @chatto/api-types --filter @chatto/lingua build
}

watch_chatto_packages() {
    watchexec \
        --restart \
        --poll 500ms \
        --watch packages/api-types \
        --watch packages/lingua \
        --ignore 'packages/api-types/dist/**' \
        --ignore 'packages/lingua/dist/**' \
        --exts ts,json \
        -- pnpm --filter @chatto/api-types --filter @chatto/lingua build
}

run_chatto_backend() {
    cd "$workspace/cli"
    go build -trimpath -tags "bootstrap nomsgpack" \
        -ldflags "-X main.Version=${CHATTO_DEVELOPMENT_VERSION:-0.5.0-dev}" \
        -o /tmp/chatto-dev .
    cd /data
    exec /tmp/chatto-dev run
}

run_chatto() {
    install_chatto_web_dependencies
    mkdir -p cli/internal/http_server/.client
    touch cli/internal/http_server/.client/.gitkeep

    watchexec \
        --restart \
        --poll 500ms \
        --watch cli \
        --watch pkg \
        --ignore 'cli/bin/**' \
        --ignore 'cli/internal/http_server/.client/**' \
        -- docker/compose-dev-entrypoint.sh chatto-backend &
    child_pids+=("$!")

    watch_chatto_packages &
    child_pids+=("$!")

    pnpm --filter chatto-frontend dev &
    child_pids+=("$!")

    wait_for_children
}

run_authling_server() {
    cd "$workspace/authling"
    GOWORK=off go tool templ generate
    pnpm --dir web build
    GOWORK=off go build -trimpath -o /tmp/authling-dev ./cmd/authling
    cd /data
    exec /tmp/authling-dev run
}

run_authling() {
    (
        cd authling
        pnpm install \
            --frozen-lockfile \
            --store-dir "$pnpm_store" \
            --filter authling-web-assets...
    )

    exec watchexec \
        --restart \
        --poll 500ms \
        --watch authling \
        --watch pkg \
        --ignore 'authling/bin/**' \
        --ignore 'authling/internal/web/assets/**' \
        --ignore authling/internal/web/pages_templ.go \
        -- docker/compose-dev-entrypoint.sh authling-server
}

run_storybook() {
    install_chatto_web_dependencies

    watch_chatto_packages &
    child_pids+=("$!")

    pnpm --dir apps/frontend exec storybook dev \
        --host 0.0.0.0 \
        --port 8080 \
        --no-open &
    child_pids+=("$!")

    wait_for_children
}

case "${1:-}" in
    authling)
        run_authling
        ;;
    authling-server)
        run_authling_server
        ;;
    chatto)
        run_chatto
        ;;
    chatto-backend)
        run_chatto_backend
        ;;
    storybook)
        run_storybook
        ;;
    *)
        echo "usage: $0 {authling|chatto|storybook}" >&2
        exit 2
        ;;
esac
