# Instructions for Agents Working in `apps/frontend/`

Frontend work uses SvelteKit, Svelte 5 runes, Tailwind 4, Paraglide i18n,
generated protobuf clients, Vitest browser tests, Playwright e2e, and Storybook.

## Svelte Tooling

- For Svelte questions or edits, use the Svelte docs/MCP workflow available to
  the agent session.
- When writing or editing `.svelte`, `.svelte.ts`, or `.svelte.js`, run the
  Svelte autofixer before handing back code.
- Do not generate a Svelte playground link for code written into this repo.

## Architecture

- Prefer store classes and thin components. Data lifecycle belongs in stores;
  components render state and call named store methods.
- Server-scoped state belongs in `ServerStateStore` or related per-server
  stores under `src/lib/state/server/`.
- Component-local `$state` is fine for UI-only state such as open/closed, hover,
  focus, draft text, and drag position.
- Component render DTOs live in `$lib/render/types`; keep them narrow and move
  callers toward protobuf-native API DTOs as Connect services replace legacy
  compatibility shapes.
- The URL is the source of truth for the active server. Pass explicit `serverId`
  values through helpers rather than relying on a global current server.
- Use Svelte `createContext` for context APIs, and prefer context over mutable
  singletons for URL-derived state.

## Svelte 5 Rules

- Use runes and Svelte 5 idioms; no legacy reactive statements.
- Avoid `$effect` unless synchronizing with DOM, subscriptions, timers, network
  calls, or other external systems. Use `$derived` for computed state.
- Do not mirror SvelteKit `load` data into stores from component `$effect`; set
  the store in the owner that already has the data.
- Wrap async/context getters in `$derived` when their result must update.
- Pass reactive values as getter functions to hooks that read them inside an
  effect; never suppress `state_referenced_locally`.
- Keep long-lived module state in `<script module>`, not instance `<script>`.
- Use `Snippet<[Args]>` for reusable layout/render snippets.
- Prefer attachments (`{@attach}`) over legacy actions for new reusable DOM
  behavior.
- Prefer Svelte template event attributes such as `onclick` and `onpointerdown`
  for component-owned DOM event handling. Reserve imperative event listeners for
  reusable actions, attachments, subscriptions, and external targets such as
  `window`, `document`, or third-party libraries.

## Routing And Navigation

- Use SvelteKit SPA routes under `src/routes/`.
- Use `resolve()` from `$app/paths` for internal links and `goto()` targets.
- For signed asset URLs and third-party URLs, use a purpose-built helper/control
  rather than disabling navigation lint rules.
- Modals use shallow routing via `pushState('', { modal: ... })`; close with
  history navigation.

## ConnectRPC And Generated Types

- Use the per-server compatibility state under `src/lib/state/server/` for
  protocol feature gating and version-skew warnings. Prefer discovery protocol
  capabilities; compare software versions only for legacy servers without
  compatibility metadata. Do not conflate protocol support with enabled server
  features or viewer permissions.
- Use the app's connection surface from
  `$lib/state/server/serverConnection.svelte.ts` for Connect base URLs,
  `/api/realtime` URLs, bearer tokens, auth-required handling, and
  reconnect/status UI state.
- Treat an intentionally dormant inactive-server transport as healthy retained
  state, not as a failed connection. Only actual transport/auth/protocol
  failures should dim its server-gutter entry.
- `$lib/render/types` is a hand-owned temporary render DTO compatibility layer,
  not generated API output. Do not add documents or generated calls for the
  retired legacy API.
- Query permissions/capability hints from the backend instead of duplicating
  authorization rules in UI code.
- When Go permission/shared types change, run `mise codegen-types`.
- Public ConnectRPC/protobuf clients live in the workspace package
  `@chatto/api-types`; keep generated files in sync with `mise codegen-proto`.

## UI And Styling

- Read [DESIGN_SYSTEM.md](DESIGN_SYSTEM.md) before changing visible UI. It is
  the canonical guide for choosing components, semantic utilities, tokens, and
  Storybook coverage.
- Use Tailwind 4 utilities and established components; avoid one-off CSS.
- Never add decorative one-sided accent borders or inset edge stripes to cards,
  rows, panels, or selected states. Use a uniform border when a real boundary is
  needed, and use fill plus the control's indicator to communicate selection.
