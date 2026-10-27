---
name: chatto-release-announcement
description: Draft a concise Chatto release announcement in Markdown from a release PR, tag, or version. Use for announcement posts, including beta releases; use chatto-release-notes for stable release pages on the docs website.
---

# Chatto Release Announcement

Write a short announcement for Chatto users and operators. Return Markdown
in the conversation unless the user requests a file or another destination.
Drafting an announcement does not include publishing it.

## Output

Return only the release announcement. Do not add a preface, research summary,
source list, completion message, or follow-up offer. Do not send progress
messages unless higher-priority instructions require them.

Use plain Markdown, or wrap the complete announcement in one `markdown` code
block for easy copying. When using a code block, put no text outside it.

## Establish The Release Scope

- Use the release PR, tag, or version supplied by the user. Preserve the full
  version, including its beta suffix.
- For a beta announcement, compare with the preceding beta. For a stable
  announcement, compare with the preceding stable release, unless the user
  specifies another range.
- Read the release PR and changelog to identify candidate changes. Inspect
  the underlying feature and fix PRs or commits for details. A release-please
  PR normally contains version and changelog edits, not the implementation.
- Check source code or product documentation when the reader impact, platform
  scope, compatibility, or required action is unclear. Do not infer these
  details from commit titles alone.
- Include only changes in the target release. Distinguish shipped behavior
  from proposals, incomplete work, and follow-up tasks.

## Select And Write

- Include only the most noteworthy changes: useful new behavior, significant
  fixes, operational improvements, and required upgrade actions. This is not
  a complete changelog.
- Use focused, pragmatic wording and ASD-STE100 Simplified Technical English
  where applicable. Use short sentences, active voice, and concrete terms.
- State what changes for the reader and why it matters. Keep each bullet to
  one or two short sentences. Use normal text lines or plain bullets. Do not
  use bold titles or a "Title: explanation" format, except for the required
  `**Breaking Change:**` marker.
- Omit marketing language, implementation inventories, test reports, internal
  refactors, and routine dependency or CI changes.
- Do not include PR or issue links or numbers in the announcement. Keep the
  research evidence separate from the post.
- Preserve material limits. Do not describe a restricted importer as a complete
  migration solution, a platform-specific fix as universal, or bounded upload
  retries as recovery across application restarts.
- Omit routine replica rollout warnings, including instructions to shut down
  old replicas first or upgrade all replicas before using a feature.

## Structure

Use a release title and, when useful, one short introductory sentence. Group
the selected changes in this order:

1. **Desktop app**: installation, updates, native integration, and desktop-only
   fixes. Name the affected operating system.
2. **For users**: chat, rooms, notifications, attachments, and other shared
   client behavior.
3. **For robots**: bots, automation, MCP tools, and integration APIs.
4. **For administrators**: account and room management, permissions, moderation,
   and authentication policy.
5. **For operators**: deployment, resource use, server configuration, backups,
   and migration tools.

Use Markdown level-two headings for these sections, in the order above.
Omit empty sections.

List each breaking change in its corresponding section, with the exact prefix
`**Breaking Change:**`. Do not add a separate breaking-changes section. Cover
incompatible behavior, removed interfaces, changed requirements, and necessary
migration steps. State who is affected and what they must do. Make actions
required before an upgrade clear.

Check for breaking changes even when commit titles have no breaking-change
marker. Do not label an additive feature or optional setting as breaking.
Keep non-breaking rollout requirements with the relevant operator bullet.
Do not repeat the same change across sections. Keep required migration details
with the change they concern.

## Final Check

Verify the release range, every included claim, and all required actions
against the inspected evidence. Remove minor items and repeated explanations.
Do not claim that a release is available unless its publication is confirmed.
