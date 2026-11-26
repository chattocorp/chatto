#!/bin/sh
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: Apache-2.0
# Smoke-test a built standalone frontend image on a read-only root file system.
# Usage: docker/frontend-image_test.sh <image>
set -eu

image=${1:?usage: frontend-image_test.sh <image>}
container=chatto-frontend-image-test-$$
mkdir -p .context
scratch=$(mktemp -d .context/frontend-image.XXXXXX)
trap 'docker rm -f "$container" > /dev/null 2>&1 || true; rm -rf "$scratch"' EXIT

# Nginx itself needs these writable mounts on a read-only root file system.
# The entrypoint must not need more.
docker run -d --name "$container" --read-only \
    --tmpfs /var/cache/nginx --tmpfs /var/run \
    -e CHATTO_FRONTEND_ORIGIN=https://app.example.org \
    -e CHATTO_FRONTEND_TITLE='Acme Chat' \
    "$image" > /dev/null

fetch() {
    docker exec "$container" wget -q -O - "$@"
}

ready=
for _ in 1 2 3 4 5 6 7 8 9 10; do
    if fetch http://127.0.0.1/ > "$scratch/index" 2> /dev/null; then
        ready=1
        break
    fi
    sleep 1
done
if [ -z "$ready" ]; then
    docker logs "$container" >&2
    echo 'The frontend image did not start.' >&2
    exit 1
fi

for path in / /chat/example/room /chat/book-club.example.com /chat/book-club.example.com/room /chat/servers /200.html; do
    fetch -S "http://127.0.0.1$path" > "$scratch/page" 2> "$scratch/headers"
    case "$path" in
        /chat/book-club.example.com*) policy=no-referrer ;;
        *) policy=strict-origin-when-cross-origin ;;
    esac
    grep -iq "Referrer-Policy: $policy" "$scratch/headers"
    grep -q '<title>Acme Chat</title>' "$scratch/page"
    grep -q '<meta property="og:image" content="https://app.example.org/icons/social-preview.png" />' "$scratch/page"
    if grep -q 'OG_META_' "$scratch/page"; then exit 1; fi
    test "$(grep -c '<title>' "$scratch/page")" = 1
done
fetch http://127.0.0.1/oauth/frontend-client-metadata.json | grep -q '"client_uri": "https://app.example.org"'
fetch http://127.0.0.1/icons/social-preview.png > /dev/null

docker logs "$container" > "$scratch/log" 2>&1
grep -q 'GET /chat/servers ' "$scratch/log"
if grep -q 'book-club.example.com' "$scratch/log"; then exit 1; fi

echo 'Frontend image tests passed.'
