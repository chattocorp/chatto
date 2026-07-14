---
name: "chatto-release-checklist"
description: "Run before making a new Chatto release. Compares the candidate version with stable and prerelease baselines, writes a developer checklist and separate user-facing announcement to .context/, and reports API changes separately."
---

# Chatto Release Checklist

Use this skill when preparing, reviewing, or announcing a new Chatto release.

This is a pre-release review workflow. Do not create tags, edit GitHub releases, push, or publish artifacts unless the user explicitly asks for that after seeing the checklist.

## Inputs

- Candidate version: prefer the version named by the user. Otherwise query release-please first:
  - Look for an open release-please PR targeting the relevant branch and use the version in its title, branch, body, or changed version files.
  - Read `.release-please-config.json` and `.release-please-manifest.json` to understand the current release-please version, release type, prerelease settings, tag format, changelog path, and extra version files.
  - If no release-please PR exists, infer the next version from the manifest/config and Conventional Commit messages since the last release tag. State that the version is inferred, not release-please-confirmed.
  - Only then fall back to `cli/version.go`, `frontend/package.json`, or the current git tag.
- Candidate ref: prefer the ref named by the user. Otherwise use the open release-please PR head ref when one exists; if there is no release-please PR, use `HEAD` on the target branch/release candidate branch.
- Stable baseline: use the newest stable tag matching `v[0-9]*.[0-9]*.[0-9]*` with no prerelease suffix. Ignore tags like `v0.4.0-beta.2` for this baseline.
- Prerelease baseline: when the candidate is a prerelease, also find the newest earlier prerelease tag in the same release line, such as `v0.4.0-beta.2` for `0.4.0-beta.3`. Use this for beta-user and API compatibility notes.

Useful commands:

```sh
gh pr list --state open --json number,title,headRefName,baseRefName,url,body,labels,author \
  --jq '.[] | select((.title | test("release"; "i")) or (.headRefName | test("release-please|autorelease"; "i")) or ([.labels[].name] | any(test("release-please|autorelease"; "i"))))'
gh pr view <release-pr-number> --json number,title,url,body,headRefName,baseRefName,files
jq . .release-please-config.json
jq . .release-please-manifest.json
git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -Ev -- '-' | head -1
git tag --list 'v<major>.<minor>.<patch>-*' --sort=-v:refname | head -5
git ls-remote origin <candidate-ref-or-pr-head>
git rev-parse <candidate-ref>
git log --oneline <baseline>..<candidate-ref>
git log --first-parent --merges --pretty=format:'%h %s' <baseline>..<candidate-ref>
git diff --stat <baseline>..<candidate-ref>
git diff --name-status <baseline>..<candidate-ref> -- proto/chatto/api/v1 proto/chatto/core/v1 cli/internal/connectapi cli/internal/http_server/server_info.go cli/internal/http_server/realtime.go apps/frontend/src/lib/api/server.ts
gh pr view <number> --json number,title,url,body,mergedAt
```

If the candidate version already has a tag, compare `<baseline>..<candidate-tag>`. If an open release-please PR exists, compare `<baseline>..<release-pr-head-ref>`. Otherwise compare `<baseline>..HEAD` and clearly say the report is for the current release candidate state.

## Workflow

1. Confirm the range:
   - Identify candidate version, candidate ref, stable baseline tag, prerelease baseline tag when applicable, and whether the candidate is stable or prerelease.
   - Record the source of the candidate version: user-provided, open release-please PR, release-please config/manifest inference, version file fallback, or tag fallback.
   - When using release-please inference, respect `.release-please-config.json` settings such as `release-type`, `versioning`, `prerelease`, `prerelease-type`, `include-v-in-tag`, and `include-component-in-tag`.
   - Record source freshness: release-please PR number, base branch, head ref, head SHA, fetch time/date, and whether the local reviewed ref matches the remote PR head SHA.
   - Note if the candidate version is older than, equal to, or newer than the stable baseline version.
   - If the candidate is a stable release and a newer stable tag already exists, stop and ask the user how to proceed.
