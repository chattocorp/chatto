#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -uo pipefail

supervised_pid=""
supervised_pgid=""
cleaning_up=false

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

stop_descendants() {
	if [[ "$cleaning_up" == true ]]; then
		return
	fi
	cleaning_up=true
	trap - HUP INT TERM EXIT

	local descendants
	descendants="$(descendants_of "$$")"
	if [[ -n "$descendants" ]]; then
		# Every mise child task may own a separate process group. Address the
		# complete tree explicitly so stopping the supervisor cannot orphan a
		# backend, Vite, LiveKit, or Mailpit process.
		kill -TERM $descendants 2>/dev/null || true
	fi
	if [[ -n "$supervised_pid" ]]; then
		kill -TERM "$supervised_pid" 2>/dev/null || true
	fi
	if [[ -n "$supervised_pgid" ]]; then
		kill -TERM -- "-$supervised_pgid" 2>/dev/null || true
	fi

	# Conductor gives the Run command 200 ms to stop after SIGHUP before it
	# force-kills that command. Give cooperative children a brief chance to
	# release resources, then reap every PID captured before the tree can be
	# reparented. Avoid per-process polling here: it can consume the entire grace
	# period by itself and make an immediate restart collide with a socket.
	sleep 0.1
	if [[ -n "$descendants" ]]; then
		kill -KILL $descendants 2>/dev/null || true
	fi
	if [[ -n "$supervised_pid" ]]; then
		kill -KILL "$supervised_pid" 2>/dev/null || true
		wait "$supervised_pid" 2>/dev/null || true
	fi
	if [[ -n "$supervised_pgid" ]]; then
		kill -KILL -- "-$supervised_pgid" 2>/dev/null || true
	fi
}

stop_from_signal() {
	local status="$1"
	stop_descendants
	exit "$status"
}

trap 'stop_from_signal 129' HUP
trap 'stop_from_signal 130' INT
trap 'stop_from_signal 143' TERM
trap stop_descendants EXIT

if (( $# == 0 )); then
	set -- mise run --jobs 4 --output prefix dev-backend ::: dev-frontend ::: dev-livekit ::: dev-mailpit
fi

# A separate process group remains addressable even if the supervised command
# exits before one of its descendants. Briefly enabling job control makes Bash
# create that group without requiring a platform-specific setsid utility.
set -m
"$@" &
supervised_pid=$!
set +m
supervised_pgid="$(ps -p "$supervised_pid" -o pgid= 2>/dev/null | tr -d ' ' || true)"
own_pgid="$(ps -p "$$" -o pgid= 2>/dev/null | tr -d ' ' || true)"
if [[ ! "$supervised_pgid" =~ ^[0-9]+$ || "$supervised_pgid" == "$own_pgid" ]]; then
	supervised_pgid=""
fi
wait "$supervised_pid"
status=$?
stop_descendants
exit "$status"
