#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"
supervisor_pid=""
descendants=""
all_test_pids=""

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
	if [[ -n "$all_test_pids" ]]; then
		kill -KILL $all_test_pids 2>/dev/null || true
	fi
}
trap cleanup EXIT

assert_signal_cleanup() {
	local signal="$1"
	local still_live
	# Background jobs inherit an ignored SIGINT from non-interactive shells on
	# macOS. Reset dispositions before exec so the supervisor's traps are what
	# this test actually exercises.
	perl -e '$SIG{HUP} = $SIG{INT} = $SIG{TERM} = "DEFAULT"; exec @ARGV' \
		"$repository_root/tools/dev-supervisor.sh" \
		bash -c 'trap "" HUP INT TERM; sleep 300 & sleep 300 & wait' &
	supervisor_pid=$!
	all_test_pids+=" $supervisor_pid"
	descendants=""

	for _ in {1..100}; do
		descendants="$(descendants_of "$supervisor_pid")"
		if [[ "$(wc -w <<<"$descendants" | tr -d ' ')" -ge 3 ]]; then
			break
		fi
		sleep 0.02
	done
	all_test_pids+=" $descendants"

	if [[ "$(wc -w <<<"$descendants" | tr -d ' ')" -lt 3 ]]; then
		echo "dev supervisor did not create the expected nested process tree for $signal" >&2
		exit 1
	fi

	kill -"$signal" "$supervisor_pid"

	# Conductor force-kills the Run command after 200 ms. Leave margin for CI
	# scheduling while still failing well before the previous two-second wait.
	for _ in {1..6}; do
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
			wait "$supervisor_pid" 2>/dev/null || true
			return
		fi
		sleep 0.02
	done

	echo "dev supervisor left processes running past Conductor's $signal grace period" >&2
	for pid in "$supervisor_pid" $descendants; do
		ps -p "$pid" -o pid,ppid,pgid,state,command >&2 || true
	done
	exit 1
}

assert_signal_cleanup HUP
assert_signal_cleanup TERM
assert_signal_cleanup INT

natural_exit_directory="$(mktemp -d)"
grandchild_file="$natural_exit_directory/grandchild.pid"
"$repository_root/tools/dev-supervisor.sh" bash -c '
	trap "" HUP INT TERM
	sleep 300 &
	echo "$!" >"$1"
	sleep 0.05
' -- "$grandchild_file" &
supervisor_pid=$!
all_test_pids+=" $supervisor_pid"
for _ in {1..100}; do
	if [[ -s "$grandchild_file" ]]; then
		break
	fi
	sleep 0.01
done
if [[ ! -s "$grandchild_file" ]]; then
	echo "supervised command did not record its grandchild" >&2
	exit 1
fi
grandchild_pid="$(cat "$grandchild_file")"
all_test_pids+=" $grandchild_pid"
wait "$supervisor_pid"
if is_live "$grandchild_pid"; then
	echo "dev supervisor left grandchild $grandchild_pid running after natural command exit" >&2
	exit 1
fi
rm -f "$grandchild_file"
rmdir "$natural_exit_directory"

trap - EXIT
