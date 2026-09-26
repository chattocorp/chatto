#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

workspace_path="${CONDUCTOR_WORKSPACE_PATH:-$PWD}"
workspace_path="$(cd "$workspace_path" && pwd -P)"
supervisor_path="$workspace_path/tools/dev-supervisor.sh"
port_base="${1:-}"
supervisor_pids=()
workspace_pids=()

if [[ -n "$port_base" ]] &&
	{ [[ ! "$port_base" =~ ^[0-9]+$ ]] || (( port_base < 1 || port_base > 65526 )); }; then
	echo "development port base must be an integer from 1 through 65526" >&2
	exit 2
fi

descendants_of() {
	local root_pid="$1"
	ps -A -o pid=,ppid= | awk -v root_pid="$root_pid" '
		{ parent[$1] = $2 }
		END {
			for (pid in parent) {
				ancestor = pid
				while (ancestor in parent && parent[ancestor] != 0) {
					if (parent[ancestor] == root_pid) {
						print pid
						break
					}
					ancestor = parent[ancestor]
				}
			}
		}
	'
}

working_directory_of() {
	local pid="$1"
	local process_directory

	if [[ -e "/proc/$pid/cwd" ]]; then
		readlink "/proc/$pid/cwd" 2>/dev/null || true
		return
	fi

	process_directory="$(lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' || true)"
	if [[ -n "$process_directory" ]]; then
		(cd "$process_directory" 2>/dev/null && pwd -P) || true
	fi
}

while read -r pid command; do
	if [[ "$command" == *"$supervisor_path"* ]]; then
		supervisor_pids+=("$pid")
	elif [[ "$command" =~ (^|[[:space:]])([^[:space:]]*/)?tools/dev-supervisor\.sh([[:space:]]|$) ]] &&
		[[ "$(working_directory_of "$pid")" == "$workspace_path" ]]; then
		# Older commands used a relative path. Conductor can also rename a
		# workspace through a symlink while its processes keep the physical path.
		# The exact working directory keeps both fallbacks workspace-specific.
		supervisor_pids+=("$pid")
	fi
done < <(ps -A -o pid= -o command=)

if (( ${#supervisor_pids[@]} > 0 )); then
	workspace_pids=("${supervisor_pids[@]}")
	for pid in "${supervisor_pids[@]}"; do
		while read -r descendant_pid; do
			workspace_pids+=("$descendant_pid")
		done < <(descendants_of "$pid")
	done

	kill -TERM "${supervisor_pids[@]}" 2>/dev/null || true

	# The supervisor normally terminates this tree itself.
	for _ in {1..10}; do
		live=false
		for pid in "${workspace_pids[@]}"; do
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
		# The pre-TERM snapshot remains valid after children are reparented.
		kill -KILL "${workspace_pids[@]}" 2>/dev/null || true
	fi
fi

if [[ -z "$port_base" ]]; then
	exit 0
fi

if ! command -v lsof >/dev/null 2>&1; then
	echo "lsof is required to check the development ports" >&2
	exit 1
fi

has_conflicts=false

report_listeners() {
	local protocol="$1"
	local port="$2"
	local listener_pids
	local process_name

	if [[ "$protocol" == TCP ]]; then
		listener_pids="$(lsof -nP -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | sort -n -u || true)"
	else
		listener_pids="$(lsof -nP -t -iUDP:"$port" 2>/dev/null | sort -n -u || true)"
	fi

	while read -r listener_pid; do
		if [[ -z "$listener_pid" ]]; then
			continue
		fi
		has_conflicts=true
		process_name="$(ps -p "$listener_pid" -o comm= 2>/dev/null | sed 's/^[[:space:]]*//' || true)"
		printf '  %s port %s: PID %s (%s)\n' "$protocol" "$port" "$listener_pid" "${process_name:-unknown process}" >&2
	done <<<"$listener_pids"
}

# Offset 1 belongs to `mise dev-frontend`, which may run beside `mise dev`.
for port_offset in 0 2 3 4 5 6 8 9; do
	report_listeners TCP "$((port_base + port_offset))"
done
report_listeners UDP "$((port_base + 7))"

if [[ "$has_conflicts" == true ]]; then
	echo "Development ports are in use. Stop the listed processes or select another port range." >&2
	exit 1
fi
