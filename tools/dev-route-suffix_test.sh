#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"
suffix_command="$repository_root/tools/dev-route-suffix.sh"

assert_suffix() {
	local expected="$1"
	local source_name="$2"
	local actual

	actual="$($suffix_command "$source_name")"
	if [[ "$actual" != "$expected" ]]; then
		echo "expected route suffix '$expected', got '$actual'" >&2
		exit 1
	fi
}

assert_suffix "explore-networked-mcp" "Explore_Networked MCP!"
assert_suffix "session-mcp" "session: mcp"
assert_suffix "local" "___"
assert_suffix "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
