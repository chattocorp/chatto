# ADR-129: Server Texts Use the Frontend Translation Catalogs

**Date:** 2026-11-27
**Status:** Accepted

## Context

The server writes some texts that people read outside the app: registration
codes, email verification codes, and password reset links (FDR-041). Until
this decision, these texts were English only.

The bundled frontend already has a complete translation workflow: Translatto
JSON catalogs in `apps/frontend/messages/<locale>/`, about 30 locales, rules
for the form of address in each language (ADR-065, `apps/frontend/AGENTS.md`),
and checks for missing keys and placeholders. A second, separate catalog
system on the server would split this work.

Go can embed only files in its own module (`cli/`). The frontend loads every
`*.json` file directly in a locale directory as a catalog section.

## Decision

- Server texts use the same Translatto catalogs. Their sections are in a
  `server/` subdirectory of each locale:
  `apps/frontend/messages/<locale>/server/<section>.json`. The frontend globs do
  not include subdirectories, so the web app does not load them.
- `mise sync-server-messages` copies these files to
  `cli/internal/i18n/messages/<locale>/`, which the binary embeds. The copies
  are in Git, so `go build` and `go test` need no extra step. A Go test fails
  when the copies and the sources differ, or when a frontend locale without a
  regional fallback has no server catalog.
- `cli/internal/i18n` reads the Translatto format: nested keys, `{placeholder}`
  interpolation, and plural objects with CLDR categories. It uses the CLDR
  plural rules of `golang.org/x/text`. Lookups fall back per key, as in the
  frontend: the requested locale, its regional fallback (for example `de-AT`
  to `de-DE`), then `en-GB`.
- The server selects the language from the request's `Accept-Language` header
  and matches it against the locales that have catalogs. The bundled frontend
  sends the language that the user selected in the app. `@chatto/client` takes
  a `locale` option where a request causes server text. Without a header, the
  server uses `en-GB`.
- Work that the server does later, such as the email queue, stores the
  selected locale with the work and translates when it runs.

## Consequences

- Translators and agents find all texts in one tree and follow the same
  language rules.
- Each server catalog exists twice in Git. Edit only the source under
  `apps/frontend/messages/` and run `mise sync-server-messages`; the test
  reports a forgotten sync.
- The frontend's catalog tests do not check the `server/` sections. The Go
  tests check them instead: every key of the base catalog in every complete
  locale, the same placeholders, and every plural category that each language
  uses for whole numbers.
- Browser defaults that do not exactly match a supported locale use the
  closest match of `golang.org/x/text/language`. For example, `fr-BE` selects
  `fr-CA`, and a bare `en` selects `en-US`, which falls back to `en-GB` text.
  Clients that know the user's choice send it explicitly.
- No API or protobuf field selects the language. A client that cannot set
  request headers gets the language of its platform's default header.

## Related

- **ADRs:** ADR-065 (frontend internationalisation), ADR-111 (Chatto client)
- **FDRs:** FDR-041 (Transactional Email Delivery)
