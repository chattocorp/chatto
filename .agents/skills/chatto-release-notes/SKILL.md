---
name: chatto-release-notes
description: Create or update Chatto release pages from release-please state, tags, changelog entries, and PRs.
---

# Chatto Release Notes

Write one docs-website MDX page per stable minor release. Follow
`apps/docs-website/AGENTS.md` for public documentation style and terminology.
Paths below are relative to the repository root.

## Release And Evidence

- Read `.release-please-config.json`, `.release-please-manifest.json`, and the
  release-please PR, if present. Target the next stable minor release unless
  the user specifies another release.
- Remove prerelease suffixes and normalize the page version to `x.y.0`.
  For example, `0.5.0-beta.4` belongs on the `0.5.0` page.
- Compare with the highest stable patch tag of the previous minor release.
  Include the complete prerelease cycle, not only changes since the last beta.
- Check commits, PR descriptions, and all relevant `CHANGELOG.md` sections.
  Include later `fix:` commits that are not yet in the changelog. Check product
  docs and code when the effect on readers is unclear.
- Stop and explain if the target release cannot be determined.

## Existing Text

The page path is
`apps/docs-website/src/content/docs/releases/<version-with-hyphens>.mdx`.
Use hyphens in filenames and dotted versions in visible text.

Treat existing pages as manually edited. Preserve their wording, order, and
unrelated content. Make targeted edits when the user requests an update.
Without that request, put proposed changes in
`.context/release-page-<version>-proposal.md`. Restructure an existing page only
when the user requests it.

## Content

- Write for users, operators, admins, and integration authors. Describe what
  changes for them. Omit internal refactors, test work, CI, and generated-file
  changes unless they change visible behavior or require reader action.
- Rank features by their effect on readers. Put the largest features first.
  Describe each new feature once, in its final stable form. Implementation
  size and breaking-change labels do not determine headline placement.
- Put compatibility requirements and migration actions in `Upgrade Notes`.
  Transport, storage, media-format, and cache changes are not feature cards
  unless product documentation or the maintainer identifies a separately
  intended user capability. Do not invent a feature from an incidental effect.
- A public integration API replacement can be a headline feature when it
  defines the release. Name the replacement and repeat the required action
  in upgrade notes. Link prerelease testers to the stable API reference;
  do not list each intermediate method or message change.
- Include every bug fix relevant to readers of the previous stable release.
  Omit fixes to behavior introduced and repaired during the prerelease cycle.
  Put migration actions for prerelease testers in upgrade notes.
- Use short, factual sentences and concrete subjects. Avoid marketing claims,
  descriptions of the page itself, emojis, PR lists, and copied changelog text.
- Describe features at the product level. Do not list every affected component,
  screen, or RPC. Keep each card understandable on its own.
- For performance changes, name the visible benefit. Put startup, backup,
  restore, and resource-use improvements with operator content. Put noticeable
  frontend loading improvements with user content.

## Page And Components

Read the components in `apps/docs-website/src/components/release-notes/`
before use. Add missing components if the requested page requires them.
Import them from `../../../components/release-notes/` in the MDX page.

Use this page order:

1. `ReleaseHero`. Until the stable release exists, use `(unreleased)` in the
   frontmatter title, an unreleased description, and `status="Unreleased"` on
   the hero. If readers must act before they upgrade, say so in the summary.
2. Audience sections, in this order: `## Using Chatto` for members,
   `## Running Chatto` for operators and administrators, and
   `## Integrating with Chatto` for bots, API clients, and other integrations.
   Omit a section that has no content. Each section has these parts in this
   order, and omits a part that has no content:
   1. A `ReleaseFeatureGrid` with cards: `size="large"` headline cards first,
      then default cards, then `size="small"` cards.
   2. `### Other noteworthy changes`: a bullet list of smaller changes that do
      not need a card. Write each bullet as one or two short sentences.
   3. `### Fixes`: a bullet list of fixes for this audience.
3. `## Upgrade Notes`, when readers must act or check compatibility.
4. `## GitHub release`.

Cards:

- Use `ReleaseFeatureGrid` with `ReleaseFeatureCard` as direct children.
  This structure supports native Grid Lanes and its polyfill.
- Use one card per feature. Use `size="large"` for headlines, `size="small"`
  for compact items, and the default size otherwise. Keep each card to one
  short paragraph: usually one sentence for small cards and at most two for
  other cards. Move extra detail to upgrade notes, grouped fixes, or the GitHub
  release. Do not add audience labels or subheadings inside cards.

Other sections:

- Put upgrade notes in plain text, not in cards or callout boxes. Put the
  actions in the order that readers must do them, and make the most critical
  action first and prominent. When the list is long, group it in `###`
  subsections, for example permissions, upgrade procedure, configuration,
  what members notice, and custom clients and bots. Link to guides for
  details. Do not list individual API changes; link to the API compatibility
  guide.
- Put each bug fix in the `### Fixes` part of the section for the audience it
  affects. Do not put fixes in feature cards. Omit fixes that already shipped
  in a patch release of the previous stable minor release.
- End with `GitHub release` and a link to
  `https://github.com/chattocorp/chatto/releases/tag/v<version>`.
  Keep this target for unreleased pages even before it exists.
- Do not force empty sections. Preserve existing sidebar order and labels when
  adding the page to `apps/docs-website/astro.config.mjs`.

## Images

Use images only when the maintainer supplies or identifies an asset, or
explicitly requests one. Do not generate screenshots or demo images by default.

Store release images in
`apps/docs-website/public/releases/<version-with-hyphens>/`.
Use `ReleaseImage` or the card's `imageSrc`, `imageAlt`, `imageCaption`, and
`imagePosition` props. Add useful alt text and a short factual caption. Place
an image beside the feature it shows.

## Verification

Run `mise x -- pnpm --filter docs-website build` after page edits. Check that
supplied images render. For material layout or style changes, check the page
at desktop and mobile widths. Report which checks ran.
