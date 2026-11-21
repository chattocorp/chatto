#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: EUPL-1.2

# Keep Linux dependencies and build output off the host checkout.
set -euo pipefail
tar -C /repo --null -T /input/source-files -cf - | tar -C /workspace -xf -
cd /workspace
exec "$@"
