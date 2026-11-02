---
name: chatto-i18n-hygiene
description: Audit and clean up Chatto frontend internationalisation, including catalog integrity, untranslated UI text, regional overrides, locale formatting, and RTL behavior. Use for an i18n hygiene pass or a focused translation audit.
---

# Chatto i18n Hygiene

Keep Chatto messages complete, consistent, and correct at their call sites.
Scope this workflow to `apps/frontend/`. Authling and Runling are independent
products. Inspect `packages/translatto/` only when a finding concerns the shared
runtime; keep Chatto policy out of that package.

## Establish scope

Read `apps/frontend/AGENTS.md`, especially **Internationalization**, and
`docs/adr/ADR-065-runtime-json-client-internationalization.md`. Use
`docs/GLOSSARY.md` for product terms. Follow the frontend-review and Svelte
skills when reviewing components.

Use the user's requested scope. With no narrower scope, check all catalogs
and scan production frontend source. State which surfaces received manual
review. An audit request is read-only. A cleanup request permits focused fixes
to confirmed defects; do not rewrite translations merely for style.

## Run existing checks

From the repository root:

```sh
mise x -- pnpm --filter chatto-frontend exec vitest run --project server src/lib/i18n
```

Read the tests before interpreting their results. `catalogs.spec.ts` checks
literal message keys, complete catalog structure, placeholders, regional
overlays, copied multi-word English, and functional examples. Other tests in
this directory cover locale selection, direction, loading, and message output.
Passing checks do not prove translation quality or complete call-site coverage.

## Inspect catalogs

- Derive the base locale, selectable locales, and sparse overlays from
  `src/lib/i18n/locales.ts`; do not infer completeness from a regional tag.
- Check every JSON file under `apps/frontend/messages/` for empty values and
  duplicate object keys. Ordinary JSON parsing silently discards duplicate
  keys. Preserve catalog structure and placeholder names and multiplicity.
- Compare sparse overrides with their configured fallback. Report redundant
  overrides, and check missing regional spelling differences. Do not fill a
  sparse overlay with a copy of its fallback.
- Inspect copied one-word English and all plural branches as well as messages
  covered by tests. Brand names, protocol tokens, and valid shared terms can
  be identical. Add an `identicalTranslations.ts` exception only when the text
  is correct for that key and locale; never use exceptions to hide missing work.
- Follow the form of address in the frontend instructions. Check German
  `du`/`dein`, Swiss spelling, British source spelling, and US overrides.
  Keep functional tokens such as `DELETE` and syntax examples valid.
- Check plural categories against the target language, not English's branch
  set. Preserve `other` and numeric `count` arguments. Use complete messages
  with placeholders instead of concatenating translated fragments.

## Inspect production call sites

Use `rg` to find candidates, then read their context. Exclude fixture text,
stories, test harnesses, documentation examples, logs, user content, and
deliberately literal identifiers before reporting untranslated copy.

Check visible text, accessibility names, tooltips, placeholders, validation,
toasts, empty states, errors, and confirmation dialogs. Trace dynamic keys and
aliases: the literal-key test cannot prove that each runtime key exists.
Do not delete apparently unused keys until dynamic callers are ruled out.

Check that locale changes update rendered labels, dates, numbers, and lists.
Look for fixed locales, early translation snapshots, and locale-independent
formatting of user-visible values. Preserve deliberate technical formats.
Check logical layout edges, directional icons, gestures, and bidi isolation.
Review HTML messages only through the existing sanitizing boundary.

Inspect `catalogSections.js` and catalog loaders when a key crosses surfaces.
Chat messages must not depend on management-only catalogs. Do not make all
catalogs eager to fix a missing translation.

## Fix and verify

For authorized fixes, update the British source and all complete locales when
the message contract changes. Add regional overrides only where needed. Keep
uncertain linguistic findings separate from confirmed defects.

Run the focused i18n suite again after fixes. Run type checks and relevant
component tests for changed call sites. For catalog-loading or chunk changes,
run the frontend build, which includes `check:i18n`; the chunk check needs a
current production build. Use `e2e/translation-loading.test.ts` for relevant
navigation and loading regressions.

For visible behavior changes, use the user's browser tool preference to verify
the affected surface in the source locale, the affected locale, and an RTL
locale when direction matters. Check locale switching and narrow-screen text
wrapping. Use `mise dev` when a server is needed and stop it afterward.
If browser verification is unavailable, state the gap.

Report confirmed findings with paths, message keys, affected locales, and user
impact. Separate fixes, remaining findings, and optional improvements. Include
the commands run and the limits of automated and manual coverage. Do not claim
that every translation is linguistically correct from structural tests alone.
