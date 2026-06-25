# Instructions for Agents Working in `apps/frontend/`

Frontend work uses SvelteKit, Svelte 5 runes, Tailwind 4, Paraglide i18n,
GraphQL, generated protobuf clients, Vitest browser tests, Playwright e2e, and
Storybook.

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
- Colocate GraphQL fragments with the component that renders the fields; stores
  compose those fragments in queries.
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

## Routing And Navigation

- Use SvelteKit SPA routes under `src/routes/`.
- Use `resolve()` from `$app/paths` for internal links and `goto()` targets.
- For signed asset URLs and third-party URLs, use a purpose-built helper/control
  rather than disabling navigation lint rules.
- Modals use shallow routing via `pushState('', { modal: ... })`; close with
  history navigation.

## GraphQL, ConnectRPC, And Generated Types

- Use the app's GraphQL client surface from `$lib/state/server/graphqlClient.svelte.ts`;
  do not use `getContextClient()` from `@urql/svelte`.
- Generated GraphQL types live in `$lib/gql/graphql`; import the `graphql` tag
  from `$lib/gql`.
- Query permissions/capability hints from the backend instead of duplicating
  authorization rules in UI code.
- When Go permission/shared types change, run `mise codegen-types`.
- When frontend GraphQL operations change, run `mise codegen-frontend`.
- Public ConnectRPC/protobuf clients live under `$lib/pb`; keep generated files
  in sync with `mise codegen-proto`.

## UI And Styling

- Use Tailwind 4 utilities and established components; avoid one-off CSS.
- Svelte files use tabs; match local style.
- Use base text size by default. Reserve smaller text for metadata.
- Clickable controls need `cursor-pointer`.
- Never use `{@html}`.
- Use `<SkeletonImg>` instead of `<img class="skeleton">`.
- Use `link` for inline links, not `text-primary`.
- Flex children with truncation or fixed-width media usually need `min-w-0`.
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

- New or changed user-visible strings go through Paraglide catalogs with both
  English and German entries. Follow ADR-043.
- Import product messages from `$lib/i18n/messages`, not generated Paraglide
  internals.
- Use nested keys grouped by feature/surface; do not use English sentences as
  keys.
- Keep user-generated values untranslated.

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
- Use event-driven updates from the per-server event bus rather than assuming a
  GraphQL cache.
- Guard subscription creation on authentication/server availability to avoid
  reconnect loops.
- For virtualized lists (`virtua`), use real wheel interaction in e2e tests; raw
  `scrollTop` writes are unreliable.

## Testing

- `mise test-frontend` runs the frontend suite.
- Unit and component specs live next to source. Route specs should not start
  with `+`; use descriptive names such as `members.page.svelte.spec.ts`.
- Pure functions/classes can use Node Vitest. Mounted Svelte components,
  DOM/CSS/localStorage/drag behavior, context, and `$effect` runtime behavior
  need browser/component tests.
- E2E is for real backend/NATS/WebSocket/multi-user/cross-route behavior.
- Use helpers from `$lib/test-utils` rather than re-rolling GraphQL/context
  mocks.
- Use `expect.element(...)` for DOM assertions and flush after Svelte state
  mutations when needed.
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
- The app preview uses Chatto tokens; do not retint Storybook manager/docs chrome.

## PWA And Assets

- PWA manifest/icons live under `static/`; regenerate icons with
  `scripts/generate-icons.mjs` when the source changes.
- The service worker shell should keep API/auth/live/uploaded-asset requests
  network-only unless an FDR/ADR says otherwise.
