#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

if [[ "$#" -ne 2 ]]; then
	echo "usage: $0 <frontend-build-directory> <target-.client-directory>" >&2
	exit 2
fi

source_directory="$(cd "$1" && pwd -P)"
target_parent="$(cd "$(dirname "$2")" && pwd -P)"
target_directory="$target_parent/$(basename "$2")"

if [[ "$(basename "$target_directory")" != ".client" ]]; then
	echo "refusing to replace non-.client target: $target_directory" >&2
	exit 2
fi
if [[ ! -f "$source_directory/200.html" && ! -f "$source_directory/200.html.gz" ]]; then
	echo "frontend build is missing 200.html or 200.html.gz: $source_directory" >&2
	exit 2
fi
if [[ "$source_directory" == "$target_directory" ]]; then
	echo "frontend source and target directories must differ" >&2
	exit 2
fi

rm -rf "$target_directory"
cp -R "$source_directory" "$target_directory"

# SvelteKit's precompress option emits raw, Brotli, and gzip representations.
# Keep both encoded forms for direct negotiation, but use gzip as the canonical
# identity fallback so the Go binary does not embed the large raw copy too.
while IFS= read -r -d '' gzip_file; do
	raw_file="${gzip_file%.gz}"
	if [[ -f "$raw_file" ]]; then
		rm "$raw_file"
	fi
done < <(find "$target_directory" -type f -name '*.gz' -print0)

printf '\n' > "$target_directory/.gitkeep"
