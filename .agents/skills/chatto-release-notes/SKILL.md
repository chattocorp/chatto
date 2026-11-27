---
name: chatto-release-notes
description: Prepare and verify Chatto release pages from Sweet Release notes.
---

# Chatto Release Notes

From 0.6.0 onward, the docs site generates one page per numbered Chatto release.
Pending notes with a planned version create a preview page. The stable release uses the same URL.
The changelog index, sidebar, and homepage link update at build time.

## Review the release

1. Read the root `.sweet.yml`, pending `.sweet/` notes, `CHANGELOG.md`, and
   the Sweet release PR if available. Confirm the target version.
2. Compare the release with the previous stable version. Review the full
   prerelease cycle, including relevant fixes since the last beta.
3. Check that the highlights explain the most important changes for members,
   operators, and integration authors. Add missing highlights as `highlight`
   notes. Keep each one to one or two sentences.
4. Check for required actions before upgrade, during deployment, and before
   rollback. Write each action as an `upgrade` note. State who must act and
   when. Link to the operator or API guide for detailed steps.
5. Follow [Writing change notes](../../../CONTRIBUTING.md#writing-change-notes)
   for other entries and the required release type.

## Verify the page

Run `mise x -- pnpm --filter docs-website build`. Check the release page
through the homepage and `/changelog/`. Confirm the version, highlights,
upgrade actions, and links. A prerelease page includes the cycle’s notes.
A stable page uses Sweet’s combined entry and links to the GitHub release.

The manual pages through 0.5.0 keep their published content. Follow
[the docs website instructions](../../../apps/docs-website/AGENTS.md) when a
maintainer requests a correction to one of these pages. For release readiness,
use [chatto-release-checklist](../chatto-release-checklist/SKILL.md).
