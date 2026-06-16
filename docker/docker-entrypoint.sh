#!/bin/sh
# Materialize a nats CLI context from Chatto's env vars so that
# `docker exec <container> nats ...` connects to the same NATS that chatto
# itself is using. Runs once per container start.
set -eu

if [ -n "${CHATTO_NATS_CLIENT_URL:-}" ]; then
    nats_dir="${HOME:-/home/chatto}/.config/nats"
    mkdir -p "$nats_dir/context"

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
    } > "$nats_dir/context/chatto.json"

    # Mark chatto as the default context so `nats <cmd>` works without
    # also needing $NATS_CONTEXT (e.g. `nats context info`, which is a
    # context-management command and ignores the env var).
    printf 'chatto\n' > "$nats_dir/context.txt"
fi

exec /chatto "$@"
