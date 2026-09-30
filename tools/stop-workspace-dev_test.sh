#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Tests tools/stop-workspace-dev.sh in a temporary workspace.

set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"
scratch="$(cd "$(mktemp -d)" && pwd -P)"
listener_pids=""

cleanup() {
	if [[ -n "$listener_pids" ]]; then
		kill -KILL $listener_pids 2>/dev/null || true
	fi
	rm -rf "$scratch"
}
trap cleanup EXIT

is_live() {
	local state
	state="$(ps -p "$1" -o state= 2>/dev/null | tr -d ' ' || true)"
	[[ -n "$state" && "$state" != Z* ]]
}

# Starts a TCP listener on port $2 with working directory $1 and waits until it
# accepts connections. Sets listener_pid.
start_listener() {
	local directory="$1"
	local port="$2"
	(
		cd "$directory"
		exec perl -MIO::Socket::INET -e '
			my $port = shift;
			my $socket = IO::Socket::INET->new(
				LocalAddr => "127.0.0.1",
				LocalPort => $port,
				Proto => "tcp",
				Listen => 5,
			) or die "listen on TCP port $port: $!";
			sleep 300;
		' "$port"
	) &
	listener_pid=$!
	listener_pids+=" $listener_pid"
	for _ in {1..100}; do
		if lsof -nP -a -p "$listener_pid" -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | grep -q .; then
			return
		fi
		sleep 0.01
	done
	echo "test listener did not listen on TCP port $port" >&2
	exit 1
}

# Conductor can reach a workspace through a symlink, so run the script through one.
workspace="$scratch/workspace"
workspace_alias="$scratch/workspace-alias"
mkdir -p "$workspace/tools" "$workspace/authling" "$scratch/elsewhere"
cp "$repository_root/tools/stop-workspace-dev.sh" "$workspace/tools/"
ln -s "$workspace" "$workspace_alias"
stop="$workspace_alias/tools/stop-workspace-dev.sh"

for port_base in $(seq 42000 10 60000); do
	if ! lsof -nP -iTCP:"$port_base-$((port_base + 9))" -sTCP:LISTEN 2>/dev/null | grep -q . &&
		! lsof -nP -iUDP:"$port_base-$((port_base + 9))" 2>/dev/null | grep -q .; then
		break
	fi
done

# A service of the workspace's stack stops, even when no other process of the
# stack remains, for example after a SIGKILL to the `mise dev` process group.
start_listener "$workspace/authling" "$((port_base + 2))"
stack_listener_pid="$listener_pid"
"$stop" "$port_base"
wait "$stack_listener_pid" 2>/dev/null || true
if is_live "$stack_listener_pid"; then
	echo "stop script left a service of the workspace running" >&2
	exit 1
fi

# A process from outside the workspace blocks startup, but it does not stop.
start_listener "$scratch/elsewhere" "$port_base"
foreign_listener_pid="$listener_pid"
conflict_output="$scratch/conflict.txt"
if "$stop" "$port_base" 2>"$conflict_output"; then
	echo "stop script did not report a foreign port listener" >&2
	exit 1
fi
grep -F "TCP port $port_base: PID $foreign_listener_pid" "$conflict_output" >/dev/null
if ! is_live "$foreign_listener_pid"; then
	echo "stop script stopped a foreign port listener" >&2
	exit 1
fi
kill -TERM "$foreign_listener_pid"
wait "$foreign_listener_pid" 2>/dev/null || true
"$stop" "$port_base"

echo "stop-workspace-dev tests passed"
