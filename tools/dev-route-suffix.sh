#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

source_name="${1:-${CONDUCTOR_WORKSPACE_NAME:-local}}"
route_suffix="$({
	printf '%s' "$source_name" |
		LC_ALL=C tr '[:upper:]' '[:lower:]' |
		LC_ALL=C tr -c 'a-z0-9' '-' |
		tr -s '-'
} | sed 's/^-//; s/-$//' | cut -c 1-63 | sed 's/-$//')"

if [[ -z "$route_suffix" ]]; then
	route_suffix="local"
fi

printf '%s\n' "$route_suffix"
