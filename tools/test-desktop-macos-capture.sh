#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: EUPL-1.2

set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"

if [[ -z "${DEVELOPER_DIR:-}" ]] &&
	[[ "$(xcode-select -p)" == "/Library/Developer/CommandLineTools" ]] &&
	[[ -d "/Applications/Xcode.app/Contents/Developer" ]]; then
	export DEVELOPER_DIR="/Applications/Xcode.app/Contents/Developer"
fi

cd "$repository_root/apps/desktop/native/macos-capture-probe"
xcrun swift test
