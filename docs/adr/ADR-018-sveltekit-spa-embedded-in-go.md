# ADR-018: SvelteKit SPA Embedded in Go Binary

**Date:** 2026-03-01

**Updated:** 2026-09-25

## Context

Chatto's design goal is a single self-hosted executable. The frontend is a SvelteKit application. The question is how to serve it:

- **Separate static hosting**: Deploy frontend to a CDN or static file server. Simple but breaks the single-binary goal and requires operators to manage two deployments.
- **SSR with Node.js**: Run SvelteKit's Node adapter alongside Go. Requires a second runtime, complicates the binary, adds operational overhead.
- **SPA mode embedded in Go**: Build SvelteKit as a static SPA and embed the output in the Go binary using `//go:embed`.

## Decision

Configure SvelteKit with `adapter-static`, `fallback: '200.html'`, and
`ssr = false`. Precompression is enabled only when
`CHATTO_FRONTEND_PRECOMPRESS=1`; CI and release workflows set this value.
Ordinary frontend builds leave precompression disabled.

The compiled SPA output is embedded into the Go binary with
`//go:embed all:.client`. Preparation of the embedded files removes each raw
file that has a gzip copy, retaining the gzip and Brotli representations.
The Go server handles:

- Serving the `200.html` fallback for all unrecognized routes (SPA client-side routing)
- Serving SvelteKit's immutable assets (`/_app/immutable/`) with 1-year cache headers and ETags
- Serving precompressed `.br` and `.gz` variants without runtime compression
- Decompressing the embedded gzip copy when a raw file is absent and an
  uncompressed response is needed
- Injecting server-side OpenGraph meta tags into the `200.html` response for asset/space preview URLs

## Consequences

- **Single application binary**: After the frontend build is prepared for
  embedding, `go build` includes the frontend, backend, and embedded NATS
  server in one executable. No separate frontend files are needed at runtime.
- **Smaller embedded frontend**: Release builds omit duplicate raw files. The
  Go server serves accepted compressed representations directly. Requests
  that need a missing raw representation incur gzip decompression.
- **No SSR**: The frontend is fully client-rendered. First paint shows a loading state until JavaScript boots. This is acceptable for an authenticated app where SEO doesn't matter.
- **OpenGraph tags are server-rendered**: Despite being an SPA, the Go server injects `<meta>` tags for link preview URLs (space invites, shared assets) by manipulating the `200.html` before serving. This gives good link previews without SSR.
- **Bundled frontend updates require a binary rebuild**: Updating the embedded
  frontend requires rebuilding the Go binary. A separately hosted frontend
  can be updated independently; see [ADR-025](ADR-025-multi-instance-client-architecture.md).
