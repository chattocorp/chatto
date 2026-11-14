#!/bin/sh
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: Apache-2.0
# Publish this deployment's OAuth identity before starting the static server.
set -eu

metadata=/usr/share/nginx/html/oauth/frontend-client-metadata.json
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

exec "$@"
