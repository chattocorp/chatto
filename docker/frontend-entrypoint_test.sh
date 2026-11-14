#!/bin/sh
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: Apache-2.0
set -eu

mkdir -p .context
scratch=$(mktemp -d .context/frontend-entrypoint.XXXXXX)
trap 'rm -rf "$scratch"' EXIT

# Exercise the real script with only its image-specific document root replaced.
sed "s|/usr/share/nginx/html|$scratch/html|" \
    "$(dirname "$0")/frontend-entrypoint.sh" > "$scratch/entrypoint.sh"
metadata=$scratch/html/oauth/frontend-client-metadata.json

for origin in https://app.chatto.run https://APP.CHATTO.RUN:443/; do
    CHATTO_FRONTEND_ORIGIN=$origin sh "$scratch/entrypoint.sh" printf '%s\n' 'argument with spaces' > "$scratch/command"
    test "$(cat "$scratch/command")" = 'argument with spaces'
    cat > "$scratch/expected" <<'EOF'
{
  "client_id": "https://app.chatto.run/oauth/frontend-client-metadata.json",
  "client_name": "Chatto Web",
  "client_uri": "https://app.chatto.run",
  "application_type": "web",
  "redirect_uris": ["https://app.chatto.run/servers/callback?mode=popup"],
  "token_endpoint_auth_method": "none",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"]
}
EOF
    diff -u "$scratch/expected" "$metadata"
done

CHATTO_FRONTEND_ORIGIN=https://staging.chatto.run:8443 sh "$scratch/entrypoint.sh" true
grep -q '"client_uri": "https://staging.chatto.run:8443"' "$metadata"
CHATTO_FRONTEND_ORIGIN='' sh "$scratch/entrypoint.sh" true
test ! -e "$metadata"
unset CHATTO_FRONTEND_ORIGIN
sh "$scratch/entrypoint.sh" true
test ! -e "$metadata"

for origin in 'http://app.chatto.run' 'https://' 'https://user:secret@app.chatto.run' \
    'https://app.chatto.run/path' 'https://app.chatto.run?x=secret' \
    'https://app.chatto.run#fragment' 'https://app.chatto.run:0' \
    'https://app.chatto.run:65536' 'https://app.chatto.run:0443' \
    'https://app..chatto.run' 'https://-app.chatto.run' 'https://app_.chatto.run' \
    'https://127.1' 'https://0127.0.0.1' 'https://999.1.1.1' \
    'https://[::1]' 'https://app.chatto.run"' \
    'https://app.chatto.run
https://other.chatto.run'; do
    if CHATTO_FRONTEND_ORIGIN=$origin sh "$scratch/entrypoint.sh" touch "$scratch/started" 2> "$scratch/error"; then
        echo 'Invalid origin was accepted' >&2
        exit 1
    fi
    test ! -e "$scratch/started"
    test ! -e "$metadata"
    grep -q '^CHATTO_FRONTEND_ORIGIN must be an HTTPS origin' "$scratch/error"
    test "$(wc -l < "$scratch/error" | tr -d ' ')" = 1
done

echo 'Frontend entrypoint tests passed.'
