---
name: "chatto-release-notes"
description: "Create or update Chatto docs website release pages by comparing release-please state, tags, commits, changelog entries, and PRs"
---

Generate or update user-facing release pages for Chatto minor releases.

Release notes now live in the docs website, not in local `.context` Markdown
files or GitHub release bodies. The output is one MDX page per minor release,
for example:

```text
apps/docs-website/src/content/docs/releases/0-4-0.mdx
```

Use a URL-safe filename with hyphens (`0-4-0`) because Starlight content slugs
do not preserve dotted version filenames as sidebar slugs. Keep the visible
title, description, hero, and version text dotted (`0.4.0`).

## Audience

Write only for:

- Chatto users.
- Self-hosters, server operators, and admins.

Do not list changes that do not affect either group. Skip routine refactors,
test work, CI work, codegen churn, internal API rearrangement, and dependency
maintenance unless they change user-visible behavior or operator action.

Name the specific actor whenever one is known. Prefer "users", "members",
"operators", "admins", or "integration authors" over generic actor wording.

API changes belong on these pages only when they affect self-hosters or
integration authors running against their own Chatto servers. Phrase them as
operator or integration impact, not as maintainer implementation detail. Do not
mention generated package moves, import paths, or internal client extraction
unless external integration authors must take action.

Major public API replacements, removed protocols, or required integration
migrations are headline release-page items. Do not hide them behind generic
phrases such as "API foundation" or place them after smaller operational
improvements. Use an explicit title like `ConnectRPC replaces GraphQL`, mark the
card `size="large"`, and repeat the required operator action in plain
`## Upgrade Notes` text.

## Target Release

- Use release-please state as the source of truth:
  - `.release-please-config.json`
  - `.release-please-manifest.json`
  - the release-please PR, if present
- Create or update the page for the immediate next stable minor release being
  prepared.
- If release-please is currently configured for prereleases, do not create a
  prerelease page. Strip the prerelease suffix and target the stable base
  version instead. Example: `0.4.0-beta.4` targets `0.4.0`.
- Release pages are per minor release only. Normalize the page version to
  `x.y.0`.
- If the stable target release has not shipped yet, clearly mark the page as
  unreleased. Use `(unreleased)` in the frontmatter title, an unreleased
  description, and `status="Unreleased"` on `ReleaseHero`. Remove those markers
  only when the stable release exists.
- If the computed target is not clear, stop and explain the ambiguity before
  writing files.

Useful local check:

```sh
jq -r '.["."]' .release-please-manifest.json
```

## Existing Page Safety

Before writing, check whether the target page already exists:

```text
apps/docs-website/src/content/docs/releases/<version-with-hyphens>.mdx
```

If it exists, assume a human maintainer has edited it manually.

- Do not overwrite or restructure the existing page unless the user explicitly
  asked for that.
- Read the existing page and preserve its voice, order, and manual wording.
- If the user did not explicitly ask you to edit an existing page, write proposed
  additions or replacements to `.context/release-page-<version>-proposal.md`
  and tell the user where they are.
- If the user explicitly asked for an update, make the smallest targeted edits
  needed and preserve unrelated text.

## Required Components

Release pages should use the dedicated release-note components in:

```text
apps/docs-website/src/components/release-notes/
```

Expected components:

- `ReleaseHero.astro` for the page opening.
- `ReleaseFeatureGrid.astro` to group feature cards.
- `ReleaseFeatureCard.astro` for one user/operator-facing feature per card.
  Use `size="large"` for headline features, `size="small"` for compact items,
  and omit `size` for normal cards. Maintainer-provided images can be attached
  with `imageSrc`, `imageAlt`, `imageCaption`, and `imagePosition`.
  Do not add audience labels or subheadings inside cards; cards should show only
  the feature title and body text unless they include a maintainer-provided
  image.
- `ReleaseImage.astro` for standalone maintainer-provided images.

If these components are missing, add them before creating a release page.

`ReleaseFeatureGrid` uses CSS Grid Lanes for the box layout, with a local
polyfill fallback for browsers that do not support `display: grid-lanes` yet.
Keep release feature cards as direct children of `ReleaseFeatureGrid` so native
lanes and the polyfill can place them correctly.

## Research Workflow

### Comparison Scope

Release pages compare the upcoming stable minor release against the highest
stable patch release of the previous minor line.

- For a `0.4.0` page, compare against the highest stable `0.3.x` tag, such as
  `v0.3.8`.
- Do not use the latest prerelease tag, such as `v0.4.0-beta.4`, as the baseline
  for a stable minor release page.
- Include all user/operator/integration-facing changes that will land in
  `0.4.0`, including changes first released in `0.4.0-beta.*`.
- Ignore prerelease tags when choosing the baseline. Prerelease tags are useful
  evidence for grouping changelog sections, not the comparison start point.
