# FDR-060: Release Highlights and Changelog

**Status:** Active
**Last reviewed:** 2026-11-26

## Behavior

Each Chatto release can have highlights: short notes about the changes that most members or operators want to know about. A contributor writes a highlight as a Sweet Release note of type `highlight`, in addition to the `feature` or `fix` note of the same change. A highlight does not change the version.

Sweet Release writes the highlights to `CHANGELOG.md` in a **Highlights** section. This section comes first in each release, before **Upgrade notes**, **Breaking changes**, **Features**, **Fixes**, **Performance**, and **Polish**. Notes of type `polish` go to **Polish**.

The **Changelog** page links to all release pages, newest first. The pages through 0.5.0 keep their published content. Each numbered Sweet release gets a page when Sweet plans it and there are pending notes to show, or when its first changelog entry appears. Prereleases share the stable version’s URL. The index labels previews as unreleased. The sidebar lists the versions, and the homepage links to the highest version.

A prerelease page groups notes from its cycle by section. If it is the newest version, it also includes pending notes. After release, the page uses Sweet’s combined stable entry. Highlights and upgrade notes come first.

In the app, **Help → What's new** lists stable releases after 0.5.0, latest first. During a prerelease cycle, the guide also lists the prereleases for that version. When the stable version ships, its entry contains the highlights from the cycle. Each entry links to its release notes on GitHub. If a release has highlights, they appear above the link. The page also links to the docs website changelog. The highlight text is in English in every locale; the page controls are translated.

The app shows **Unreleased** when there are pending highlights in `.sweet/`. A release consumes these notes.

## Rationale

Most members do not read a full changelog. One or two sentences about each notable change tell them what is new without the fixes and internal details. A separate note type lets the author of a change write for this reader and keeps the full note for operators.

The app bundles the highlights at build time and makes no request for them, so it shows the highlights of its own version and does not connect to a third party. A link to GitHub or the docs website opens only when the user selects it. The app does not bundle the complete changelog because of its size.

## Implementation

- `tools/changelog.mjs` reads `CHANGELOG.md` and the pending notes in `.sweet/`, and parses the Sweet Release entries. The docs website and the frontend build use it.
- The frontend build caches of mise (`build-frontend` sources) and Turbo (`chatto-frontend#build` inputs) include `CHANGELOG.md`, `.sweet.config.yml`, `.sweet/*.md`, and the parser.
- The docs website loads the changelog in the `releases` content collection (`apps/docs-website/src/content.config.ts`) to build release pages and their index at `/changelog/`.
- The frontend Vite plugin `chatto-release-highlights` provides only the highlights as `virtual:chatto-release-highlights`. The page is `apps/frontend/src/routes/help/whats-new/`.
