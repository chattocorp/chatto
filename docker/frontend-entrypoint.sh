#!/bin/sh
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: Apache-2.0
# Publish this deployment's OAuth identity and link-preview metadata before
# starting the static server.
#
# Generated files go to a runtime directory, not to the web root. Nginx also
# needs /var/run for its pid file, so a read-only root file system needs no
# additional writable mount. nginx.conf serves these files from this directory.
set -eu

runtime=/var/run/chatto-frontend
metadata=$runtime/oauth/frontend-client-metadata.json
origin=${CHATTO_FRONTEND_ORIGIN:-}

if [ -n "$origin" ]; then
    # Accept a DNS hostname and optional port, not a URL with credentials,
    # paths, queries, or fragments. The allowlist also makes JSON insertion safe.
    if ! printf '%s\n' "$origin" | awk '
        NR != 1 { invalid = 1 }
        {
            if ($0 !~ /^https:\/\/[A-Za-z0-9.-]+(:[0-9]+)?\/?$/) invalid = 1
            sub(/^https:\/\//, "")
            sub(/\/$/, "")
            count = split($0, authority, ":")
            if (count == 2 && (authority[2] !~ /^[1-9][0-9]*$/ || authority[2] + 0 > 65535)) invalid = 1
            if (length(authority[1]) > 253) invalid = 1
            labels = split(authority[1], hostname, ".")
            for (i = 1; i <= labels; i++) {
                label = hostname[i]
                if (length(label) > 63 || label !~ /^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$/) invalid = 1
            }
            # Browsers interpret numeric final labels as IPv4 addresses.
            # Require canonical dotted decimal so the client ID stays exact.
            if (hostname[labels] ~ /^[0-9]+$/ || hostname[labels] ~ /^0[xX][0-9a-fA-F]+$/) {
                if (labels != 4) invalid = 1
                for (i = 1; i <= labels; i++) {
                    if (hostname[i] !~ /^(0|[1-9][0-9]*)$/ || hostname[i] + 0 > 255) invalid = 1
                }
            }
        }
        END { exit invalid || NR != 1 }
    '; then
        echo 'CHATTO_FRONTEND_ORIGIN must be an HTTPS origin with a DNS hostname and optional port (1-65535), without credentials, path, query, or fragment.' >&2
        exit 1
    fi

    # Match the canonical origin that browsers use to construct the client ID.
    origin=$(printf '%s' "${origin%/}" | tr '[:upper:]' '[:lower:]')
    origin=${origin%:443}
    mkdir -p "$(dirname "$metadata")"
    cat > "$metadata" <<EOF
{
  "client_id": "$origin/oauth/frontend-client-metadata.json",
  "client_name": "Chatto Web",
  "client_uri": "$origin",
  "application_type": "web",
  "redirect_uris": ["$origin/servers/callback?mode=popup"],
  "token_endpoint_auth_method": "none",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"]
}
EOF
else
    # A container can restart with a different environment and the same files.
    rm -f "$metadata"
fi

# Link previews. Every start renders the SPA document from the unmodified
# build output, so a restart with different values replaces the earlier tags.
document=$runtime/200.html
template=/usr/share/nginx/200.html.template
title=${CHATTO_FRONTEND_TITLE:-Chatto}
description=${CHATTO_FRONTEND_DESCRIPTION:-Chat with your teams and communities. Sign in to any Chatto server and keep all your conversations in one place.}
image=${CHATTO_FRONTEND_IMAGE:-/icons/social-preview.png}

case $image in
    https://*) ;;
    /*) image=${origin:+$origin$image} ;;
    *)
        echo 'CHATTO_FRONTEND_IMAGE must be an HTTPS URL or a path that starts with /.' >&2
        exit 1
        ;;
esac

escape_html() {
    printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' \
        -e 's/"/\&quot;/g' -e "s/'/\&#39;/g" | tr '\n\r\t' '   '
}

meta() {
    printf '<meta %s="%s" content="%s" />\n' "$1" "$2" "$(escape_html "$3")"
}

mkdir -p "$runtime"
tags=$(
    printf '<title>%s</title>\n' "$(escape_html "$title")"
    printf '<meta name="description" content="%s" />\n' "$(escape_html "$description")"
    meta property og:type website
    meta property og:site_name "$title"
    meta property og:title "$title"
    meta property og:description "$description"
    if [ -n "$origin" ]; then meta property og:url "$origin/"; fi
    if [ -n "$image" ]; then
        meta property og:image "$image"
        meta name twitter:card summary_large_image
        meta name twitter:image "$image"
    else
        meta name twitter:card summary
    fi
    meta name twitter:title "$title"
    meta name twitter:description "$description"
)
# Replace the block of default tags, including its markers. index() matches
# literal text, and ENVIRON avoids escape processing of the values.
if ! TAGS=$tags awk '
    { doc = doc $0 "\n" }
    END {
        start = index(doc, "<!-- OG_META_START -->")
        end = index(doc, "<!-- OG_META_END -->")
        if (!start || end < start) exit 1
        printf "%s%s%s", substr(doc, 1, start - 1), ENVIRON["TAGS"], substr(doc, end + 20)
    }
' "$template" > "$document.tmp"; then
    echo "$template has no OG_META_START and OG_META_END block." >&2
    exit 1
fi
mv "$document.tmp" "$document"
# gzip_static would serve an older compressed copy before the new document.
rm -f "$document.gz"
if command -v gzip > /dev/null; then gzip -c "$document" > "$document.gz"; fi

exec "$@"
