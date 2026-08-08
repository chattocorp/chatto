# ADR-065: Runtime JSON Client Internationalization

**Date:** 2026-08-05
**Status:** Accepted
**Supersedes:** [ADR-043](ADR-043-client-shell-internationalization.md)

## Context

ADR-043 introduced Paraglide as Chatto's compile-time message system. After the
catalog expanded to 29 locales and 21 sections, generated locale modules became
a material part of Vite's module graph and production builds approached Node's
default heap limit. The generated facade also duplicated application policy and
made every frontend command depend on an i18n compilation step.

The catalogs are already versioned JSON, Chatto does not need localized routes
or server-side rendering, and browsers provide locale-aware plural and number
rules through `Intl`. Translation services also exchange JSON more naturally
than generated TypeScript modules.

## Decision

Chatto will use the small, framework-neutral `@chatto/lingua` package under
`packages/lingua`. It reads ordinary nested JSON catalogs at runtime and does
not generate source code. The package is Apache-2.0 licensed and contains no
Chatto product policy or runtime dependencies.

British English (`en-GB`) remains the complete source and fallback locale. Its
21 sections are imported eagerly so message lookup is synchronous from first
render. Non-base catalogs remain individual Vite dynamic JSON imports in source,
but Rolldown coalesces them into at most two physical payload chunks per locale.
SvelteKit layouts own those two coarse catalog boundaries: the root layout loads
the public shell, and the `/chat` layout loads the remainder before entering the
authenticated application. Catalog availability never participates in a global
navigation hook. Switching locales loads the currently active boundary before
publishing the new locale. Sparse locale catalogs, including `en-US`, fall back
per key to British English.

Application code calls `m('section.path', values)` through
`$lib/i18n/messages`. Translation keys are intentionally runtime strings; the
system validates catalogs and interpolation placeholders in tests rather than
compiling generated message functions.

Keys ending in `_count` or `.count` contain CLDR plural-category objects and
require a finite numeric `count`. The selected branch need not display the
count, but every branch must use the same interpolation placeholders. Keys
ending in `_html` or `.html` can only use the separate HTML API, which escapes
interpolated values and produces a branded value for the application's reviewed
sanitizing renderer. Ordinary messages always produce text.

The locale set, negotiation, client-owned persistence, canonical unlocalized
routes, language-neutral events, and translation-quality policy from ADR-043
remain unchanged.

## Consequences

Production, development, test, desktop, and container builds no longer invoke
an i18n compiler or include generated locale JavaScript in Vite's module graph.
Adding or editing a translation is a JSON-only change. Catalog sections remain
independent source and loading units, while production users pay at most one
public-shell request and one authenticated-application request per selected
locale. Non-base translations stay outside initial route bundles. The production
bundle check enforces both properties.

Message keys are not statically checked at each call site. Complete-catalog
shape tests, placeholder validation, missing-key markers, and focused runtime
tests provide the guardrails instead. Renaming a key therefore requires search
and test discipline.

The base catalogs add their JSON data to the initial application bundle. This
is deliberate: it guarantees synchronous fallback without loading waterfalls.
A selected non-base locale costs roughly 17–26 KiB over Brotli at the current
catalog size. Public routes load about half of that; entering chat loads the
remainder once. This coarse split avoids a global navigation interceptor while
retaining a smaller public entry path.
