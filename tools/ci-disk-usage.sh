#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Report storage use without changing caches or masking the original job result.
set -u

echo 'Filesystem capacity'
df -h
echo 'Filesystem inodes'
df -i
echo 'Cache and workspace sizes'
for directory in \
  "$HOME/go/pkg/mod" \
  "$HOME/.cache/go-build" \
  "$HOME/.local/share/mise" \
  "${GITHUB_WORKSPACE:-$PWD}" \
  /ms-playwright; do
  if [ -d "$directory" ]; then
    du -sh "$directory"
  fi
done

if command -v pnpm >/dev/null 2>&1; then
  store=$(pnpm store path --silent)
  if [ -d "$store" ]; then
    du -sh "$store"
  fi
fi

# Diagnostics must not turn an otherwise successful job into a failure.
exit 0
