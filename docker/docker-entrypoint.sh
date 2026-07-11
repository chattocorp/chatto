#!/bin/sh
# Apply the configured runtime user and start Chatto.
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

if [ "$(id -u)" = "0" ]; then
    exec su-exec chatto:chatto /chatto "$@"
fi

exec /chatto "$@"
