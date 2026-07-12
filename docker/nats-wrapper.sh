#!/bin/sh
# Make the bundled NATS CLI use Chatto's connection settings without writing a
# CLI context into the container. Explicit NATS connection settings take
# precedence and leave the entire connection setup under operator control.
set -eu

if [ -z "${NATS_URL:-}" ] && [ -z "${NATS_CONTEXT:-}" ]; then
    if [ -n "${CHATTO_NATS_CLIENT_URL:-}" ]; then
        export NATS_URL="$CHATTO_NATS_CLIENT_URL"
    fi
    if [ -z "${NATS_CREDS:-}" ] && [ -n "${CHATTO_NATS_CLIENT_CREDENTIALS_FILE:-}" ]; then
        export NATS_CREDS="$CHATTO_NATS_CLIENT_CREDENTIALS_FILE"
    fi
fi

exec /usr/local/libexec/nats "$@"
