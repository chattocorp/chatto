#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Exercise the actual mise dependency chain. No Chatto server is started.
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd -P)"
cd "$repository_root"
mkdir -p .context
test_directory="$(mktemp -d "$repository_root/.context/chatto-build-test.XXXXXX")"
embedded_asset=""
cleanup() {
	if [[ -n "$embedded_asset" && ! -f "$embedded_asset" && -f "$test_directory/asset-backup" ]]; then
		cp -p "$test_directory/asset-backup" "$embedded_asset"
	fi
	rm -rf "$test_directory"
}
trap cleanup EXIT

run_chatto() {
	local log="$1"
	# CI enables color even for redirected logs. Keep assertions independent of it.
	if ! MISE_COLOR=0 mise run --output prefix --no-timings chatto --version > "$log" 2>&1; then
		cat "$log" >&2
		exit 1
	fi
}

assert_skipped() {
	local log="$1" output task
	output="$(< "$log")"
	for task in deps-frontend deps-cli sync-cli-legal build-frontend build-dev-cli; do
		if [[ "$output" != *"[$task] sources up-to-date, skipping"* ]]; then
			cat "$log" >&2
			echo "Unchanged task did not skip: $task" >&2
			exit 1
		fi
	done
}

run_chatto "$test_directory/first.log"
run_chatto "$test_directory/second.log"
assert_skipped "$test_directory/second.log"

# A single missing asset must invalidate the output tree, even when the
# directory and all source files still exist.
embedded_asset="$repository_root/cli/internal/http_server/.client/200.html"
if [[ ! -f "$embedded_asset" ]]; then
	embedded_asset="$embedded_asset.gz"
fi
cp -p "$embedded_asset" "$test_directory/asset-backup"
rm "$embedded_asset"
run_chatto "$test_directory/restored.log"
cmp "$test_directory/asset-backup" "$embedded_asset"
run_chatto "$test_directory/after-restore.log"
assert_skipped "$test_directory/after-restore.log"

# Environment inputs must invalidate the task without changes to source files.
CHATTO_BUILD_VERSION=0.0.0-mise-freshness-test run_chatto "$test_directory/version-change.log"
[[ "$(< apps/frontend/build/_app/version.json)" == '{"version":"0.0.0-mise-freshness-test"}' ]] || {
	echo 'Changed build version did not rebuild the frontend.' >&2
	exit 1
}
CHATTO_BUILD_VERSION=0.0.0-mise-freshness-test run_chatto "$test_directory/version-unchanged.log"
assert_skipped "$test_directory/version-unchanged.log"
run_chatto "$test_directory/default-version.log"
run_chatto "$test_directory/default-version-unchanged.log"
assert_skipped "$test_directory/default-version-unchanged.log"

echo 'Chatto build freshness checks passed.'
