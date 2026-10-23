#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Checks that the development ports are free before the stack starts. It lists
# each process that listens on one of them and exits 1. It never stops a
# process. The `[env]` section of `mise.toml` defines the ports.
#
# Usage: mise check-dev-ports [full]
#   Without arguments, it checks the ports of `mise dev`: Chatto and its NATS.
#   With `full`, it also checks the services of `mise dev-full`.

set -euo pipefail

if ! command -v lsof >/dev/null 2>&1; then
	echo "warning: lsof is not installed, so the development ports are not checked" >&2
	exit 0
fi

blocked=false
check() {
	local protocol="$1" port="$2" pids pid
	# Port zero disables the embedded NATS listener.
	if [[ "$port" == 0 ]]; then
		return
	fi
	if [[ "$protocol" == TCP ]]; then
		pids="$(lsof -nP -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | sort -u || true)"
	else
		pids="$(lsof -nP -t -iUDP:"$port" 2>/dev/null | sort -u || true)"
	fi
	for pid in $pids; do
		blocked=true
		printf '  %s port %s: PID %s (%s)\n' "$protocol" "$port" "$pid" \
			"$(ps -p "$pid" -o command= 2>/dev/null | cut -c1-100 || echo unknown process)" >&2
	done
}

# The Vite port is not checked: `mise dev-frontend` may run beside the stack.
check TCP "${CHATTO_DEV_CHATTO_PORT:?run this script through mise check-dev-ports}"
check TCP "$CHATTO_DEV_NATS_PORT"
if [[ "${1:-}" == full ]]; then
	for port in "$CHATTO_DEV_AUTHLING_PORT" "$CHATTO_DEV_RUNLING_PORT" \
		"$CHATTO_DEV_LIVEKIT_PORT" "$CHATTO_DEV_LIVEKIT_RTC_TCP_PORT" \
		"$CHATTO_DEV_SMTP_PORT" "$CHATTO_DEV_MAILPIT_PORT"; do
		check TCP "$port"
	done
	check UDP "$CHATTO_DEV_LIVEKIT_RTC_UDP_PORT"
fi

if [[ "$blocked" == true ]]; then
	echo "error: other processes use the development ports listed above." >&2
	echo "Stop them, for example a previous \`mise dev\`, then start the stack again." >&2
	exit 1
fi
