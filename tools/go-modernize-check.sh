#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Reports Go modernize findings for the Go module in the current directory.
# Each argument is one build-tag set to check, for example "" for the default
# build and "bootstrap,test_endpoints" for tagged files. Without arguments, the
# script checks the default build only.
#
# Generated protobuf files (*.pb.go) are skipped: protoc-gen-go owns them.
# To fix findings, run the modernize command printed below with -fix.

set -euo pipefail

if [ "$#" -eq 0 ]; then
	set -- ""
fi

found=0
for tags in "$@"; do
	status=0
	output="$(modernize -tags="$tags" ./... 2>&1)" || status=$?
	# modernize exits with 3 when it reports diagnostics. Any other non-zero
	# status means that it could not analyze the module.
	if [ "$status" -ne 0 ] && [ "$status" -ne 3 ]; then
		printf '%s\n' "$output" >&2
		exit "$status"
	fi
	findings="$(printf '%s\n' "$output" | grep -v '\.pb\.go:' | grep -v '^$' || true)"
	if [ -n "$findings" ]; then
		printf '%s\n' "$findings" >&2
		printf 'Fix with: modernize -tags=%q -fix ./...\n' "$tags" >&2
		found=1
	fi
done

exit "$found"
