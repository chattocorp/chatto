# Instructions for Agents Working in `apps/frontend/`

Frontend work uses SvelteKit, Svelte 5 runes, Tailwind 4, Lingua JSON i18n,
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
- Component render DTOs live in focused modules under `$lib/render`; keep them
  narrow and normalize generated protobuf data at API boundaries.
- The URL is the source of truth for the active server. Pass explicit `serverId`
  values through helpers rather than relying on a global current server.
- Descendants of a `[serverId]` route should obtain that server's store and
  connection from `ServerScope`. Reserve `serverRegistry` for providers and
  genuinely cross-server surfaces, and pass explicit server or viewer identity
  into reusable render components.
- Use Svelte `createContext` for context APIs, and prefer context over mutable
  singletons for URL-derived state.
- Reusable leaf render components, especially timeline rows, must not acquire
  app-shell UI context solely to navigate or open sidebars. Accept callback
  props from the owning route or room component so app-shell responsibilities
  stay with the owner and the leaf remains independently mountable in tests and
  stories.
- Prefer SvelteKit and Vite's automatic route and dynamic-import chunking.
  Introduce custom Rolldown chunk groups only for a measured need, and verify
  that they do not pull lazy dependencies into representative initial route
  graphs; Rolldown groups matched modules' dependencies recursively by default.
- When a frontend change alters imports of interaction components guarded by
  the production bundle check, run `mise build-frontend`; lint and component
  tests do not exercise the production route graph.
- When a host can improve a browser operation, expose a narrow optional
  capability through a focused `$lib/desktop` adapter and feature-detect that
  capability at the point of use. Keep the browser implementation as the
  default path; do not branch shared behavior on user agents, URL schemes,
  platforms, or a general Desktop flag. Follow ADR-072.

## Svelte 5 Rules

- Use runes and Svelte 5 idioms; no legacy reactive statements.
- Avoid `$effect` unless synchronizing with DOM, subscriptions, timers, network
  calls, or other external systems. Use `$derived` for computed state.
- Choose the smallest lifecycle owner for reusable browser and DOM behavior:
  use a Svelte attachment when behavior belongs to one element; use a mountable,
  possibly headless component when behavior should follow conditional rendering
  or use Svelte markup lifecycle such as `<svelte:window>` or
  `<svelte:document>`; use a reactive class when behavior owns complex or shared
  state, has multiple consumers, or needs a lifecycle independent of the
  component tree. Prefer attachments and mountable components over a reactive
  class used only to arrange setup and cleanup.
- For a simple component-lifetime timer, render the headless
  `$lib/lifecycle/Interval.svelte` or `$lib/lifecycle/Deadline.svelte` component
  instead of repeating timer setup and cleanup in a feature `$effect`. Keep
  debounce timers, request timeouts, animation scheduling, and timers owned by
  stores or protocol lifecycles with those owners; the headless components are
  specifically for behavior whose lifetime follows conditional markup.
- Do not mirror SvelteKit `load` data into stores from component `$effect`; set
  the store in the owner that already has the data.
- Wrap async/context getters in `$derived` when their result must update.
- Pass reactive values as getter functions to hooks that read them inside an
  effect; never suppress `state_referenced_locally`.
- Keep long-lived module state in `<script module>`, not instance `<script>`.
- Document a component for Svelte Language Server hover text with a markup
  `<!-- @component ... -->` comment. JSDoc inside `<script>` documents the
  adjacent JavaScript declaration, not the component itself.
- Use `Snippet<[Args]>` for reusable layout/render snippets.
- Prefer attachments (`{@attach}`) over legacy actions for new reusable DOM
  behavior.
- Prefer Svelte template event attributes such as `onclick` and `onpointerdown`
  for component-owned DOM event handling. Use `<svelte:window>` and
  `<svelte:document>` for component-owned handlers on those global targets.
  Reserve imperative event listeners for reusable actions, attachments,
  subscriptions, and third-party libraries.

## Routing And Navigation

- Use SvelteKit SPA routes under `src/routes/`.
- Use `resolve()` from `$app/paths` for internal links and `goto()` targets.
- For signed asset URLs and third-party URLs, use a purpose-built helper/control
  rather than disabling navigation lint rules.
- Modals use shallow routing via `pushState('', { modal: ... })`; close with
  history navigation.

## ConnectRPC And Generated Types

- Use the per-server compatibility state under `src/lib/state/server/` for
  feature gating and version-skew warnings. Record each gated feature's minimum
  server version in the shared compatibility table. Do not conflate versioned
  protocol support with enabled server features or viewer permissions.