2. Gather release context:
   - Inspect commits and merged PRs between the stable baseline and candidate. When a prerelease baseline exists, also inspect the narrower prerelease-baseline-to-candidate range for beta-specific compatibility notes.
   - Read `CHANGELOG.md` for generated release-please content in the candidate range.
   - Check relevant FDRs/ADRs when a change is user-facing or architectural enough that the announcement would otherwise be guesswork.
   - Build an evidence map that links major announcement bullets to PRs, commits, or docs. Keep this map in the developer checklist, not the announcement.
3. Review API changes separately:
   - Public ConnectRPC/protobuf API: inspect diffs under `proto/chatto/api/v1/`, generated frontend files under `apps/frontend/src/lib/pb/chatto/api/v1/`, generated Go files under `cli/internal/pb/chatto/api/v1/`, and generated docs under `apps/docs-website/src/content/docs/reference/connectrpc-api/`.
   - Persisted protobuf/event shapes: inspect `proto/chatto/core/v1/` and call out higher-risk persisted EVT/RUNTIME_STATE compatibility changes. When these files change, read `proto/AGENTS.md` and use `chatto-event-sourcing` guidance. Check removed fields, reused tags/oneof numbers, reserved or retired tags, replay compatibility, and old self-hosted event streams.
   - Realtime websocket API: inspect `proto/chatto/api/v1/realtime.proto`, `cli/internal/http_server/realtime.go`, `cli/internal/core/my_events_model.go`, and frontend event-bus/client code under `apps/frontend/src/lib/state/server/`.
   - Retired legacy API compatibility: only call this out when a release removes, reintroduces, or changes compatibility behavior for retired public API clients. The current public API is ConnectRPC plus `/api/realtime`.
   - Server discovery and stable HTTP surfaces: inspect `GET /api/server` implementation in `cli/internal/http_server/server_info.go`, matching tests, and frontend client code in `apps/frontend/src/lib/api/server.ts`.
   - Other client-visible HTTP endpoints: call out auth, upload, asset, webhook, health, metrics, or CORS behavior changes when present.
4. Classify API changes:
   - Breaking: removed fields/services/endpoints, renamed fields, changed required fields, changed enum/string meanings, tightened validation, changed auth/CORS requirements, incompatible persisted protobuf changes, or behavior that can strand older clients.
   - Additive: new optional fields, new services/methods/endpoints, new enum values clients can ignore, expanded response metadata, or backward-compatible docs/codegen refreshes.
   - Internal-only: generated code churn or resolver refactors with no external behavior change.
5. Classify announcement items:
   - `feat` commits and new user/operator/client capabilities go under `New Features`.
   - `fix` commits go under `Bug Fixes`, using `Fixed an issue ...` phrasing.
   - `perf`, user-visible refactors, UI polish, docs/setup improvements, and behavior changes go under `Changes`.
   - Deployment, config, storage, backup, image, metrics, security, and operational notes go under `Notes for Self-Hosters`.
   - Public proto, ConnectRPC, realtime websocket, retired legacy API compatibility, `/api/server`, CORS, auth, upload, asset, webhook, health, or metrics compatibility notes go under `Notes for API Users`.
6. Write two files:
   - Developer checklist: `.context/release-checklist-<version>.md`
   - Publishable announcement: `.context/release-announcement-<version>.md`

The announcement file must speak only to users, self-hosters, admins, and client developers. Do not include maintainer-only readiness status, blockers, source freshness, commands, PR evidence, uncertainty, or internal review notes in the announcement. If the developer checklist has blockers, still write the announcement as a draft but mark only the developer checklist as not ready.

## Developer Checklist Format

Use this Markdown structure:

```md
# Chatto <version> Release Checklist

Compared stable baseline `<stable-baseline>` to `<candidate-ref>` on <YYYY-MM-DD>.
Prerelease baseline: `<prerelease-baseline-or-none>`.

## Release Readiness

- Status: <Ready | Needs review | Blocked>
- Release blockers: <None | concise blocker list>
- Manual checks: <concise manual verification list>
- Recommendation: <publish / resolve blockers first / review announcement only>

## Source Freshness

- Candidate version source: `<source>`
- Release-please PR: <number/url or none>
- Release-please base/head: `<base>` / `<head>`
- Reviewed SHA: `<sha>`
- Remote head SHA: `<sha>`
- Freshness result: <matches remote head | stale | not checked, with reason>

## Compatibility Matrix

- Stable self-hosters upgrading from `<stable-baseline>`: <impact>
- Beta users upgrading from `<prerelease-baseline>`: <impact or not applicable>
- Retired legacy API clients: <impact>
- ConnectRPC clients: <impact>
- Realtime websocket clients: <impact>
- Operators using Docker Compose: <impact>
- Operators using clustered replicas: <impact>

## Generated Output Status

- Public protobuf source changed: <yes/no>
- Generated Go protobuf/Connect files changed: <yes/no/not applicable>
- Generated TypeScript protobuf/Connect files changed: <yes/no/not applicable>
- Generated ConnectRPC docs changed: <yes/no/not applicable>
- Retired legacy API compatibility changed: <yes/no/not applicable>
- Codegen/drift check present: <yes/no/not applicable>

## Publishable Announcement File

- Path: `.context/release-announcement-<version>.md`
- Status: <draft | reviewed>

## Announcement Evidence

- <Announcement bullet or theme>: <PRs/commits/docs that support it>

## API Changes

### ConnectRPC and Public Protobufs

- <Breaking/Additive/Internal-only classification plus impact>

### Persisted Protobufs and Event Streams

- <Compatibility notes for durable event/runtime state schemas>

### Realtime WebSocket

- <Breaking/Additive/Internal-only classification plus impact>

### Retired Legacy API Compatibility

- <Compatibility notes if retired legacy API behavior changed; otherwise state no change/not applicable>

### Server Discovery and HTTP Compatibility

- <Notes for /api/server, auth, upload, asset, webhook, health, metrics, or CORS changes>

## Diff Sources

- Stable baseline tag: `<stable-baseline>`
- Prerelease baseline tag: `<prerelease-baseline-or-none>`
- Candidate version/ref: `<version>` / `<candidate-ref>`
- Candidate version source: `<source>`
- Commits reviewed: `<count>`
- PRs reviewed: <links or numbers when available>
- Commands: `<important commands used>`

## Follow-Up Checklist

- [ ] Release blockers are resolved or explicitly accepted.
- [ ] Breaking or upgrade notes are reflected in release notes and PR title/body when needed.
- [ ] Public protobuf changes have generated outputs and docs.
- [ ] API documentation is current when ConnectRPC, realtime websocket, or retired legacy API compatibility behavior changed.
- [ ] Announcement wording has been reviewed by a human before publishing.
```

## Announcement Format

Write this structure to `.context/release-announcement-<version>.md`:

```md
# Chatto <version>

<A short human-facing opening paragraph suitable for users, admins, self-hosters, and client developers. Keep it concise and concrete. Qualify "new" or "first" claims as "since <stable-baseline>" or "for beta users" when needed.>

### New Features

- <New capability or workflow. Say "None." if there are no user-facing features.>

### Changes

- <Changed behavior, polish, performance, docs, or operational improvements. Say "None." if not applicable.>

### Bug Fixes

- <Fixed issue. Use "Fixed an issue ..." phrasing. Say "None." if not applicable.>

### Notes for Self-Hosters

- <Deployment, config, migration, image, storage, backup, security, or operational notes. Say "None identified." if not applicable.>

### Notes for API Users

- <Client-developer-facing API changes, compatibility hazards, or migration notes. Say "None identified." if not applicable.>
```

Keep announcement wording user-facing. Avoid maintainer-only phrases like "confirm this before publishing", "drift check", "PR #...", "candidate ref", "blocked", or "internal-only" in the announcement file.

## Output

- Always tell the user the exact checklist and announcement paths.
- Summarize the candidate version, stable baseline tag, prerelease baseline tag when applicable, readiness status, and API-change classification in the final response.
- If no API changes are found, say so explicitly and mention which surfaces were checked.
- If the user later wants to update a GitHub release body, use the `chatto-release-notes` skill after this checklist is reviewed.