- Prefer an established component, then a semantic utility from `src/app.css`,
  then raw Tailwind for local layout. Do not use `!` overrides to invent
  missing component variants; extend the component and its story instead.
- Svelte files use tabs; match local style.
- Use base text size by default. Reserve smaller text for metadata.
- Keep one text size within a compact surface such as a menu, popover, control,
  or nested row. Do not mix smaller metadata text with base-sized actions in
  the same surface; express hierarchy with color, weight, spacing, and icons.
- Use browser/platform default text rendering. Do not apply global font
  smoothing such as Tailwind `antialiased`, `-webkit-font-smoothing`, or
  `-moz-osx-font-smoothing`.
- Clickable controls need `cursor-pointer`.
- Do not use `{@html}` directly in feature components. Render trusted markdown
  HTML through `$lib/ui/MarkdownHtml.svelte`, which is the reviewed exception.
- Use `<SkeletonImg>` instead of `<img class="skeleton">`.
- Use `link` for inline links, not a hand-built `text-action` treatment.
- Flex children with truncation or fixed-width media usually need `min-w-0`.
- Prefer native browser scrolling for scrollable regions and galleries; do not
  intercept wheel, touch, or pointer scrolling unless the interaction is
  explicitly custom and approved.
- App-wide pan gestures must yield to horizontal scrollers, form and media
  controls, custom drag surfaces, and browser top-layer UI such as dialogs,
  popovers, and fullscreen elements. Mark custom surfaces with the gesture's
  explicit opt-out and cover both pointer and touch paths in tests.
- Do not double-nest `Panel`.
- `PaneHeader` actions are icon affordances. Put primary actions such as Save,
  Cancel, and Create in the page body or form area.
- Use forms for input groups with submit buttons: real `<form>`, submit button,
  native validation, and Enter-to-submit.
- Keep modal footer actions visible, horizontal, and `justify-end gap-2`.

## Floating UI

- Tooltips, popovers, context menus, autocompletes, and dropdowns should use
  `FloatingPopover` or a wrapper such as `ContextMenu` or `HelpTooltip`.
- Do not hand-roll floating UI with fixed positioning and z-index; top-layer
  popovers avoid clipping/stacking issues.
- Use established `.menu`, `menu-section`, `btn`, dialog, toast, and chat overlay
  patterns before inventing new floating styles.

## Internationalization

- New or changed user-visible strings go through the British English (`en-GB`)
  source and every complete translated Paraglide catalog. Preserve message
  structure and placeholders. Add a sparse US English (`en-US`) override when
  spelling or terminology differs; do not duplicate identical base messages.
  Locale identifiers use BCP 47 tags such as `en-GB`. Follow ADR-043.
- Import product messages from `$lib/i18n/messages`, not generated Paraglide
  internals.
- Use nested keys grouped by feature/surface; do not use English sentences as
  keys.
- Keep user-generated values untranslated.
- Do not product-qualify end-user accounts, users, members, or usernames in UI
  copy. Use "account", "user", "member", or "username"; in German, use forms
  such as "Konto", "Mitglied", and "Benutzername" without the product name as
  a prefix.

## Admin And Settings UI

- Server admin routes live under `/chat/[serverId]/server-admin/`.
- Checkboxes and similar binary controls in Server Admin should save immediately
  and confirm through toast.
- Use Save buttons only for multi-field forms that submit together; disable until
  dirty.
- Reuse admin/settings components from `$lib/components/admin`,
  `$lib/components/settings`, `$lib/components/rbac`, and `$lib/ui/form`.
- Implicit roles such as `everyone` should display as automatic/disabled, not as
  normal editable assignments.

## Pagination, Lists, And Realtime UI

- Use automatic "load more" pagination when a scroll/container edge is reached.
- Use event-driven updates from the per-server event bus and explicit projected
  refetches rather than assuming a normalized client cache.
- For paginated caches reconciled from realtime snapshots, queue relevant
  updates during first hydration instead of restarting it, fence and retry
  stale append reads, and version per-resource async refreshes so older
  responses cannot restore deleted or superseded data.
