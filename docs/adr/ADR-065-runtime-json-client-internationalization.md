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

British English (`en-GB`) remains the complete source and fallback locale.
Shared sections are imported eagerly so message lookup is synchronous from first
render. Management-only sections (`admin`, `rbac`, and `server_settings`) use
dynamic JSON imports for every locale, including British English. Messages used
outside management belong in shared sections: room actions and moderation text
in `room`, management navigation in `chat`, and permission descriptions in
`permissions`.

SvelteKit layout loads own three catalog groups: public, chat, and management.
The root layout loads the public shell. The `/chat` layout loads shared chat
sections without reading the child route. The management layout loads its
sections on entry. This keeps child navigation from rerunning ancestor access
checks or refreshing all translated text. Repeated requests for the same sections
do not change the translation revision. Rolldown keeps these groups in separate
payloads per locale. Catalog downloads never block a global navigation hook.
The existing completed-navigation handler deactivates management sections on
exit, without downloads or a translation refresh. Switching locales
loads the currently active group before publishing the new locale. Leaving
management removes its sections from subsequent locale loads but keeps previous
downloads cached. Sparse locale catalogs, including `en-US`, fall back
per key through their configured language parent when one exists and then to
British English. The current sparse regional overlays are `de-AT -> de-DE`,
`de-CH -> de-DE`, and `en-US -> en-GB`; complete regional catalogs such as
`nl-BE` and `fr-CA` do not configure a parent fallback and load only their
selected locale.

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
locale. Non-base translations stay outside initial route bundles.

Message keys are not statically checked at each call site. Complete-catalog
shape tests, placeholder validation, missing-key markers, and focused runtime
tests provide the guardrails instead. Renaming a key therefore requires search
and test discipline.

The shared base catalogs add their JSON data to the initial application bundle
to provide synchronous fallback. Management-only English and translated catalogs
are downloaded on management entry. A sparse regional overlay also loads its
configured parent catalogs for fallback validation and lookup. A failed catalog
load leaves published translations unchanged and can be retried. This split
keeps management text out of normal chat downloads, including for English users.
