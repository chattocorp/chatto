#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Starts Chatto through Paseo's supervised dev service. Paseo owns the port,
# proxy route, and process lifetime. Use exec so stop signals reach mise,
# which stops all processes it starts. Data stays in this worktree's cli/data.
set -euo pipefail

: "${PASEO_PORT:?start the dev service through Paseo}"
: "${PASEO_URL:?start the dev service through Paseo}"

export CHATTO_DEV_CHATTO_PORT="$PASEO_PORT"
export CHATTO_DEV_CHATTO_URL="$PASEO_URL"
export CHATTO_WEBSERVER_BIND_ADDRESS=127.0.0.1
# Paseo allocates one port, not a port block. Chatto connects to its embedded
# NATS server in process, so this service needs no separate NATS TCP listener.
export CHATTO_DEV_NATS_PORT=0

exec mise dev
