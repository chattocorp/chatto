---
name: chatto-release-checklist
description: Check Chatto release readiness, including release metadata, upgrade guidance, and docs website consistency. Use when preparing or reviewing a Chatto release or release PR.
---

# Chatto Release Checklist

Read [the release process](../../../docs/RELEASING.md). Paths below are relative to
the repository root. This checklist applies to Chatto, not the independently
versioned products and libraries in this repository.

## Before Publication

During release preparation or PR review, check the proposed release commit
and available CI evidence. Mark publication checks as pending until publication
occurs; missing published artifacts at this stage are not failures.

- Identify the target version and whether it is planned, prerelease, or stable
  from Sweet Release state, the release PR, and published tags or releases.
  Check that version targets and changelog entries agree with that state.
- Confirm the intended source commit. If the tag exists, verify that it points
  to the commit that introduced the version. Check that workflow runs use
  that tag and commit. Do not move published tags.
- Check that release notes describe the changes since the previous stable
  release and include required upgrade actions. Use
  [chatto-release-notes](../chatto-release-notes/SKILL.md) when writing or
  updating the release page.
- Check required CI results for the release commit. Distinguish failed checks
  from checks that are still running or waiting for approval.

## Compatibility And Upgrades

- Use [chatto-api-compatibility](../chatto-api-compatibility/SKILL.md) for
  public API and protocol changes since the previous stable release.
- Check changes to `MINIMUM_SUPPORTED_SERVER_VERSION` and
  `MINIMUM_CONNECTABLE_SERVER_VERSION` against root `AGENTS.md`. Verify the
  required PR labels and explanations, including the justification and
  operator upgrade action for a connection-threshold increase.
- Check evidence of upgrades from the previous stable release, especially
  when persisted events, schemas, storage, or migrations change. State the
  source and target versions, what was tested, and the results. Flag missing
  upgrade evidence; fresh-install tests alone do not verify upgrades.
- Confirm that release notes and operator guidance contain the required
  migration actions and any supported rollback limits.

## Docs Website

When preparing or reviewing a Chatto release, check the docs website as well
as the release page. Read `docs/RELEASING.md#documentation-channels` to
distinguish stable documentation from development documentation.

- Check the homepage's `Release Notes` action in
  `apps/docs-website/src/content/docs/index.mdx`. It must link to the release
  page for the latest stable minor version represented by the site. Preparing
  an unreleased page alone does not make it the latest stable release.
- Check release navigation in `apps/docs-website/astro.config.mjs` and links
  from guides to release notes. Verify that target pages and fragments exist
  and match the release discussed. Preserve intentional historical links.
- Check release titles, descriptions, hero status, version labels, and GitHub
  release links against the release state. Check installation examples,
  upgrade guidance, and compatibility guidance for stale version references
  or claims that conflict with the release notes.
- Follow the human-authored page rules in `apps/docs-website/AGENTS.md`.
  Report required corrections to protected pages unless the user has requested
  those edits. For review-only tasks, report findings without edits.
- Verify changed navigation links in a local browser preview. If checking a
  deployed site, report its channel and distinguish deployed behavior from
  local changes. Stable docs corrections ship with the next patch release.

## After Publication

Use [the release process](../../../docs/RELEASING.md) for the expected
artifacts and publication workflows. Check each applicable target separately:

- Server archives and checksums on the GitHub release.
- Server and frontend container images in GHCR.
- Desktop packages and checksums, with successful signing verification for
  Windows and signing and notarisation for macOS.
- The `@chatto/client` npm package.
- The Homebrew update and stable docs build for a stable release. Verify the
  deployed stable docs version and release links after deployment.
- The independent iOS TestFlight workflow and upload result.

Confirm that artifacts and workflow results match the intended version and
source commit. Check GitHub's draft and prerelease status; only the highest
stable Chatto version must claim **Latest**. A successful server workflow or
GitHub release does not prove that every publication target succeeded.

Report each target as verified, pending, failed, or not applicable, with
evidence or the reason. Follow the release process for partial publication
and retries; do not rebuild a published version with changed contents.

## Verification And Report

Run `mise x -- pnpm --filter docs-website build` after page edits. Report
findings, checks run, and checks still needed. Do not claim release readiness
from documentation checks alone. This checklist does not authorize tagging,
publication, deployment, or approval of release workflows.
