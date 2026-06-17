---
name: "chatto-release-notes"
description: "Create or update Chatto release notes, including existing GitHub releases, by comparing tags, commits, and PRs"
---

Generate or update user-facing release notes for a Chatto release.

## Workflow

- Identify the release tag. If the user did not provide one, find the newest git tag and the previous tag.
- Inspect the existing GitHub release body with `gh release view <tag> --json body,url --jq ...`.
- Inspect the commits and PRs between the previous tag and the release tag. Use PR bodies when available; they usually explain user impact better than commit titles.
- Write the updated release notes to `.context/release-<version>.md`, where `<version>` is the release tag.
- Update the existing GitHub release with `gh release edit <tag> --notes-file .context/release-<version>.md`.
- Verify the stored release body afterward with `gh release view <tag> --json body --jq .body`. If useful, diff it against the local `.context` file.

## Format

- Keep any generated changelog that is already present in the release body intact. Add the human-facing release description above it.
- Start with a short one-paragraph summary of the release.
- Keep phrasing tight. Avoid explaining why an obvious user-facing improvement is useful when the title and direct description already make it clear.
- Group noteworthy human-written changes under concise `###` headings that make sense for the release, such as "Highlights", "Sign-in and Setup", "Permissions and Administration", "Calls and Realtime Reliability", or "Chat Polish".
- Group API changes under a dedicated `## API Changes` section, using plain bullets without bold summaries.
- Use `## Upgrade Notes ⚠️` when breaking changes or upgrade work affect server operators, admins, or client developers.
- Use `- **Bold Summary**: explanatory text` only for notable new features and changes.
- Bug fixes should always be written in the format `- Fixed an issue [description of issue]`, with no bold summary.
- Keep wording end-user focused. Describe what people can do now, what behaves better, or what operators need to know.
- Mention breaking changes and upgrade work in a dedicated upgrade section when they affect server operators, admins, or client developers.
- Do not summarize internal refactoring unless it has direct user, operator, or client-developer impact.
- Keep it concise and focused. Avoid emojis except the warning marker in `## Upgrade Notes ⚠️`.

## Fallback

If the user asks for a standalone announcement instead of updating a GitHub release, write the Markdown announcement to `.context/release-<version>.md` and tell the user where it is.
