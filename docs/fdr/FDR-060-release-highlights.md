# FDR-060: Release Highlights and Changelog

**Status:** Active
**Last reviewed:** 2026-11-22

## Behavior

Each Chatto release can have highlights: short notes about the changes that most members or operators want to know about. A contributor writes a highlight as a Sweet Release note of type `highlight`, in addition to the `feature` or `fix` note of the same change. A highlight does not change the version.

Sweet Release writes the highlights to `CHANGELOG.md` in a **Highlights** section. This section comes first in each release, before **Breaking changes**, **Features**, **Fixes**, **Performance**, and **Smaller changes**. Notes of type `polish` go to **Smaller changes**.

The docs website has a **Changelog** page in the **Releases** group. It shows each Chatto release after 0.5.0, latest first, with all its sections. The highlights are in a tinted box. **Smaller changes** is folded. The page is generated from `CHANGELOG.md` when the website is built.

In the app, **Help → What's new** shows the highlights of each release that has highlights, latest first. Each release links to its release notes on GitHub. The page also links to the docs website changelog. The app does not show releases without highlights. The highlight text is in English in every locale; the page controls are translated.

## Rationale

Most members do not read a full changelog. One or two sentences about each notable change tell them what is new without the fixes and internal details. A separate note type lets the author of a change write for this reader and keeps the full note for operators.

The app bundles the highlights at build time and makes no request for them, so it shows the highlights of its own version and does not connect to a third party. A link to GitHub or the docs website opens only when the user selects it. The app does not bundle the complete changelog because of its size.

Entries through 0.5.0 have the release-please format and are not shown. The curated release pages on the docs website stay available until the changelog replaces them.

## Implementation

- `tools/changelog.mjs` parses the Sweet Release entries of `CHANGELOG.md`. The docs website and the frontend build use it.
- The docs website loads the changelog in the `changelog` content collection (`apps/docs-website/src/content.config.ts`) and shows it at `/changelog/`.
- The frontend Vite plugin `chatto-release-highlights` provides only the highlights as `virtual:chatto-release-highlights`. The page is `apps/frontend/src/routes/help/whats-new/`.
