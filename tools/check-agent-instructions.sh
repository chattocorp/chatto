#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repository_root"

failed=false

while IFS= read -r instruction_file; do
	if [[ "$instruction_file" != "AGENTS.md" ]] &&
		! rg --fixed-strings --quiet "$instruction_file" AGENTS.md; then
		echo "AGENTS.md does not route to $instruction_file" >&2
		failed=true
	fi
done < <(rg --files -g 'AGENTS.md' | sort)

link_output="$(
	rg --with-filename --only-matching \
		'\[[^]]+\]\([^()]+\)' \
		$(rg --files -g 'AGENTS.md')
)"

while IFS=: read -r source_file markdown_link; do
	target="${markdown_link#*(}"
	target="${target%)}"
	target="${target%%#*}"
	if [[ "$target" == http://* || "$target" == https://* || -z "$target" ]]; then
		continue
	fi
	resolved_target="$(dirname "$source_file")/$target"
	if [[ ! -e "$resolved_target" ]]; then
		echo "$source_file links to missing path: $target" >&2
		failed=true
	fi
done <<<"$link_output"

while IFS= read -r misplaced_skills; do
	echo "repository-local skills must live in .agents/skills: $misplaced_skills" >&2
	failed=true
done < <(find . -type d -path '*/.agents/skills' ! -path './.agents/skills')

if [[ "$failed" == true ]]; then
	exit 1
fi

echo "Agent instruction checks passed."
