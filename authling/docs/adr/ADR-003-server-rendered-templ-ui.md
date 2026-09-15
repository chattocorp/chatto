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

JavaScript and progressive enhancement may be introduced for a demonstrated
interaction need. htmx is the preferred first option for server-driven
enhancement after htmx 4 is stable, but it is not part of the initial runtime
or a prerequisite for any authentication flow.

## Consequences

Authentication pages share request validation and response rendering with the
Go application and do not require a parallel JSON API. Pages remain accessible
when JavaScript is unavailable or blocked by policy.

Authling gains Node-based build dependencies, generated templ Go files, and
generated browser assets. The Vite output under `internal/web/assets/` is not
versioned; CI, tests, and release builds regenerate it before compiling the Go
binary. Font and icon licensing remains part of the shipped-artifact review.

The Content Security Policy prohibits scripts and restricts styles and fonts
to Authling's own origin. Images may also use data URLs. Forms use Authling's
origin; pages that continue an OIDC request also permit its validated client
redirect origin so the browser can complete the redirect chain. This exception
does not permit arbitrary return URLs. A future enhancement that needs
JavaScript must deliberately revise the policy and preserve the
non-JavaScript path.
