#!/bin/sh
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: Apache-2.0
set -eu

mkdir -p .context
scratch=$(mktemp -d .context/frontend-entrypoint.XXXXXX)
trap 'rm -rf "$scratch"' EXIT

# Exercise the real script with only its image-specific paths replaced.
sed -e "s|/usr/share/nginx|$scratch|g" -e "s|/var/run|$scratch/run|g" \
    "$(dirname "$0")/frontend-entrypoint.sh" > "$scratch/entrypoint.sh"
metadata=$scratch/run/chatto-frontend/oauth/frontend-client-metadata.json
document=$scratch/run/chatto-frontend/200.html
printf '%s\n' '<html><head>' '    <!-- OG_META_START -->' '    <title>Default</title>' \
    '    <!-- OG_META_END -->' '</head><body></body></html>' > "$scratch/200.html.template"

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

# Link previews: defaults with an origin.
description='Chat with your teams and communities. Sign in to any Chatto server and keep all your conversations in one place.'
mkdir -p "$(dirname "$document")"
printf 'stale' > "$document.gz"
CHATTO_FRONTEND_ORIGIN=https://APP.chatto.run/ sh "$scratch/entrypoint.sh" true
cat > "$scratch/expected" <<EOF
<html><head>
    <title>Chatto</title>
<meta name="description" content="$description" />
<meta property="og:type" content="website" />
<meta property="og:site_name" content="Chatto" />
<meta property="og:title" content="Chatto" />
<meta property="og:description" content="$description" />
<meta property="og:url" content="https://app.chatto.run/" />
<meta property="og:image" content="https://app.chatto.run/icons/social-preview.png" />
<meta name="twitter:card" content="summary_large_image" />
<meta name="twitter:image" content="https://app.chatto.run/icons/social-preview.png" />
<meta name="twitter:title" content="Chatto" />
<meta name="twitter:description" content="$description" />
</head><body></body></html>
EOF
diff -u "$scratch/expected" "$document"
if command -v gzip > /dev/null; then
    gzip -dc "$document.gz" | diff -u "$document" -
else
    test ! -e "$document.gz"
fi

# Overrides are escaped, and a restart renders from the template again.
CHATTO_FRONTEND_TITLE='Acme & "Friends" <Chat>' \
    CHATTO_FRONTEND_DESCRIPTION="Bob's place" \
    CHATTO_FRONTEND_IMAGE=https://cdn.example.com/preview.png \
    sh "$scratch/entrypoint.sh" true
grep -q '<title>Acme &amp; &quot;Friends&quot; &lt;Chat&gt;</title>' "$document"
grep -q '<meta property="og:description" content="Bob&#39;s place" />' "$document"
grep -q '<meta property="og:image" content="https://cdn.example.com/preview.png" />' "$document"
test "$(grep -c '<title>' "$document")" = 1
if grep -q 'og:url' "$document"; then exit 1; fi

# Without an origin, a relative image cannot become an absolute URL.
sh "$scratch/entrypoint.sh" true
if grep -q 'og:image' "$document"; then exit 1; fi
grep -q '<meta name="twitter:card" content="summary" />' "$document"

# A template without the block stops startup.
cp "$scratch/200.html.template" "$scratch/template.orig"
printf '<html></html>\n' > "$scratch/200.html.template"
if sh "$scratch/entrypoint.sh" touch "$scratch/started" 2> "$scratch/error"; then
    echo 'A template without the block was accepted' >&2
    exit 1
fi
test ! -e "$scratch/started"
grep -q 'has no OG_META_START and OG_META_END block' "$scratch/error"
mv "$scratch/template.orig" "$scratch/200.html.template"

for image in http://cdn.example.com/preview.png preview.png; do
    if CHATTO_FRONTEND_IMAGE=$image sh "$scratch/entrypoint.sh" touch "$scratch/started" 2> "$scratch/error"; then
        echo 'Invalid image was accepted' >&2
        exit 1
    fi
    test ! -e "$scratch/started"
    grep -q '^CHATTO_FRONTEND_IMAGE must be' "$scratch/error"
done

echo 'Frontend entrypoint tests passed.'
