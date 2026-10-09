#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Checks the boundary between Paseo, the launcher, mise, and port preflight.
# Run from the repository root. No development services are started.
set -euo pipefail

mkdir -p .context
scratch=$(mktemp -d "$PWD/.context/test-paseo-dev.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
mkdir "$scratch/bin"

# Check actual mise evaluation of defaults and launcher overrides.
env -u CONDUCTOR_PORT -u CHATTO_DEV_CHATTO_PORT -u CHATTO_DEV_NATS_PORT \
	-u CHATTO_DEV_SMTP_PORT -u CHATTO_DEV_LIVEKIT_URL \
	-u CHATTO_DEV_CHATTO_URL -u CHATTO_DEV_WORKSPACE \
	mise env --json >"$scratch/default.json"
env -u CHATTO_DEV_CHATTO_PORT -u CHATTO_DEV_NATS_PORT \
	-u CHATTO_DEV_SMTP_PORT -u CHATTO_DEV_LIVEKIT_URL \
	-u CHATTO_DEV_CHATTO_URL -u CHATTO_DEV_WORKSPACE CONDUCTOR_PORT=45000 \
	mise env --json >"$scratch/conductor.json"
env CONDUCTOR_PORT=45000 CHATTO_DEV_CHATTO_PORT=53001 \
	CHATTO_DEV_NATS_PORT=0 CHATTO_DEV_CHATTO_URL=https://dev--test.example.com \
	CHATTO_DEV_SMTP_PORT=53009 CHATTO_DEV_LIVEKIT_URL=wss://livekit--test.example.com \
	mise env --json >"$scratch/paseo.json"
python3 - "$scratch" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
for name, port, nats, url in (
    ("default", "4000", "4004", "http://chatto.local.localhost:4000"),
    ("conductor", "45000", "45004", "http://chatto.ws45000.localhost:45000"),
    ("paseo", "53001", "0", "https://dev--test.example.com"),
):
    data = json.loads((root / f"{name}.json").read_text())
    assert data["CHATTO_DEV_CHATTO_PORT"] == port, name
    assert data["CHATTO_DEV_NATS_PORT"] == nats, name
    assert data["CHATTO_DEV_CHATTO_URL"] == url, name
    if name == "paseo":
        assert data["CHATTO_DEV_SMTP_PORT"] == "53009"
        assert data["CHATTO_DEV_LIVEKIT_URL"] == "wss://livekit--test.example.com"
    else:
        base = 4000 if name == "default" else 45000
        assert data["CHATTO_DEV_SMTP_PORT"] == str(base + 8)
        assert data["CHATTO_DEV_LIVEKIT_URL"] == f"ws://localhost:{base + 5}"
PY

cat >"$scratch/bin/mise" <<'SH'
#!/usr/bin/env bash
set -eu
[[ "$*" == dev ]]
printf '%s\n' "$CHATTO_DEV_CHATTO_PORT" "$CHATTO_DEV_CHATTO_URL" "$CHATTO_DEV_NATS_PORT" "$CHATTO_WEBSERVER_BIND_ADDRESS"
SH
chmod +x "$scratch/bin/mise"
actual=$(PATH="$scratch/bin:$PATH" PASEO_PORT=53001 \
	PASEO_URL=https://dev--test.example.com bash tools/paseo-dev.sh)
[[ "$actual" == $'53001\nhttps://dev--test.example.com\n0\n127.0.0.1' ]]

# A separate dev start must not open the active stack's worktree data. The
# owning dev-full process can still launch its own Chatto child.
launcher="$PWD/tools/paseo-dev.sh"
mkdir -p "$scratch/.context/paseo-stack"
printf '%s' "$$" >"$scratch/.context/paseo-stack/owner"
if (cd "$scratch" && PATH="$scratch/bin:$PATH" PASEO_PORT=53001 \
	PASEO_URL=https://dev--test.example.com bash "$launcher" >blocked.log 2>&1); then
	echo 'error: dev accepted an active dev-full owner' >&2
	exit 1
fi
actual=$(cd "$scratch" && PATH="$scratch/bin:$PATH" PASEO_PORT=53001 \
	PASEO_URL=https://dev--test.example.com CHATTO_PASEO_STACK_OWNER="$$" bash "$launcher")
[[ "$actual" == $'53001\nhttps://dev--test.example.com\n0\n127.0.0.1' ]]
for missing in PASEO_PORT PASEO_URL; do
	if env PATH="$scratch/bin:$PATH" PASEO_PORT=53001 \
		PASEO_URL=https://dev--test.example.com env -u "$missing" \
		bash tools/paseo-dev.sh >"$scratch/missing.log" 2>&1; then
		echo "error: launcher accepted missing $missing" >&2
		exit 1
	fi
done

# lsof must never see port zero. A busy real port must still reject startup.
cat >"$scratch/bin/lsof" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$PORT_CHECK_LOG"
if [[ " $* " == *" -iTCP:53002 "* ]]; then
	printf '%s\n' "$$"
fi
SH
chmod +x "$scratch/bin/lsof"
export PORT_CHECK_LOG="$scratch/ports.log"
PATH="$scratch/bin:$PATH" CHATTO_DEV_CHATTO_PORT=53001 \
	CHATTO_DEV_NATS_PORT=0 bash tools/check-dev-ports.sh
[[ $(cat "$PORT_CHECK_LOG") == '-nP -t -iTCP:53001 -sTCP:LISTEN' ]]
if PATH="$scratch/bin:$PATH" CHATTO_DEV_CHATTO_PORT=53002 \
	CHATTO_DEV_NATS_PORT=0 bash tools/check-dev-ports.sh >"$scratch/busy.log" 2>&1; then
	echo 'error: port preflight accepted an occupied port' >&2
	exit 1
fi
echo 'Paseo development launcher checks passed.'