- Treat prerelease-only refinements to a new system as part of that system, not
  as separate release-page features. For example, if a new ConnectRPC API lands
  during the `0.4.0` prerelease cycle and later prereleases clean up method
  names, message shapes, or generated docs before the stable release, describe
  the stable ConnectRPC API once. Mention beta-to-stable migration work only in
  upgrade notes for operators or integration authors who tested prereleases.

Identify the baseline by listing stable tags for the previous minor:

```sh
git tag --list 'v0.3.*' --sort=-version:refname | grep -v '-' | head -1
```

Then inspect changes from that baseline to the current release-preparation
state:

```sh
git log --oneline <previous-minor-highest-stable-tag>..HEAD
git diff --name-only <previous-minor-highest-stable-tag>..HEAD
```

- Inspect `CHANGELOG.md` for all stable and prerelease sections that roll into
  the target stable release.
- Inspect commits and PRs in the comparison range. Use PR bodies when available;
  they usually explain impact better than commit titles.
- Build a candidate bug-fix inventory from every `Bug Fixes` section in the
  comparison range, plus any post-prerelease `fix:` commits not yet in
  `CHANGELOG.md`. Then filter it for stable-release readers.
- Cross-check docs/FDRs/ADRs only when they clarify user behavior or operator
  implications.
- Filter aggressively for the release-page audience. Keep a scratch list of
  skipped internal changes if needed, but do not put it in the visible page.
- A prerelease-only bug in a feature that was introduced and fixed before the
  stable release should not be listed as a separate stable-release fix unless it
  changes the final user/operator behavior. Fold it into the feature wording or
  omit it.
- For newly introduced APIs or subsystems, do not list beta-to-beta hardening,
  validation, generated-doc coverage, method-shape fixes, auth plumbing, or
  error mapping as stable-release fixes. Those details are useful to maintainers
  and prerelease testers, but stable users only see the final API/subsystem.

Useful commands:

```sh
git tag --list --sort=-version:refname
gh pr view <number> --json title,body,url
```

## Page Structure

- Use concise frontmatter:

```mdx
---
title: Chatto <version> (unreleased)
description: Unreleased release notes for Chatto <version>.
---
```

For already shipped stable releases, remove `(unreleased)` from the title and
use `Release notes for Chatto <version>.` as the description.

- Import the release-note components from
  `../../../components/release-notes/...`.
- Start with `ReleaseHero`. For unreleased pages, pass `status="Unreleased"`.
- Decide the page grouping based on the release contents. Do not force a fixed
  set of sections when the release does not need them.
- Always list the biggest tentpole changes first, directly after the hero. This
  first section does not need a heading when the cards themselves make the
  release shape clear.
- Add a self-hosters/integrators section only when there are changes that affect
  server operators, admins, or integration authors. Use `## Running and
  Integrating Chatto` by default unless a more specific title fits the release
  better.
- Use one `ReleaseFeatureCard` per notable feature.
- Keep feature card body text concise and scannable. Use one short paragraph:
  usually one sentence for small cards, one or two sentences for normal cards,
  and at most two sentences for large headline cards. If a card needs more
  detail, move operator action to `## Upgrade Notes`, bug fixes to "Smaller
  fixes you'll appreciate", or exhaustive links/PR detail to `## GitHub release`.
- Do not put bug fixes in `ReleaseFeatureCard`, even if several fixes share a
  theme or feel user-visible. Bug fixes belong only in the grouped "Smaller
  fixes you'll appreciate" section.
- Do not create a second feature card for refinements, cleanup, renames, or
  shape changes made to a feature before its first stable release. Fold those
  details into the main feature card only when they affect the stable behavior,
  or into `## Upgrade Notes` when they affect prerelease testers.
- Do not generate screenshots or demo images. If the maintainer already provided
  an image or explicitly asks you to add one, place it near the card or section
  it supports with `ReleaseImage` or the image props on `ReleaseFeatureCard`.
- Add a plain `## Upgrade Notes` section only when server operators, admins, or
  integration authors need to act or review compatibility. Do not put upgrade
  notes in a box or card.
- Do not list every shape change for an API that is new in this stable release.
  If a public API replaced an older integration surface, name the replacement
  and required migration once, then point prerelease testers at the stable
  generated reference. Detailed method/message churn from beta releases does not
  belong on the release page.
- Add a "Smaller fixes you'll appreciate" section for stable-release
  user/operator/integration-impacting bug fixes. Group the fixes by
  functionality with concise `###` subheadings such as "Messages and Threads",
  "Notifications and Unread State", "Calls and Media", or "API and Operations".
  Do not use one ungrouped catch-all list.
