#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
driver_dir="$(mktemp -d)"
trap 'rm -rf "$driver_dir"' EXIT

cd "$repo_root/cli"
mise x -- go test -c -tags test_endpoints -o "$driver_dir/client" ./internal/http_server
cd "$repo_root/authling"
mise build
mise deps-playwright
CONFORMANCE_CLIENT_BINARY="$driver_dir/client" mise x -- node tools/conformance/run.mjs --automated --client-driver "$repo_root/tools/oidc-conformance-client.mjs"
