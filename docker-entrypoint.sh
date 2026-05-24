#!/bin/sh
# Materialize a nats CLI context from Chatto's env vars so that
# `docker exec <container> nats ...` connects to the same NATS that chatto
# itself is using. Runs once per container start.
set -eu

if [ -n "${CHATTO_NATS_CLIENT_URL:-}" ]; then
    ctx_dir="${HOME:-/home/chatto}/.config/nats/context"
    mkdir -p "$ctx_dir"

    # jq isn't in the image; build the JSON by hand. Values come from
    # operator-controlled env vars and Chatto's own config validation
    # rejects malformed URLs, so plain interpolation is acceptable here.
    {
        printf '{\n'
        printf '  "description": "chatto runtime",\n'
        printf '  "url": "%s"' "$CHATTO_NATS_CLIENT_URL"
        if [ -n "${CHATTO_NATS_CLIENT_CREDENTIALS_FILE:-}" ]; then
            printf ',\n  "creds": "%s"' "$CHATTO_NATS_CLIENT_CREDENTIALS_FILE"
        fi
        printf '\n}\n'
    } > "$ctx_dir/chatto.json"
fi

exec chatto "$@"
