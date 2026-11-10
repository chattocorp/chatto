#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Keep Linux dependencies and build output off the host checkout.
set -euo pipefail
tar -C /repo --null -T /input/source-files -cf - | tar -C /workspace -xf -
cd /workspace
exec "$@"