- Use the app's connection surface from
  `$lib/state/server/serverConnection.svelte.ts` for Connect base URLs,
  `/api/realtime` URLs, bearer tokens, auth-required handling, and
  reconnect/status UI state.
- Keep the known-server catalogue and per-server sessions device-local and as
  separate state owners. Server IDs and origins are immutable after
  registration. Never serialize Chatto bearer tokens, user summaries, or
  reauthentication state into a public or shared catalogue.
- `StorageSlot.set` intentionally treats unavailable/full browser storage as a
  best-effort no-op. When protocol correctness or security requires state to
  survive a reload or lost response, persist it before the external effect and
  read it back successfully before sending the request.
- Persist rotating credentials and other security-sensitive cross-tab state in
  independently keyed, versioned per-server records. Never let an ordinary
  metadata write replace them from a whole-registry in-memory snapshot; merge
  authoritative security fields at compatibility-adapter boundaries.
- Treat an intentionally dormant inactive-server transport as healthy retained
  state, not as a failed connection. Only actual transport/auth/protocol
  failures should dim its server-gutter entry.
- `$lib/render/timelineEvents` contains the hand-owned timeline presentation
  model; transient realtime signals belong in `$lib/realtimeEvents`. Do not
  combine the two delivery paths or add calls for the retired legacy API.
- Query permissions/capability hints from the backend instead of duplicating
  authorization rules in UI code.
- Public ConnectRPC/protobuf clients live in the workspace package
  `@chatto/api-types`; keep generated files in sync with `mise codegen-proto`.

## UI And Styling

- Read [DESIGN_SYSTEM.md](DESIGN_SYSTEM.md) before changing visible UI. It is
  the canonical guide for choosing components, semantic utilities, tokens, and
  Storybook coverage.
- Use Tailwind 4 utilities and established components; avoid one-off CSS.
- Keep `src/app.css` scoped to Tailwind's `source('./')` detection root. Production
  CSS must not scan frontend E2E tests, scripts, or build configuration outside
  `src`; co-located unit tests and stories remain inside the application boundary.
- Use Iconify's dynamic utilities, for example `icon-[uil--check]`. Keep its Tailwind
  plugin in dynamic mode; configuring `prefixes` eagerly expands complete icon
  collections and materially increases production-build memory.
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
- When an element supports both right-click actions and touch long-press
  actions, suppress touch-synthesized `contextmenu` events while the long-press
  gesture is active so only one action surface opens.

## Internationalization

- New or changed user-visible strings go through the British English (`en-GB`)
  source and every complete translated JSON catalog. Preserve message
  structure and placeholders. Add a sparse US English (`en-US`) override when
  spelling or terminology differs; do not duplicate identical base messages.
  Locale identifiers use BCP 47 tags such as `en-GB`. Follow ADR-065.
- German translations, including regional overlays, must address users with
  the informal `du`/`dein` forms rather than the formal `Sie`/`Ihr` forms.
- Import product messages from `$lib/i18n/messages`; keep the framework-neutral
  JSON runtime in `packages/lingua` free of Chatto-specific catalogs and policy.
- Catalogs are ordinary nested JSON and require no compilation. The British
  English source is bundled as the synchronous fallback. SvelteKit layout
  loads own the coarse non-base catalog boundaries: the root layout loads the
  public shell and `/chat` loads the complete selected locale. Catalog loading
  must not block a global navigation hook. Production builds coalesce the
  section imports into at most a public and chat payload per non-base locale;
  keep the bundle check enforcing that request boundary. Keys ending in
  `_count` or `.count` contain CLDR plural objects and receive `{ count }`. Keys
  ending in `_html` or `.html` are only rendered through the reviewed sanitizing
  HTML boundary.
- Use nested keys grouped by feature/surface; do not use English sentences as
  keys.
- Keep user-generated values untranslated.
- Prefer logical Tailwind layout utilities (`start`/`end`, `s`/`e`, `ps`/`pe`,
  `ms`/`me`, and `text-start`/`text-end`) when an edge follows reading
  direction. Keep physical left/right positioning only for coordinates,
  centring, media controls, and other deliberately physical behavior.
- Mirror directional icons and horizontal gestures in RTL. Isolate
  user-authored names and message content with `bdi`, `dir="auto"`, or an
  equivalent bidi boundary; keep code, identifiers, and URLs deliberately LTR
  where their syntax requires it.
- Do not product-qualify end-user accounts, users, members, or usernames in UI
  copy. Use "account", "user", "member", or "username"; in German, use forms
  such as "Konto", "Mitglied", and "Benutzername" without the product name as
  a prefix.

