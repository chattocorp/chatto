# Instructions for Agents Working in `apps/docs-website/`

The docs website is Chatto's public documentation site, built with Astro and
Starlight.

## Audience

- Write for community members, server operators, administrators, and API
  consumers.
- Do not put maintainer workflow text in visible docs pages. Hidden source
  comments are fine when useful.
- The repository, binaries, and Docker images are public. Do not document private
  repo or registry access steps.

## Keep Docs In Sync

- New or changed environment variables/TOML options: update
  `src/content/docs/reference/environment-variables.mdx`.
- New or changed user-facing features: update the relevant guide.
- Changed config defaults or deployment semantics: update both reference and
  guide pages that mention them.
- New pages usually need sidebar entries in `astro.config.mjs`.
- Generated ConnectRPC reference pages must remain useful to API consumers, not
  protobuf maintainers.

## Style

- Be direct, concise, and confident.
- Use second person, present tense.
- Lead with the actionable fact, not background.
- Prefer tables, short lists, and examples over long prose.
- Show config examples before explaining them.
- Use base readable text size; reserve smaller text for labels, badges, and
  metadata.

## Terminology

- Use "server" or "Chatto server" for a deployment.
- Use "server process" or "replica" for one running binary behind a load
  balancer.
- Keep literal config names containing `instance` unchanged.
- Use "calls" or "voice and video calls", not "voice calls" alone.
- Do not recommend MinIO. Prefer Cloudflare R2, Wasabi, Backblaze B2, or AWS S3
  in examples.
- Use `example.com` placeholder domains and `<generate-me>` for secrets.

## Starlight

Use built-in Starlight components where they make the page clearer:

- `Steps` for setup/tutorial sequences.
- `Aside` for `tip`, `note`, `caution`, and `danger` callouts.
- `FileTree` for directory/file structures.
- `LinkCard` and `CardGrid` for cross-references.
- `Tabs` and `TabItem` for alternatives such as TOML vs environment variables.

Prefer linking to dedicated guides over repeating detailed instructions.

## Diagrams

- SVG architecture diagrams live in `src/assets/` and are imported raw with
  `?raw` when animation is needed.
- Support light/dark mode inside SVG styles.
- Keep service box colors muted and connection/dot styling consistent with the
  existing diagrams.
- Use smooth `animateMotion` easing for moving dots; avoid linear motion.
