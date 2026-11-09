#!/usr/bin/env bash
# Tag every release in a verified push. The first Sweet manifest is a baseline.
set -euo pipefail

base="${PUSH_BEFORE:?}"
head="${PUSH_AFTER:?}"
if [[ "$base" =~ ^0+$ ]]; then
  range="$head"
else
  git merge-base --is-ancestor "$base" "$head"
  range="$base..$head"
fi

if ! git cat-file -e "$base:.sweet.yml" 2>/dev/null; then
  # A migration push can also contain later releases. Start after the first
  # manifest, rather than skipping the whole push or retagging version 0.5.0.
  base=""
  while IFS= read -r commit; do
    if git cat-file -e "$commit:.sweet.yml" 2>/dev/null; then
      base="$commit"
      break
    fi
  done < <(git rev-list --reverse --first-parent "$range")
  [[ -n "$base" ]] || exit 0
fi

git config user.name github-actions[bot]
git config user.email 41898282+github-actions[bot]@users.noreply.github.com
mise x -- sweet git tag --base "$base" --head "$head" --json > "$RUNNER_TEMP/tags.json"
while IFS= read -r tag; do
  git push origin "refs/tags/$tag"
done < <(jq -r '.[].name' "$RUNNER_TEMP/tags.json")
