#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Checks that the ports of `mise dev` are free before the stack starts. It
# lists each process that listens on one of them and exits 1. It never stops
# a process. The comment above the `dev` task in `mise.toml` lists the ports.
#
# Usage: check-dev-ports.sh PORT_BASE

set -euo pipefail

port_base="${1:-}"
if [[ ! "$port_base" =~ ^[0-9]+$ ]] || (( port_base < 1 || port_base > 65526 )); then
	echo "usage: $0 PORT_BASE (an integer from 1 through 65526)" >&2
	exit 2
fi
if ! command -v lsof >/dev/null 2>&1; then
	echo "warning: lsof is not installed, so the development ports are not checked" >&2
	exit 0
fi

blocked=false
report() {
	local protocol="$1" port="$2" pids="$3" pid
	for pid in $pids; do
		blocked=true
		printf '  %s port %s: PID %s (%s)\n' "$protocol" "$port" "$pid" \
			"$(ps -p "$pid" -o command= 2>/dev/null | cut -c1-100 || echo unknown process)" >&2
	done
}

# Offset 1 belongs to `mise dev-frontend`, which may run beside `mise dev`.
for port_offset in 0 2 3 4 5 6 8 9; do
	port=$((port_base + port_offset))
	report TCP "$port" "$(lsof -nP -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | sort -u || true)"
done
port=$((port_base + 7))
report UDP "$port" "$(lsof -nP -t -iUDP:"$port" 2>/dev/null | sort -u || true)"

if [[ "$blocked" == true ]]; then
	echo "error: other processes use the development ports listed above." >&2
	echo "Stop them, for example a previous \`mise dev\`, then start the stack again." >&2
	exit 1
fi