- Keep a realtime resume cursor RAM-only and owned by the exact per-server
  projection it advances. Socket teardown must not discard either one, and a
  recreated projection must resume without a cursor so it receives a reset.
- Treat undecodable realtime frames and unknown projection operations as fatal
  for that socket. Validate each projection event before mutation and never
  advance a cursor across input the reducer did not fully understand.
- Treat authorization loss, message deletion, key shredding, and account
  deletion as asynchronous privacy boundaries. Clearing current render state
  is insufficient: invalidate or fence older reads and optimistic rollbacks,
  and apply the boundary to every response that can arrive later.
- Application code must leave realtime transport ownership to the central
  coordinator: only the URL-active server keeps a persistent WebSocket, while
  inactive servers use serialized short-lived catch-ups over the same stream.
- Guard subscription creation on authentication/server availability to avoid
  reconnect loops.
- For virtualized lists (`virtua`), use real wheel interaction in e2e tests; raw
  `scrollTop` writes are unreliable.

## Testing

- `mise test-frontend` runs the frontend suite.
- Run frontend verification commands that compile Paraglide sequentially. In
  particular, do not run `mise lint-frontend` and `mise test-frontend` in
  parallel: one process can read `src/lib/paraglide/` while the other is
  rewriting it and report invalid generated-code diagnostics.
- Unit and component specs live next to source. Route specs should not start
  with `+`; use descriptive names such as `members.page.svelte.spec.ts`.
- Pure functions/classes can use Node Vitest. Mounted Svelte components,
  DOM/CSS/localStorage/drag behavior, context, and `$effect` runtime behavior
  need browser/component tests.
- E2E is for real backend/NATS/WebSocket/multi-user/cross-route behavior.
- When changing multi-server authentication or shared chat providers, cover an
  authenticated remote server with an anonymous origin server.
- Use helpers from `$lib/test-utils` rather than re-rolling connection/context
  mocks.
- Use `expect.element(...)` for DOM assertions and flush after Svelte state
  mutations when needed.
- For focused component tests, filter to the relevant test instead of initially
  running the entire spec. Use a plain substring without regular-expression
  characters such as `+`:

```sh
mise x -- pnpm --filter chatto-frontend exec vitest --run \
  src/path/Component.svelte.spec.ts -t 'plain substring'
```

- If Vite reloads after first-run dependency optimization and then stops making
  progress, terminate the test and rerun it once with the warmed cache.
- E2E runs locally without Docker/Tilt/OrbStack; Playwright starts its own
  embedded-NATS Chatto binary.
- Prefer targeted e2e runs before the full suite:

```sh
mise x -- pnpm exec playwright test e2e/dm.test.ts --retries=0
mise test-e2e
```

- Do not use raw `waitForTimeout`; use observable assertions or shared timeout
  constants. The only exception is documented wall-clock timing.
- Test realtime features from the receiver's perspective too, not only the actor.
- Permission tests need both allowed and denied cases.
- Use stable selectors (`data-testid` where needed) and unique message/body text.
- Monitor browser console/page errors in e2e when touching runtime behavior.

## Storybook

- Add or update stories for reusable components in `src/lib/ui/`,
  `src/lib/ui/form/`, and `src/lib/components/admin/`.
- Update stories when component props, variants, or design tokens change.
- Use addon-svelte-csf v5 conventions; pass `asChild` on `<Story>` blocks that
  contain markup.
- Stories should document behavior through realistic variants, not long prose.
- Literal fixture copy local to a story is exempt from Paraglide catalogs.
  Production component and route strings still require British English and
  German, plus US English overrides where wording differs.
- The app preview uses Chatto tokens; do not retint Storybook manager/docs chrome.
- Route accessibility coverage lives in `e2e/accessibility.test.ts`. Keep its
  representative public, authenticated, mobile, admin, and dialog scans free of
  blanket axe exclusions.

## PWA And Assets

- PWA manifest/icons live under `static/`; regenerate icons with
  `scripts/generate-icons.mjs` when the source changes.
- The service worker shell should keep API/auth/live/uploaded-asset requests
  network-only unless an FDR/ADR says otherwise.
