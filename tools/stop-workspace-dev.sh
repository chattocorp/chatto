#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Stops the development stack of the workspace that contains this script, and
# reports other processes that use its development ports.
#
# Usage: tools/stop-workspace-dev.sh [PORT_BASE]
#
# PORT_BASE defaults to CONDUCTOR_PORT, or 4000 outside Conductor. A process
# belongs to the stack when it listens on a development port and its working
# directory is inside the workspace. Every service of `mise dev` does. The
# script stops these processes. It does not stop other listeners. It exits
# with status 1 when such a listener remains.

set -euo pipefail

workspace_path="$(cd "$(dirname "$0")/.." && pwd -P)"
port_base="${1:-${CONDUCTOR_PORT:-4000}}"

if [[ ! "$port_base" =~ ^[0-9]+$ ]] || (( port_base < 1 || port_base > 65526 )); then
	echo "development port base must be an integer from 1 through 65526" >&2
	exit 2
fi

if ! command -v lsof >/dev/null 2>&1; then
	echo "lsof is required to check the development ports" >&2
	exit 1
fi

# Offset 1 belongs to `mise dev-frontend`, which may run beside `mise dev`.
tcp_port_offsets=(0 2 3 4 5 6 8 9)
udp_port_offset=7

# Prints "<protocol> <port> <pid>" for each listener on a development port.
listeners() {
	local port_offset port pid
	for port_offset in "${tcp_port_offsets[@]}"; do
		port=$((port_base + port_offset))
		for pid in $(lsof -nP -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true); do
			echo "TCP $port $pid"
		done
	done
	port=$((port_base + udp_port_offset))
	for pid in $(lsof -nP -t -iUDP:"$port" 2>/dev/null || true); do
		echo "UDP $port $pid"
	done
}

is_inside_workspace() {
	local directory
	directory="$(lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' || true)"
	directory="$( (cd "$directory" 2>/dev/null && pwd -P) || true)"
	[[ -n "$directory" && ( "$directory" == "$workspace_path" || "$directory" == "$workspace_path/"* ) ]]
}

stack_pids=()
while read -r _ _ pid; do
	if is_inside_workspace "$pid"; then
		stack_pids+=("$pid")
	fi
done < <(listeners)

if (( ${#stack_pids[@]} > 0 )); then
	kill -TERM "${stack_pids[@]}" 2>/dev/null || true
	for _ in {1..40}; do
		live=false
		for pid in "${stack_pids[@]}"; do
			if kill -0 "$pid" 2>/dev/null; then
				live=true
				break
			fi
		done
		if [[ "$live" == false ]]; then
			break
		fi
		sleep 0.05
	done
	if [[ "$live" == true ]]; then
		kill -KILL "${stack_pids[@]}" 2>/dev/null || true
		sleep 0.1
	fi
fi

has_conflicts=false
while read -r protocol port pid; do
	has_conflicts=true
	process_name="$(ps -p "$pid" -o comm= 2>/dev/null | sed 's/^[[:space:]]*//' || true)"
	printf '  %s port %s: PID %s (%s)\n' "$protocol" "$port" "$pid" "${process_name:-unknown process}" >&2
done < <(listeners)

if [[ "$has_conflicts" == true ]]; then
	echo "Development ports are in use. Stop the listed processes or select another port range." >&2
	exit 1
fi
