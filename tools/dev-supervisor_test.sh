#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"
supervisor_pid=""
descendants=""
all_test_pids=""
archive_workspace=""
archive_workspace_alias=""
conflict_output=""

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
	if [[ -n "$archive_workspace" ]]; then
		rm -rf "$archive_workspace"
	fi
	if [[ -n "$archive_workspace_alias" ]]; then
		rm -f "$archive_workspace_alias"
	fi
	if [[ -n "$conflict_output" ]]; then
		rm -f "$conflict_output"
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

# The archive fallback must kill the recorded child tree even if the supervisor
# cannot run its TERM trap.
archive_workspace="$(mktemp -d)"
archive_workspace="$(cd "$archive_workspace" && pwd -P)"
archive_workspace_alias="$archive_workspace-alias"
mkdir -p "$archive_workspace/tools"
cp "$repository_root/tools/dev-supervisor.sh" "$archive_workspace/tools/dev-supervisor.sh"
chmod +x "$archive_workspace/tools/dev-supervisor.sh"
ln -s "$archive_workspace" "$archive_workspace_alias"
(
	cd "$archive_workspace"
	exec perl -e '$SIG{HUP} = $SIG{INT} = $SIG{TERM} = "DEFAULT"; exec @ARGV' \
		"$archive_workspace_alias/tools/dev-supervisor.sh" \
		bash -c 'trap "" HUP INT TERM; sleep 300 & sleep 300 & wait'
) &
supervisor_pid=$!
all_test_pids+=" $supervisor_pid"
for _ in {1..100}; do
	descendants="$(descendants_of "$supervisor_pid")"
	if [[ "$(wc -w <<<"$descendants" | tr -d ' ')" -ge 3 ]]; then
		break
	fi
	sleep 0.02
done
all_test_pids+=" $descendants"
if [[ "$(wc -w <<<"$descendants" | tr -d ' ')" -lt 3 ]]; then
	echo "archive cleanup test did not create the expected process tree" >&2
	exit 1
fi
kill -STOP "$supervisor_pid"
CONDUCTOR_WORKSPACE_PATH="$archive_workspace" "$repository_root/tools/stop-workspace-dev.sh"
for _ in {1..20}; do
	still_live=false
	for pid in "$supervisor_pid" $descendants; do
		if is_live "$pid"; then
			still_live=true
			break
		fi
	done
	if [[ "$still_live" == false ]]; then
		break
	fi
	sleep 0.02
done
if [[ "$still_live" == true ]]; then
	echo "workspace archive cleanup left the development process tree running" >&2
	exit 1
fi
rm -f "$archive_workspace_alias"
archive_workspace_alias=""
rm -rf "$archive_workspace"
archive_workspace=""

# A process that does not belong to the workspace must block startup without
# being terminated merely because it owns a development port.
for port_base in $(seq 42000 10 60000); do
	if ! lsof -nP -iTCP:"$port_base-$((port_base + 9))" -sTCP:LISTEN 2>/dev/null | grep -q . &&
		! lsof -nP -iUDP:"$port_base-$((port_base + 9))" 2>/dev/null | grep -q .; then
		break
	fi
done
perl -MIO::Socket::INET -e '
	my $port = shift;
	my $socket = IO::Socket::INET->new(
		LocalAddr => "127.0.0.1",
		LocalPort => $port,
		Proto => "tcp",
		Listen => 5,
	) or die "listen on TCP port $port: $!";
	sleep 300;
' "$port_base" &
foreign_listener_pid=$!
all_test_pids+=" $foreign_listener_pid"
for _ in {1..100}; do
	if lsof -nP -a -p "$foreign_listener_pid" -iTCP:"$port_base" -sTCP:LISTEN 2>/dev/null | grep -q .; then
		break
	fi
	sleep 0.01
done
conflict_output="$(mktemp)"
if CONDUCTOR_WORKSPACE_PATH="$repository_root" \
	"$repository_root/tools/stop-workspace-dev.sh" "$port_base" 2>"$conflict_output"; then
	echo "workspace cleanup did not report a foreign port listener" >&2
	exit 1
fi
grep -F "TCP port $port_base: PID $foreign_listener_pid" "$conflict_output" >/dev/null
if ! is_live "$foreign_listener_pid"; then
	echo "workspace cleanup stopped a foreign port listener" >&2
	exit 1
fi
kill -TERM "$foreign_listener_pid"
wait "$foreign_listener_pid" 2>/dev/null || true
rm -f "$conflict_output"
conflict_output=""
CONDUCTOR_WORKSPACE_PATH="$repository_root" \
	"$repository_root/tools/stop-workspace-dev.sh" "$port_base"

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