- The fixes section must cover every bug fix relevant to readers upgrading from
  the previous stable release. Skip internal/CI/test fixes, prerelease-only
  repairs to unreleased behavior, generated-doc fixes, low-level API polish for
  an API introduced in this release, and implementation hardening that does not
  change the stable user/operator/integration outcome. Do not create an API
  subsection just to account for changelog entries that only mattered between
  betas.
- Keep bug-fix bullets concrete, but vary the phrasing in longer lists so the
  section reads naturally. Use `Fixed ...`, `Prevented ...`, `Kept ...`, or
  another direct verb when it is clearer than repeating `Fixed an issue where`.
- End the page with a `## GitHub release` section that links to the canonical
  GitHub release URL for the stable version. The URL is always
  `https://github.com/chattocorp/chatto/releases/tag/v<version>`, for example
  `https://github.com/chattocorp/chatto/releases/tag/v0.4.0`.
- Do not duplicate the full release-please changelog on the docs page. The
  GitHub release is the exhaustive source for PR and commit links.
- Avoid raw changelog dumps and PR-number lists in the human-written narrative
  sections.
- Avoid emojis in visible release pages.

## GitHub Release Link

Every release page must end with `## GitHub release`.

Use this format:

```mdx
## GitHub release

The generated GitHub release lists every PR and commit:
[github.com/chattocorp/chatto/releases/tag/v<version>](https://github.com/chattocorp/chatto/releases/tag/v<version>)
```

For unreleased pages, keep the same link target even if the GitHub release does
not exist yet. The page is already marked as unreleased, and the URL will become
valid when the stable release is published.

## Manual Images

Do not create or generate release-page images by default. Use images only when
the maintainer provides an asset, points you at an existing asset, or explicitly
asks you to add one.

Store release-specific images under:

```text
apps/docs-website/public/releases/<version-with-hyphens>/<descriptive-name>.png
```

For a standalone image, import `ReleaseImage` and reference it as:

```mdx
<ReleaseImage
  src="/releases/<version-with-hyphens>/<descriptive-name>.png"
  alt="..."
  caption="..."
/>
```

For an image attached to a feature card, use:

```mdx
<ReleaseFeatureCard
  title="..."
  imageSrc="/releases/<version-with-hyphens>/<descriptive-name>.png"
  imageAlt="..."
  imageCaption="..."
>
  ...
</ReleaseFeatureCard>
```

- Always provide useful alt text.
- Keep captions factual and short.
- Place the image near the feature it supports.
- The feature must be visible without explanation.
- Verify the image file renders in the docs page after the docs build.

## Sidebar

New pages usually need a sidebar entry in:

```text
apps/docs-website/astro.config.mjs
```

Add a `Releases` sidebar group if none exists. If a group exists, add only the
new page. Preserve existing order and labels.

## Wording Rules

- Lead with what changes for the user or operator.
- In visible section headings, speak to the reader directly. Avoid labels such
  as "users" when the page is addressing them; prefer headings like "Smaller
  fixes you'll appreciate".
- Do not write meta copy about the release page itself. Do not frame summaries
  as an explanation of what the page or release notes cover; describe the
  release directly instead.
- Avoid self-referential filler such as "Chatto adds", "Chatto moves", or
  "Chatto now". The page is already about Chatto. Prefer direct subject-first
  copy such as "Presence now shows up..." or "The public API has moved...".
- Use "server" or "Chatto server" for deployments.
- Use "self-hosters" sparingly; prefer "operators" when the sentence is about
  operating a running deployment.
- Mention breaking changes and upgrade work only when they affect server
  operators, admins, users, or self-hosted integrations.
- Feature cards should sell the outcome, not enumerate every touched service,
  screen, component, RPC, or edge case. Prefer plain product-level summaries
  such as "The first layer of localization has landed, with the frontend now
  available in English and German" over inventories like "the app shell,
  sidebar, settings, auth, chat, room, composer, calls, admin, media, and shared
  UI strings now use message catalogs." Leave implementation coverage for
  upgrade notes, reference docs, or the generated GitHub release.
- For performance cards, name the user/operator-visible improvement and keep it
  in the right audience section. In `## Running and Integrating Chatto`, focus
  on operator outcomes such as faster startup, replay, backup, restore,
  deployment, or resource use. Put frontend-only loading wins in a user-facing
  section only when users will actually notice them. Prefer titles like "Faster
  startup performance" or "Pages load faster" over internal labels like "Faster
  loading paths", "projection replay improvements", or "bundle splitting".
- For fixes, use direct, concrete wording. Avoid vague bullets, but do not force
  every item into the same sentence template.
- Keep cards independent so maintainers can move, delete, or rewrite one card at
  a time.

## Verification

- Run the docs build after edits:

```sh
mise x -- pnpm --filter docs-website build
```

- If the page or component styling changed materially, open the docs site in the
  browser and check the release page on desktop and mobile widths.
- Do not claim full verification if only the build ran.
