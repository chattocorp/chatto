#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Run from cli/ through mise. Go applies -timeout to all -count repetitions
# in a package, not to each repetition. Keep repetitions in one process to
# detect state leaks, with a 15-minute budget per repetition.
set -euo pipefail

count="${CHATTO_GO_STABILITY_COUNT:-3}"
# Bound the decimal input before shell arithmetic to avoid overflow and keep
# the resulting timeout within Go's duration limit.
if [[ ! "$count" =~ ^[1-9][0-9]{0,6}$ ]]; then
    printf '%s\n' 'CHATTO_GO_STABILITY_COUNT must be a positive integer of at most seven digits.' >&2
    exit 2
fi
timeout_minutes=$((15 * count))
printf 'Backend stability: %s repetitions, %sm timeout per package (15m per repetition).\n' "$count" "$timeout_minutes"

exec go test -trimpath -race -tags test_endpoints \
    "-shuffle=${CHATTO_GO_TEST_SHUFFLE:-on}" "-count=$count" "-timeout=${timeout_minutes}m" \
    ./internal/testutil ./internal/core ./internal/connectapi ./internal/http_server ./internal/mcpserver
