#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"
supervisor_pid=""
descendants=""

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

is_live() {
	local pid="$1"
	local state
	state="$(ps -p "$pid" -o state= 2>/dev/null | tr -d ' ' || true)"
	[[ -n "$state" && "$state" != Z* ]]
}

cleanup() {
	if [[ -n "$descendants" ]]; then
		kill -KILL $descendants 2>/dev/null || true
	fi
	if [[ -n "$supervisor_pid" ]]; then
		kill -KILL "$supervisor_pid" 2>/dev/null || true
	fi
}
trap cleanup EXIT

"$repository_root/tools/dev-supervisor.sh" bash -c 'sleep 300 & sleep 300 & wait' &
supervisor_pid=$!

for _ in {1..100}; do
	descendants="$(descendants_of "$supervisor_pid")"
	if [[ "$(wc -w <<<"$descendants" | tr -d ' ')" -ge 3 ]]; then
		break
	fi
	sleep 0.02
done

if [[ "$(wc -w <<<"$descendants" | tr -d ' ')" -lt 3 ]]; then
	echo "dev supervisor did not create the expected nested process tree" >&2
	exit 1
fi

kill -HUP "$supervisor_pid"

for _ in {1..100}; do
	still_live=false
	if is_live "$supervisor_pid"; then
		still_live=true
	fi
	for pid in $descendants; do
		if is_live "$pid"; then
			still_live=true
		fi
	done
	if [[ "$still_live" == false ]]; then
		trap - EXIT
		exit 0
	fi
	sleep 0.02
done

echo "dev supervisor left processes running after SIGHUP" >&2
ps -p "$supervisor_pid" $descendants -o pid,ppid,pgid,state,command >&2 || true
exit 1
