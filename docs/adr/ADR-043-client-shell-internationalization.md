# ADR-043: Client-Shell Internationalization

**Date:** 2026-06-22
**Updated:** 2026-07-14

## Context

Chatto needs internationalization support for its user interface. The current frontend is English-first:

- Most product UI text is hardcoded in Svelte components and TypeScript helpers.
- The PWA manifest is English-only.
- Date and time helpers already use `Intl`, but some helpers still pin `en-US` for display labels.
- Server and room state is event-sourced, so persisted events must remain language-neutral facts rather than rendered human text.

The architecture shapes the i18n approach:

- Chatto ships the SvelteKit app as a static SPA embedded in the Go binary, with SSR disabled. Runtime production requests do not run SvelteKit server hooks.
- The frontend is multi-server and instance-agnostic. One browser client can connect to several Chatto servers, and there is no global account that spans those servers.
- Canonical app routes already encode server selection and chat navigation. Locale-prefixed routes would add churn to a routing surface that is mostly private app UI, not public SEO content.

Chatto needs an i18n system that can be adopted incrementally, gives good type-safety, keeps bundles reasonable as locales grow, and does not mix translated presentation text into durable domain data.

## Decision

Chatto will internationalize the web client with a compile-time message catalog system, using Paraglide JS unless a future implementation spike finds a blocking issue. Paraglide is acceptable because it uses in-repository project configuration and message files; it must not require a hosted translation service, account, or network call for normal local development, CI builds, or runtime operation.

British English (`en-GB`) is the source and fallback locale. US English (`en-US`) is a regional content locale with sparse overrides for differences in spelling and terminology. German uses explicit `de-DE`, `de-AT`, and `de-CH` content locales; the former language-only `de` preference migrates to `de-DE`.

The other supported content locales are Dutch (`nl-NL`, `nl-BE`), Swedish (`sv-SE`), French (`fr-FR`, `fr-CA`), Spanish (`es-ES`, `es-419`), Portuguese (`pt-BR`, `pt-PT`), Norwegian Bokmål (`nb-NO`), Polish (`pl-PL`), Ukrainian (`uk-UA`), Japanese (`ja-JP`), and Esperanto (`eo`). Message catalogs are version-controlled. Product UI strings should move into generated message functions as areas are touched, and new product UI strings should use those message functions from the start.

Locale identifiers use canonical BCP 47 language tags such as `en-GB`, not POSIX-style identifiers such as `en_GB`. A locale identifies translated content, not only formatting conventions: regional variants must be distinct locales when their spelling, terminology, grammar, or other wording differs. A regional catalog may contain only its differences and inherit missing messages through Paraglide's locale fallback. The locale picker must name supported variants explicitly rather than presenting an ambiguous language label such as “English”.

Locale payloads should be split into separate bundles and loaded lazily. Chatto compiles Paraglide with `locale-modules`, keeps `en-GB` in the base client bundle, and loads non-base locale modules through app-owned dynamic imports. Product code imports messages from `$lib/i18n/messages` and locale runtime helpers from `$lib/i18n/runtime`; it must not import `$lib/paraglide/messages` directly because that generated index eagerly imports every locale module. Locale switches lazy-load the target catalog and update the current SPA reactively without a full-page reload. The i18n facade is generated from Paraglide's typed locale-module output and the locales in `project.inlang/settings.json` by `apps/frontend/scripts/generate-i18n-facade.mjs`; it remains the app-owned boundary that preserves lazy locale loading and in-place locale changes.

Chatto will prefer static, typed message keys over runtime string lookups:

- Component chrome, settings labels, dialogs, validation messages, empty states, toast text, and system-event labels use generated message functions.
- Stable enums, permission names, event types, and notification levels are mapped explicitly to message functions at the UI boundary.
- User-generated content, server names, room names, display names, message bodies, uploaded filenames, and other user-authored values are displayed as authored and are not translated.
- Backend APIs should return structured data, stable codes, enum values, and parameters rather than pre-rendered localized product copy.
- Persisted EVT events and projections must store language-neutral facts, not localized labels or sentences.

Message keys should be semantic and stable rather than English-as-key. Catalog files should use nested JSON grouped by feature or UI surface, such as `settings.preferences.title`, `auth.login.submit`, or `room.event.user_joined`. Catalog storage is split by locale and top-level domain (`apps/frontend/messages/en-GB/auth.json`, `apps/frontend/messages/en-US/auth.json`, `apps/frontend/messages/de/auth.json`, etc.) through Inlang's multiple `pathPattern` support so contributors do not have to edit one monolithic locale file. The message-format plugin flattens nested JSON internally, and Paraglide exposes nested paths as quoted exports used with bracket notation, such as `m["settings.preferences.title"]()`. If Paraglide later provides a stronger Svelte-specific namespacing convention, Chatto may adopt it, but new code must still keep keys stable across wording changes.