## Admin And Settings UI

- Management routes live under `/chat/[serverId]/manage/`: server-scoped pages
  under `manage/server/`, rooms under `manage/rooms/`, and room groups under
  `manage/room-groups/`.
- Server Configuration, server-scoped User Preferences, and App Preferences
  share the standard pane-page composition. Put their content in `PaneContent`
  and frame each page-level form or control group with a titled, padded `Panel`;
  use `FormSection` only to subdivide one panel, never instead of its frame.
- SvelteKit reuses resource pages when only a route parameter changes. Fence
  async loads and saves by both resource ID and load generation so late
  responses cannot update the next resource's form state.
- Send sparse patches from settings forms: omit unchanged fields so stale form
  values cannot overwrite concurrent updates or emit misleading durable facts.
- When an interactive edit returns an OCC conflict, do not retry it silently or
  replace the user's draft. Keep the form state and show a localized,
  actionable conflict message explaining that the resource changed and must be
  reloaded before saving again.
- Checkboxes and similar binary controls in Server Admin should save immediately
  and confirm through toast.
- Use Save buttons only for multi-field forms that submit together; disable until
  dirty.
- Reuse admin/settings components from `$lib/components/admin`,
  `$lib/components/settings`, `$lib/components/rbac`, and `$lib/ui/form`.
- Implicit roles such as `everyone` should display as automatic/disabled, not as
  normal editable assignments.

## Pagination, Lists, And Realtime UI

- When adapting canonical users or members for avatar-bearing UI, preserve
  identity fields such as `isBot`; prefer the shared `UserAvatar` and
  `UserAvatarUserView` shapes over surface-local copies.

- Use automatic "load more" pagination when a scroll/container edge is reached.
- Use TanStack Query for snapshot-style ConnectRPC reads. Scope private query
  keys by server and connection session, keep the cache memory-only, and purge
  it at authentication and privacy boundaries. Keep realtime projections,
  timelines, notifications, presence, calls, and message search in their
  owning per-server stores; see ADR-062.
- Use event-driven updates from the per-server event bus and explicit projected
  refetches rather than assuming a normalized client cache.
- When a snapshot query also reconciles a realtime-owned store, do not replay a
  cached mount snapshot into that store; wait for a successful fresh response.
  If a mutation cancels an in-flight refresh, serialize related mutations and
  resume authoritative reconciliation after both success and failure.
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

- Review visible frontend changes in a browser using Chrome DevTools MCP.
- Do not run frontend checks, tests, builds, or other commands that invoke
  SvelteKit sync concurrently in the same checkout. They share generated
  `.svelte-kit` state and can produce transient missing-type failures.
- `mise test-frontend` runs the frontend suite.
- The server, browser-component, and Storybook Vitest projects run sequentially
  to bound peak memory while still executing the complete suite.
- `mise lint-frontend` and `mise test-frontend` may run independently; neither
  rewrites generated internationalisation code.
- Unit and component specs live next to source. Route specs should not start
  with `+`; use descriptive names such as `members.page.svelte.spec.ts`.
- Pure functions/classes can use Node Vitest. Mounted Svelte components,
  DOM/CSS/localStorage/drag behavior, context, and `$effect` runtime behavior
  need browser/component tests.
- Keep debounce assertions independent of browser-suite scheduling: use fake
  timers or dispatch the complete input value synchronously instead of timing
  multi-keystroke `userEvent.type` calls against the production delay.
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
- For performance work, capture a baseline before changing code and compare it
  with the candidate on the same machine and under the same fixture settings.
  The performance suite records five samples by default, compares their
  medians, and retains every raw sample in the result file:

```sh
CHATTO_E2E_PERF_RESULT_PATH=/tmp/chatto-performance-base.json \
  mise test-e2e-performance

# Make the performance change, then measure it with the same settings.
CHATTO_E2E_PERF_RESULT_PATH=/tmp/chatto-performance-candidate.json \
  mise test-e2e-performance

mise compare-e2e-performance -- \
  /tmp/chatto-performance-base.json \
  /tmp/chatto-performance-candidate.json
```

  Do not compare results from different machines, fixture sizes, measurement
  versions, or sample counts. Use `CHATTO_E2E_PERF_SAMPLES` only when both runs
  use the same value. Inspect the raw samples and min/max statistics when a
  median moves unexpectedly; do not optimize against a single observation.
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
- Literal fixture copy local to a story is exempt from application catalogs.
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
