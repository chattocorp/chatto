#!/usr/bin/env bash
# Describe one existing release tag in the shape of Sweet's tag.yml outputs, so
# a manual CI run can publish that tag again.
#
# Environment:
# - TAG: the tag to publish, such as v0.6.0-beta.4. Untrusted input.
# - REF: the ref of the run. Only main and release-[0-9]* branches can publish.
# - GITHUB_OUTPUT: receives `tags` (a JSON array with one entry) and `packages`
#   (a JSON object from the package path to that entry).
#
# Each entry is {package, version, tag, commit, prerelease, highest}, with the
# same rules as Sweet's tag-releases.sh. `highest` is true when the version is
# the highest stable version among the package's tags on origin.
#
# Needs `git` with an `origin` remote and full history, `sweet`, and `jq`.
set -euo pipefail

fail() {
  echo "::error::$1"
  exit 1
}

tag="${TAG:-}"
ref="${REF:-}"

if [[ "$ref" != refs/heads/main && ! "$ref" =~ ^refs/heads/release-[0-9] ]]; then
  fail "Run this from main or a release-[0-9]* branch, not ${ref}."
fi
# Allow only the characters of Sweet tag names. This also rejects whitespace,
# shell and Git revision syntax, and option-like values.
if [[ ! "$tag" =~ ^[A-Za-z0-9][A-Za-z0-9._/+-]{0,127}$ || "$tag" == *..* || "$tag" == */ ]]; then
  fail "The tag must be a Sweet release tag, such as v0.6.0-beta.4."
fi

# SemVer 2.0.0. Sweet versions can have a prerelease and build metadata.
number='(0|[1-9][0-9]*)'
identifier='[0-9A-Za-z-]+'
semver="^${number}\.${number}\.${number}(-${identifier}(\.${identifier})*)?(\+${identifier}(\.${identifier})*)?$"

# Select the one package whose tag prefix starts the tag, followed by a version.
matches="$(sweet status --json | jq -c --arg tag "$tag" '
  [.[] | (.tagPrefix // "") as $prefix
       | select(($prefix | length) > 0 and ($tag | startswith($prefix)))
       | {package: .path, prefix: $prefix, version: ($tag[($prefix | length):])}]')"
matches="$(jq -c --arg semver "$semver" '[.[] | select(.version | test($semver))]' <<< "$matches")"
case "$(jq length <<< "$matches")" in
  1) ;;
  0) fail "${tag} does not match the tag prefix and version of a Sweet package." ;;
  *) fail "${tag} matches more than one Sweet package." ;;
esac
package="$(jq -r '.[0].package' <<< "$matches")"
prefix="$(jq -r '.[0].prefix' <<< "$matches")"
version="$(jq -r '.[0].version' <<< "$matches")"

# Read the tag from origin. An annotated tag lists its commit as `<tag>^{}`.
remote="$(git ls-remote --tags origin "refs/tags/${tag}" "refs/tags/${tag}^{}")"
commit="$(awk -v ref="refs/tags/${tag}^{}" '$2 == ref { print $1 }' <<< "$remote")"
if [[ -z "$commit" ]]; then
  commit="$(awk -v ref="refs/tags/${tag}" '$2 == ref { print $1 }' <<< "$remote")"
fi
[[ -n "$commit" ]] || fail "${tag} does not exist on origin."

# Publish only commits of the release branches, as the tag job does.
git fetch --quiet origin '+refs/heads/main:refs/remotes/origin/main' '+refs/heads/release-*:refs/remotes/origin/release-*'
git cat-file -e "${commit}^{commit}" 2> /dev/null || fail "The commit of ${tag} is not in the history of origin."
trusted=false
while IFS= read -r branch; do
  if git merge-base --is-ancestor "$commit" "$branch"; then
    trusted=true
    break
  fi
done < <(git for-each-ref --format='%(refname)' refs/remotes/origin/main 'refs/remotes/origin/release-[0-9]*')
[[ "$trusted" == true ]] || fail "${tag} is not on origin/main or an origin/release-[0-9]* branch."

# The package's tags on origin, for the highest stable version.
versions="$(git ls-remote --tags --refs origin "refs/tags/${prefix}*" |
  awk -v prefix="refs/tags/${prefix}" 'index($2, prefix) == 1 { print substr($2, length(prefix) + 1) }')"

entry="$(jq -cn --arg package "$package" --arg version "$version" --arg tag "$tag" \
  --arg commit "$commit" --arg versions "$versions" '
  def stable: capture("^(?<a>0|[1-9][0-9]*)\\.(?<b>0|[1-9][0-9]*)\\.(?<c>0|[1-9][0-9]*)(\\+[0-9A-Za-z.-]+)?$")
    | [.a, .b, .c | tonumber];
  ([$versions | split("\n")[] | select(length > 0) | stable] | max) as $highest |
  {package: $package, version: $version, tag: $tag, commit: $commit,
   prerelease: ($version | split("+")[0] | contains("-")),
   highest: ([$version | stable][0] | . != null and . == $highest)}')"

{
  echo "tags=$(jq -c '[.]' <<< "$entry")"
  echo "packages=$(jq -c '{(.package): .}' <<< "$entry")"
} >> "$GITHUB_OUTPUT"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "## Release tag to publish again"
    echo
    echo "- Tag: \`${tag}\`"
    echo "- Package: \`${package}\`"
    echo "- Commit: \`${commit}\`"
    echo "- Prerelease: $(jq -r .prerelease <<< "$entry"), highest stable: $(jq -r .highest <<< "$entry")"
  } >> "$GITHUB_STEP_SUMMARY"
fi
