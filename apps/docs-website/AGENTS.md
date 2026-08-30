# Instructions for Agents Working in `apps/docs-website/`

The docs website is the public Chatto documentation site. It uses Astro and
Starlight.

## Audience

- Write for community members, server operators, administrators, and API users.
- Do not put maintainer workflow text in visible pages. You can use hidden
  source comments when they help.
- The repository, binaries, and Docker images are public. Do not document
  private repository or registry access.

## Keep Docs In Sync

- Public API stability or version-skew changes: update
  `guides/integrations/api-compatibility.mdx` and the API
  overview generator in `tools/split-connectrpc-docs.mjs` when its summary
  changes.
- New or changed environment variables/TOML options: update
  `src/content/docs/reference/environment-variables.mdx`.
- New or changed user-facing features: update the relevant guide.
- Changed config defaults or deployment semantics: update both reference and
  guide pages that mention them.
- Add a sidebar entry in `astro.config.mjs` for a new page when needed.
- Keep generated ConnectRPC reference pages useful to API users.
- Do not add instructions to upgrade all replicas for a routine feature.
  Operators already keep replicas on a consistent version. Add rollout
  instructions only when an upgrade needs a special order, downtime, an
  irreversible migration, or another action beyond a normal deployment.

## Style

- Follow the ASD-STE100 rules and the approved exclusion list in the root
  `AGENTS.md`. The root list is the only documentation exclusion list.
- Use direct, short sentences.
- Use the second person and present tense.
- Give the action before background information.
- Prefer tables, short lists, and examples to long text.
- Show configuration examples before an explanation.
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

Use built-in Starlight components when they make the page clear:

- `Steps` for setup/tutorial sequences.
- `Aside` for `tip`, `note`, `caution`, and `danger` callouts.
- `FileTree` for directory/file structures.
- `LinkCard` and `CardGrid` for cross-references.
- `Tabs` and `TabItem` for alternatives such as TOML vs environment variables.

Link to a dedicated guide instead of repeating detailed instructions.

## Diagrams

- Put SVG architecture diagrams in `src/assets/`. Import them with `?raw` for
  animation.
- Support light and dark mode in SVG styles.
- Use muted service-box colors. Keep connection and dot styles consistent with
  existing diagrams.
- Use smooth `animateMotion` easing for moving dots. Do not use linear motion.
