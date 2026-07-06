#!/bin/sh
# Materialize a nats CLI context from Chatto's env vars so that
# `docker exec <container> nats ...` connects to the same NATS that chatto
# itself is using. Runs once per container start.
set -eu

if [ "$(id -u)" = "0" ]; then
    # LinuxServer-style PUID/PGID support: start as root only long enough to
    # map the internal app user to the operator's chosen host IDs, then drop
    # privileges below. Mounted directories are not recursively chowned here;
    # operators should make writable mounts owned by these IDs.
    PUID="${PUID:-1000}"
    PGID="${PGID:-1000}"

    case "$PUID" in
        ''|*[!0-9]*) echo "PUID must be a numeric user ID, got: $PUID" >&2; exit 1 ;;
    esac
    case "$PGID" in
        ''|*[!0-9]*) echo "PGID must be a numeric group ID, got: $PGID" >&2; exit 1 ;;
    esac

    current_uid="$(id -u chatto)"
    current_gid="$(id -g chatto)"
    if [ "$current_gid" != "$PGID" ]; then
        groupmod -o -g "$PGID" chatto
    fi
    if [ "$current_uid" != "$PUID" ] || [ "$current_gid" != "$PGID" ]; then
        usermod -o -u "$PUID" -g "$PGID" chatto
    fi
    export HOME=/home/chatto
fi

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

if [ "$(id -u)" = "0" ]; then
    if [ -d "${HOME:-/home/chatto}/.config" ]; then
        # The entrypoint writes this internal CLI context before su-exec.
        # Keep that generated container state readable by the runtime user.
        chown -R chatto:chatto "${HOME:-/home/chatto}/.config"
    fi
    exec su-exec chatto:chatto /chatto "$@"
fi

exec /chatto "$@"
