#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Runs one service of the full development stack as a Geordi task.
#
# `.geordi.yml` gives each service of `mise dev-full` its own task and named
# ports. Geordi gives every task in a workspace all of these ports as
# GEORDI_PORT_<NAME>. This script maps them to the port overrides of the
# `[env]` section of `mise.toml`, so that all services agree on each other's
# ports, then runs the given mise task.
#
# Usage: tools/geordi-dev-stack.sh <mise task>
#   For example: tools/geordi-dev-stack.sh dev-stack-mailpit

set -euo pipefail

: "${GEORDI_PORT_CHATTO_FULL:?run this script through a Geordi task}"

export CHATTO_DEV_CHATTO_PORT_OVERRIDE="$GEORDI_PORT_CHATTO_FULL"
export CHATTO_DEV_CHATTO_URL_OVERRIDE="http://localhost:$GEORDI_PORT_CHATTO_FULL"
export CHATTO_DEV_MAILPIT_PORT_OVERRIDE="${GEORDI_PORT_MAILPIT:?}"
export CHATTO_DEV_SMTP_PORT_OVERRIDE="${GEORDI_PORT_SMTP:?}"
export CHATTO_DEV_LIVEKIT_PORT_OVERRIDE="${GEORDI_PORT_LIVEKIT:?}"
# LiveKit uses one port number for RTC over TCP and over UDP. The two
# protocols have separate port spaces, and Geordi checks only TCP ports.
export CHATTO_DEV_LIVEKIT_RTC_TCP_PORT_OVERRIDE="${GEORDI_PORT_LIVEKIT_RTC:?}"
export CHATTO_DEV_LIVEKIT_RTC_UDP_PORT_OVERRIDE="$GEORDI_PORT_LIVEKIT_RTC"
export CHATTO_DEV_AUTHLING_PORT_OVERRIDE="${GEORDI_PORT_AUTHLING:?}"
export CHATTO_DEV_RUNLING_PORT_OVERRIDE="${GEORDI_PORT_RUNLING:?}"

exec mise run "$@"
