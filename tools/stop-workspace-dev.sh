#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

workspace_path="${CONDUCTOR_WORKSPACE_PATH:-$PWD}"
workspace_path="$(cd "$workspace_path" && pwd -P)"
supervisor_path="$workspace_path/tools/dev-supervisor.sh"
supervisor_pids=()
workspace_pids=()

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

while read -r pid command; do
	if [[ "$command" == *"$supervisor_path"* ]]; then
		supervisor_pids+=("$pid")
	fi
done < <(ps -A -o pid= -o command=)

if (( ${#supervisor_pids[@]} == 0 )); then
	exit 0
fi

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
		exit 0
	fi
	sleep 0.05
done

# The pre-TERM snapshot remains valid after children are reparented.
kill -KILL "${workspace_pids[@]}" 2>/dev/null || true
