# ADR-003: Render the User Interface with templ and Embedded Vite Assets

**Status:** Accepted

**Date:** 2026-07-31

## Context

Authling needs small, security-sensitive signup, login, logout, consent, and
account-management pages. A single-page application would require a separate
browser API, duplicate navigation and form state, and additional client-side
authentication boundaries that these flows do not currently justify.

The interface still needs a deliberate visual system. Authling must be able to
use Tailwind CSS, web fonts, and a maintained icon collection without loading
resources from third-party CDNs. Browser assets must remain part of the
self-contained Authling executable.

## Decision

Authling will render HTML on the server with templ. Ordinary links and forms
are the baseline interaction model, and all essential flows must work without
client-side JavaScript.

Vite is a build-time asset pipeline, not an application runtime or rendering
framework. It compiles Tailwind CSS and packages locally installed fonts and
compile-time Iconify glyphs. The generated assets are embedded into the Go
binary and served from Authling's own origin. A production deployment does not
need Node.js and does not contact a font, icon, or asset CDN.

The Go templates and browser assets are Authling-owned. They must not import
Chatto frontend code or rely on this repository's layout. Authling owns a
nested pnpm workspace and `mise.toml`; contributors run its development tasks
from the `authling/` directory, with `mise authling` as the CLI passthrough.
Repository-level CI may enter that directory, but Chatto's root task and
package catalogs do not own Authling's workflow.

Ordinary navigation uses locally bundled htmx 2 to replace the body without
reloading the document. This addresses repeated white flashes during full-page
navigation. The earlier plan to wait for htmx 4 is replaced by this limited use
of the stable htmx 2 release. The server still returns complete HTML pages.
Ordinary sign-in uses htmx so its account redirect can animate. Login validation
errors replace the form without a second submission. OAuth sign-in, other forms,
consent pages, and authentication links with query parameters use native
navigation. JavaScript is not a prerequisite for any authentication flow.

The enhancement uses an explicit route allowlist. It updates the title, URL,
and keyboard focus. History navigation requests fresh HTML; htmx history storage
is disabled so account data and form contents are not saved in browser storage.
An asset-version change triggers a fresh document load. Failed GET requests use
native navigation to show the server response. No third-party system receives
navigation requests, account data, or the user's IP address through this feature.

The [design system](../design-system.md) defines semantic color tokens, shared
type and spacing scales, and compact page layouts. Application-neutral templ
primitives live in `internal/web/ui`; Tailwind component classes live in
`web/src/app.css`. Feature templates own data, validation rules, and navigation.
The `/dev/ui` static component preview is registered only in `bootstrap` builds.

## Consequences

Authentication pages share request validation and response rendering with the
Go application and do not require a parallel JSON API. Pages remain accessible
when JavaScript is unavailable or blocked by policy.

Authling gains Node-based build dependencies, generated templ Go files, and
generated browser assets. The Vite output under `internal/web/assets/` is not
versioned; CI, tests, and release builds regenerate it before compiling the Go
binary. Font and icon licensing remains part of the shipped-artifact review.

The Content Security Policy restricts scripts, styles, and fonts
to Authling's own origin, with one exact style hash for the initial light/dark
canvas rule in the HTML head. Images may also use data URLs. Forms use Authling's
origin; pages that continue an OIDC request also permit its validated client
redirect origin so the browser can complete the redirect chain. This exception
does not permit arbitrary return URLs. Inline scripts and eval remain prohibited.
htmx disables eval, response scripts, and injected indicator styles. Future
enhancements must preserve the non-JavaScript path.
