# ADR-043: Client-Shell Internationalization

**Date:** 2026-06-22

## Context

Chatto needs internationalization support for its user interface. The current frontend is English-first:

- Most product UI text is hardcoded in Svelte components and TypeScript helpers.
- The app shell declares `lang="en"` in `app.html`, and the PWA manifest is English-only.
- Date and time helpers already use `Intl`, but some helpers still pin `en-US` for display labels.
- Server and room state is event-sourced, so persisted events must remain language-neutral facts rather than rendered human text.

The architecture shapes the i18n approach:

- Chatto ships the SvelteKit app as a static SPA embedded in the Go binary, with SSR disabled. Runtime production requests do not run SvelteKit server hooks.
- The frontend is multi-server and instance-agnostic. One browser client can connect to several Chatto servers, and there is no global Chatto account that spans those servers.
- Canonical app routes already encode server selection and chat navigation. Locale-prefixed routes would add churn to a routing surface that is mostly private app UI, not public SEO content.

Chatto needs an i18n system that can be adopted incrementally, gives good type-safety, keeps bundles reasonable as locales grow, and does not mix translated presentation text into durable domain data.

## Decision

Chatto will internationalize the web client with a compile-time message catalog system, using Paraglide JS unless a future implementation spike finds a blocking issue. Paraglide is acceptable because it uses in-repository project configuration and message files; it must not require a hosted translation service, account, or network call for normal local development, CI builds, or runtime operation.

English (`en`) is the source and fallback locale. German (`de`) is the first supported non-English locale because the project can review it directly. Message catalogs are version-controlled. Product UI strings should move into generated message functions as areas are touched, and new product UI strings should use those message functions from the start.

Locale payloads should be split into separate bundles and loaded lazily. Chatto compiles Paraglide with `locale-modules`, keeps English in the base client bundle, and loads non-base locale modules through app-owned dynamic imports. Product code imports messages from `$lib/i18n/messages` and locale runtime helpers from `$lib/i18n/runtime`; it must not import `$lib/paraglide/messages` directly because that generated index eagerly imports every locale module. Locale switches lazy-load the target catalog and update the current SPA reactively without a full-page reload. The i18n facade is generated from Paraglide's typed locale-module output by `frontend/scripts/generate-i18n-facade.mjs`; it remains the app-owned boundary that preserves lazy locale loading and in-place locale changes.

Chatto will prefer static, typed message keys over runtime string lookups:

- Component chrome, settings labels, dialogs, validation messages, empty states, toast text, and system-event labels use generated message functions.
- Stable enums, permission names, event types, and notification levels are mapped explicitly to message functions at the UI boundary.
- User-generated content, server names, room names, display names, message bodies, uploaded filenames, and other user-authored values are displayed as authored and are not translated.
- Backend APIs should return structured data, stable codes, enum values, and parameters rather than pre-rendered localized product copy.
- Persisted EVT events and projections must store language-neutral facts, not localized labels or sentences.

Message keys should be semantic and stable rather than English-as-key. Catalog files should use nested JSON grouped by feature or UI surface, such as `settings.preferences.title`, `auth.login.submit`, or `room.event.user_joined`. Catalog storage is split by locale and top-level domain (`frontend/messages/en/auth.json`, `frontend/messages/de/auth.json`, etc.) through Inlang's multiple `pathPattern` support so contributors do not have to edit one monolithic locale file. The message-format plugin flattens nested JSON internally, and Paraglide exposes nested paths as quoted exports used with bracket notation, such as `m["settings.preferences.title"]()`. If Paraglide later provides a stronger Svelte-specific namespacing convention, Chatto may adopt it, but new code must still keep keys stable across wording changes.

Locale selection is owned by the client shell, not by the active server. The effective locale is resolved in this order:

1. A browser-local Chatto locale preference.
2. The browser's language preferences.
3. `en`.

The selected locale applies to the whole SPA. It does not change when the user navigates between connected servers. This avoids conflicting per-server language settings in a multi-server client and keeps language selection available before authentication.

The frontend should include a user-facing language preference UI early in the implementation, backed by browser-local persistence. This gives users and testers a deterministic way to switch between `en` and `de` without changing browser settings.

Server-synced language preference may be added later as an additive user-profile feature, but it must define clear multi-server conflict semantics before becoming authoritative. A local browser override remains necessary for signed-out screens, first paint, and separately hosted clients.

Chatto will keep canonical app routes unlocalized for now. The app will not introduce `/de/...`, `/fr/...`, or translated route slugs for authenticated chat routes in the first i18n phase. Localized routing can be reconsidered later for public documentation, marketing pages, invite previews, or other public content where URL language carries real value.

The app shell must set language metadata correctly:

- `document.documentElement.lang` and `dir` are set as early as practical from the resolved locale and updated when the locale changes.
- Direction support is part of the locale model, but RTL locales require an explicit UI audit before being listed as supported.
- The default static web manifest remains English until Chatto adds a deliberate per-locale manifest strategy.

Dates, times, numbers, plurals, and relative labels should be formatted with the active locale through `Intl` or the message system's formatter support. Existing timezone and time-format settings still control timezone and 12/24-hour behavior; locale controls language, calendar labels, week starts, number formatting, and plural rules.

Locales should be added only when Chatto can maintain acceptable translation quality. Machine translation may be used to draft catalogs, but supported locales should be reviewed and kept complete enough that the product does not feel half-translated.

Agent and contributor instructions should be updated with the i18n policy. New user-visible frontend strings should normally add or update message keys in both `en` and `de`, with a best-effort German translation when the author can provide one. If a translation is uncertain, the PR should mark it clearly for review instead of silently omitting it or hardcoding English.

## Consequences

Compile-time message functions give Chatto type-checked message usage, tree-shakable locale bundles, and a clear path for incremental conversion. The cost is generated-code/tooling setup and catalog maintenance.

Supporting both `en` and `de` from the start prevents the architecture from being English-only in practice and gives the project a reviewable real translation target. The tradeoff is that every converted surface must carry translation work immediately.

Keeping locale selection client-owned avoids a poor multi-server user experience where switching servers changes the whole UI language. The tradeoff is that language does not automatically sync across devices in the first implementation.

Keeping app routes canonical avoids churn in SvelteKit route files, typed route helpers, saved links, notification targets, and multi-server URLs. The tradeoff is that Chatto does not get localized private-app URLs.

Leaving user-generated content untranslated keeps authorship, moderation, search, and encryption boundaries clear. Users can still use browser translation tools or future explicit translation features for message content, but that is a separate product feature.

Keeping persisted events language-neutral preserves replay compatibility and prevents future locale changes from requiring data migrations. Rendered wording can evolve without rewriting event history.

RTL support remains a deliberate later milestone. The i18n architecture should not block RTL, but Chatto should not claim RTL locale support until layouts, truncation, icons, gestures, and directional affordances have been reviewed in-browser.