Locale selection is owned by the client shell, not by the active server. The effective locale is resolved in this order:

1. A browser-local Chatto locale preference.
2. The browser's language preferences.
3. `en-GB`.

Locale negotiation prefers an exact supported regional tag. A language-only preference uses the project's documented default region for that language, such as `en-GB`, `de-DE`, `nl-NL`, `fr-FR`, `es-ES`, or `pt-BR`. The former stored `en` and `de` preferences migrate to `en-GB` and `de-DE`, respectively, so explicit choices are not lost during the transition.

The selected locale applies to the whole SPA. It does not change when the user navigates between connected servers. This avoids conflicting per-server language settings in a multi-server client and keeps language selection available before authentication.

The frontend includes a user-facing language preference UI backed by browser-local persistence. Every supported regional content locale is listed explicitly, and language names are localised with `Intl.DisplayNames` so the picker remains readable after switching locale.

Server-synced language preference may be added later as an additive user-profile feature, but it must define clear multi-server conflict semantics before becoming authoritative. A local browser override remains necessary for signed-out screens, first paint, and separately hosted clients.

Chatto will keep canonical app routes unlocalized for now. The app will not introduce `/de/...`, `/fr/...`, or translated route slugs for authenticated chat routes in the first i18n phase. Localized routing can be reconsidered later for public documentation, marketing pages, invite previews, or other public content where URL language carries real value.

The app shell must set language metadata correctly:

- `document.documentElement.lang` and `dir` are set as early as practical from the resolved locale and updated when the locale changes.
- Direction support is part of the locale model, but RTL locales require an explicit UI audit before being listed as supported.
- The default static web manifest remains English until Chatto adds a deliberate per-locale manifest strategy.

Dates, times, numbers, plurals, and relative labels should be formatted with the active content locale through `Intl` or the message system's formatter support. Existing timezone and time-format settings still control timezone and explicit 12/24-hour behavior; locale controls language, default date and time conventions, calendar labels, week starts, number formatting, and plural rules. Region-bearing content locales are authoritative for these defaults. A language-only content locale may inherit the browser region until a more specific supported content locale exists. A separate formatting-region override may be added later without changing the content locale.

Locales should be added only when Chatto can maintain acceptable translation quality. Machine translation may be used to draft catalogs, but supported locales should be reviewed and kept complete enough that the product does not feel half-translated.

Agent and contributor instructions should be updated with the i18n policy. New user-visible frontend strings must add or update the `en-GB` source and all complete translated catalogs while preserving message structure and placeholders. Add an `en-US` override only when US spelling or terminology differs; do not duplicate identical base messages. If a translation is uncertain, the PR should mark it clearly for review instead of silently omitting it or hardcoding English.

## Consequences

Compile-time message functions give Chatto type-checked message usage, tree-shakable locale bundles, and a clear path for incremental conversion. The cost is generated-code/tooling setup and catalog maintenance.

Supporting regional English content locales prevents “English” from silently mixing dialects or treating region as formatting-only. Sparse `en-US` overrides avoid duplicating the complete British English catalog, but contributors must recognise and maintain genuine regional wording differences.

Supporting both English and German prevents the architecture from being English-only in practice and gives the project a reviewable non-English translation target. The tradeoff is that every converted surface must carry translation work immediately.

Keeping locale selection client-owned avoids a poor multi-server user experience where switching servers changes the whole UI language. The tradeoff is that language does not automatically sync across devices in the first implementation.

Keeping app routes canonical avoids churn in SvelteKit route files, typed route helpers, saved links, notification targets, and multi-server URLs. The tradeoff is that Chatto does not get localized private-app URLs.

Leaving user-generated content untranslated keeps authorship, moderation, search, and encryption boundaries clear. Users can still use browser translation tools or future explicit translation features for message content, but that is a separate product feature.

Keeping persisted events language-neutral preserves replay compatibility and prevents future locale changes from requiring data migrations. Rendered wording can evolve without rewriting event history.

RTL support remains a deliberate later milestone. The i18n architecture should not block RTL, but Chatto should not claim RTL locale support until layouts, truncation, icons, gestures, and directional affordances have been reviewed in-browser.
