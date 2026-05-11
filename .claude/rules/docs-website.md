---
paths:
  - "docs-website/**/*"
---

# Docs Website

The docs website lives in `docs-website/` and is built with [Starlight](https://starlight.astro.build/) (Astro).

## Keeping Docs in Sync

The docs website must stay in sync with the codebase. When adding, changing, or removing user-facing features, configuration options, or deployment behavior, update the corresponding docs pages:

- New environment variables or TOML options → add to `docs-website/src/content/docs/reference/environment-variables.mdx`
- New or changed features → update relevant guide in `docs-website/src/content/docs/guides/`
- Changed config defaults or semantics → update both the reference and any guides that mention the option

If you notice documentation that is out of date or inconsistent with the code, alert the user about the drift before proceeding.

## Assumptions

- The code repository is public and all binaries and Docker images (including `ghcr.io/hmans/chatto`) are publicly available. Don't include setup steps for repository access or container registry authentication.
- Refer to calls as **"voice and video calls"** or just **"calls"** — never "voice calls" alone. LiveKit handles both audio and video.

## Starlight Components

Use built-in Starlight components where appropriate. All are imported from `@astrojs/starlight/components`:

| Component | Use for |
|-----------|---------|
| `Steps` | Numbered setup/tutorial sequences |
| `Aside` | Callouts — `tip`, `note`, `caution`, `danger` |
| `FileTree` | Showing directory/file structures |
| `LinkCard` | Cross-references to other docs pages |
| `CardGrid` | Laying out multiple `LinkCard`s side-by-side |
| `Tabs` / `TabItem` | Showing alternatives (e.g., dev vs. prod config) |

## Sidebar Configuration

The sidebar is configured in `docs-website/astro.config.mjs` under `starlight.sidebar`. When adding new pages, add them to the appropriate section there.

## Avoiding Duplication

Prefer linking to dedicated guide pages rather than repeating detailed instructions in multiple places. For example, the Docker Compose page should link to the S3 storage guide rather than documenting S3 configuration inline. Use `LinkCard` components for these cross-references.

## Writing Style

- **Direct and concise.** Lead with what the reader needs to know, not background. Skip "In this guide, we will..." preambles.
- **Second person, present tense.** "You can run multiple replicas" not "One can run" or "The user runs."
- **Confident tone.** State facts plainly. Avoid hedging ("might", "perhaps", "it should be noted that").
- **Short paragraphs.** One idea per paragraph. Use tables and lists over long prose.
- **Show, then explain.** Put the config example first, then explain what it does — not the other way around for simple options.

## Terminology

- **"server"** (or "Chatto server" when clarity is needed) refers to a Chatto deployment — the logical entity with users, rooms, and data. Don't use "instance" for this; the codebase renamed `Instance` → `Server` (ADR-029) and the public docs follow suit.
- **"server process"** or **"replica"** for individual running copies of the Chatto binary behind a load balancer. Use "server process" when contrasting with the deployment as a whole (e.g. "any server process can serve any request", "all server processes must share the same secrets").
- Env var and config-key names that literally contain "instance" (e.g. `CHATTO_LIVEKIT_INSTANCE_ID`, `livekit.instance_id`) are real configuration identifiers — keep them as-is. You can still write the surrounding prose in terms of "server" / "server process".
- **"calls"** or **"voice and video calls"** — never "voice calls" alone.
- Don't recommend MinIO — it's dead. Use Cloudflare R2, Wasabi, Backblaze B2, or AWS S3 as example providers.

## Content Conventions

- Use `example.com` as the placeholder domain (e.g., `chat.example.com`, `livekit.chat.example.com`)
- Use `<generate-me>` as a placeholder for secrets that need to be generated
- Show both TOML config and environment variable alternatives where applicable (use `Tabs` component)
- Link to the environment variables reference for full option lists rather than duplicating them

## Font Sizing

Don't shrink text. Use the base font size for all readable content — titles, descriptions, body text. Only use smaller sizes (`text-xs`, `0.6rem`, etc.) for badges, labels, port numbers, and other metadata that isn't meant to be read as prose.

## Architecture Diagrams

SVG architecture diagrams live in `docs-website/src/assets/` and are imported as raw strings (`?raw`) for inline rendering — this is required for SVG animations to work (an `<img>` tag won't animate).

### Design Patterns

- **Dark/light mode**: Use `@media (prefers-color-scheme: light)` inside the SVG `<style>` to provide both color schemes. Dark mode is the default.
- **Box colors**: Each service type has its own subtle fill + border color (e.g., `.box-chatto`, `.box-nats`). Keep these muted — they shouldn't compete with the animated dots.
- **Connection lines**: Use `.conn` (solid) for persistent connections and `.conn-dash` (dashed) for direct/UDP connections that bypass the proxy.

### Animation Guidelines

- Use `<animateMotion>` with `<mpath>` to move dots along connection paths.
- **Easing**: Always use `calcMode="spline"` with `keySplines="0.4 0 0.2 1"` for a smooth ease-in-out feel. Never use linear (`calcMode="linear"` or default).
- **Bidirectional connections** (e.g., Chatto↔NATS): Use a single dot that bounces back and forth with `keyPoints="0;1;0"` and `keyTimes="0;0.5;1"` (two spline segments). Don't use two separate dots on parallel offset paths — it looks janky.
- **Unidirectional connections** (e.g., Browser→Caddy): Use one or two dots with staggered `begin` offsets traveling in the same direction.
- **Dot sizing**: Use `r="3"` for primary traffic dots, `r="2.5"` for secondary/API dots. Use `opacity="0.7"` to visually de-emphasize less important connections.
- **Dot colors**: `.dot` (sky blue) for main HTTP/WS traffic, `.dot-yellow` for media/WebRTC traffic, `.dot-blue` for internal messaging (NATS).
