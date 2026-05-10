# General Rules

## Prime Directives

- DO keep tests and documentation up to date when making changes (see `docs-website.md` for docs website specifics)
- DON'T create commits unless explicitly asked to do so

## Priorities

- Efficient resource usage (CPU, memory, storage) to minimize hosting costs
- Single executable with everything out of the box (exceptions for pluggable advanced features like full-text search)
- Reliable message sending and delivery is OF THE HIGHEST PRIORITY - must be ROCK-SOLID

## Early Stage

No data migration needed; breaking changes to APIs and storage schemas are acceptable.

## Public API Surface

- **GraphQL introspection and the `/api/playground` UI are intentionally enabled by default for everyone, including production deployments.** Letting people poke at Chatto's API is part of the product — it's how operators, integrators, and curious users learn the system. Do NOT propose gating either behind a "dev mode" flag, an admin-only toggle, env var, or build tag, even if a security review flags them as a "reconnaissance vector." A previous attempt to do this (PR #180, finding L1) was reverted on this branch. If a future security review raises this again, push back and reference this rule. Real protection for sensitive operations belongs in resolver-level authz, not in hiding the schema.

## Code Style & Approach

- **Prefer the simplest possible approach first.** Do not over-engineer solutions. If a fix can be done in 2-3 lines, do not create abstractions, wrappers, or complex architectures. Wait for user feedback before adding complexity.
- **When fixing bugs involving caches or state, prefer minimal, targeted invalidation** over clearing entire caches. Avoid full-page reload flashes or broad cache wipes. Only invalidate the specific stale data.
- **Functions that depend on "which instance" should require an explicit instance ID parameter.** Don't default to a global "current instance" — it creates coupling and timing bugs. Navigation helpers, storage functions, and state lookups all take `instanceId` as the first parameter.

## Verify Before Declaring Done

- **Run *relevant* tests before claiming a change works.** "Tests pass" only counts as verification if the tests you ran would actually fail when the change you made is wrong. A green pure-state unit test does not verify a component refactor; a green type-check does not verify runtime behavior; a green build does not verify a feature.
- **Pick the test layer that exercises what you changed.** If you touched a component's `$effect`, mount the component. If you touched cross-instance subscription wiring, drive a real subscription. If a layer that would catch your class of bug doesn't exist yet, write it — that's the test you owe the change. See `testing-frontend.md` "Match the test to the change."
- **`effect_update_depth_exceeded`, missing-context errors, hydration mismatches, and other Svelte-runtime failures only fire when a component is mounted.** Pure unit tests cannot reach them. After any refactor that changes how a component reads or writes store/context state, render the component in a test or in the browser before declaring done.
- **E2E tests run locally without any external stack.** Each Playwright test spawns its own `chatto` binary with embedded NATS on a random port (see `e2e/fixtures/server.ts`); they do NOT need `mise dev`, Docker, OrbStack, or a separate test database. Use `mise test-e2e` for the full suite, or `mise x -- pnpm exec playwright test <path>` for a subset. The Playwright `globalSetup` invokes `mise build-e2e-server` on every run, and mise's source/output tracking makes that a no-op when nothing has changed — so backend edits + e2e re-runs Just Work without a manual rebuild step.
- **When you can't run a particular test layer locally**, say so explicitly and ask the user to verify, rather than implying full coverage. Never write "verified" or "all tests pass" in a way that overstates what was actually exercised.
- **One green signal is not green.** `mise test-frontend` skipping e2e, type-check passing while runtime explodes, lint passing while semantics break — each is a partial signal. Acknowledge what's still uncovered.

## Lint, Type, and Vet Errors

- **Fix lint/type/vet errors at the source — do NOT silence them.** Disabling a rule (via `eslint-disable`, ESLint config overrides for specific files, `// @ts-expect-error`, `// @ts-ignore`, `//nolint`, etc.) is almost never the right answer. Find and fix the underlying issue.
- **Common anti-patterns to avoid:**
  - Adding `eslint-disable-next-line` to make a "stubborn" rule shut up. There is almost always a code restructuring that satisfies the rule properly.
  - Adding rule overrides in `eslint.config.js` to disable a rule for an entire directory without first proving the rule cannot be satisfied. Always investigate proper ways first.
  - Discarding errors with `_` in Go (`resp, _ := ...`) when `go vet` flags downstream uses. Check the error.
- **Verify your "fix" actually works.** A lint-passing change that breaks runtime behavior or another tool's contract is not a fix. Run the affected tests/build before declaring victory.
- **Examples of proper fixes (from real cases in this codebase):**
  - `svelte/no-navigation-without-resolve` on `goto(resolve(…) + '?highlight=' + id)`: pass the search string into `resolve()` itself — it accepts `RouteIdWithSearchOrHash`, e.g. `` resolve(`/path/[id]?q=${val}`, params) ``.
  - `httpresponse` vet warning on `resp, _ := client.Post(…)` followed by `defer resp.Body.Close()`: capture the error and `t.Fatalf` on it before the defer.
- **Examples where a *targeted* disable is the right call (with justification):**
  - `no-empty-pattern` on Playwright `async ({}, testInfo) => …`: Playwright parses the first parameter's source text to determine which fixtures the test needs and rejects a non-destructure Identifier (`_`) at runtime with "First argument must use the object destructuring pattern". The empty `{}` is the only way to declare a hook that takes `testInfo` without requesting fixtures. Handled via a scoped override in `eslint.config.js` for `e2e/**/*.ts`.
- **If a rule genuinely cannot be satisfied,** prefer a *scoped* config override over per-line disables, with a comment in the config explaining the runtime/tool constraint that forces the exception. Discuss with the user before adding either.

## UI & Frontend Style

- **Don't default to smaller font sizes.** Use the base text size unless there's a clear reason to go smaller (e.g., timestamps, metadata footnotes). Only use `text-xs` or `text-sm` when explicitly asked or when it's an established pattern in the codebase for that specific element type.
- **Keep text and labels aligned.** When elements appear above or below each other in a layout, they should share the same indentation. Use the same flex layout structure (spacers, gaps, padding) rather than calculating offsets manually. If a label sits above a content block, mirror the content block's layout so they line up naturally.
- **Always add `cursor-pointer` to clickable elements.** Buttons, toggles, and other interactive elements must have `cursor-pointer` in their class list. Tailwind CSS v4 does not add this automatically for buttons.
- **Never use `{@html}`.** It bypasses Svelte's XSS protection. Use snippets or components to compose rich content instead. Even for "safe" hardcoded strings — it sets a bad precedent and makes auditing harder.
- **Use `<SkeletonImg>` instead of `<img class="skeleton">`.** The `.skeleton` CSS utility adds a shimmer background that looks wrong behind transparent PNGs and should only show while loading. Use the `SkeletonImg` component (`$lib/ui/SkeletonImg.svelte`) which reactively applies `.skeleton` until `onload` fires. Never use imperative `classList.remove()` in a reactive framework — track state declaratively instead.
- **Wrap form inputs in a real `<form>` element.** Any panel with text inputs and a Save/submit button must use `<form onsubmit={handler}>` with `type="submit"` on the primary button — not `onclick` on a plain button. This is what gives users Enter-to-submit, native validation, browser autofill, and keyboard accessibility. Do `e.preventDefault()` in the handler to suppress native navigation. The settings page (`frontend/src/routes/chat/[instanceId]/settings/+page.svelte`) is a reference implementation.
- **Flex items with fixed-width content need `min-w-0` to shrink on mobile.** A flex item defaults to `min-width: auto`, which equals the content's intrinsic size. For images, iframes, video players, and other replaced elements — or any descendant with a hard pixel width — that intrinsic size silently overrides `max-width: 100%` and causes horizontal overflow on narrow viewports. If a flex item must be allowed to shrink below its content, add `min-w-0`. Common offenders: image attachments, video players, and link previews rendered inside `flex` / `flex-wrap` parents (see `MessageAttachments.svelte`).

## Planning

- **Gather broader context before finalizing scope** - Review related files and patterns across the codebase before committing to a task's boundaries
- **Start with most impactful/dependent tasks** - If task A is a prerequisite for task B, complete A first to unblock further work
- **Research before implementing** - Before suggesting optimizations or implementation approaches for NATS/JetStream, Go embed, or other infrastructure tools, read the actual documentation and SDK source first. Do not guess at API capabilities.

## iOS Safari Gotchas

- **`pointer-events: none` doesn't block touch-scroll.** It only suppresses click/tap events. iOS Safari routes touch-scroll gestures at the compositor level, bypassing `pointer-events`. When overlaying scrollable content (e.g., a slideover pane over a scrollable list), use the HTML `inert` attribute instead — it truly disables all interaction including scroll. (Safari 15.5+, Chrome 102+, Firefox 112+.)
- **Use `overscroll-y-contain` on isolated scroll containers.** Prevents scroll chaining — where scrolling past the edge of one container starts scrolling a parent or sibling. Good default for chat message lists and any panel that shouldn't leak scroll gestures.

## E2E Test Anti-Flakiness

- **Never use raw `waitForTimeout(number)`.** Always use a `TIMEOUTS.*` constant from `e2e/constants.ts`. This makes global tuning a single-line change and makes the intent clear.
- **Prefer observable state over fixed delays.** Instead of `waitForTimeout(500)`, use `expect(locator).toBeVisible()` or `toPass()` with polling intervals. Fixed delays don't adapt to CI slowness.
- **Use `toPass()` with exponential backoff for negative assertions.** When asserting something should NOT happen (e.g., no unread dot after a thread reply), use `toPass()` with `POLLING_INTERVALS` to give events time to propagate before asserting absence.
- **Use `waitForLoadState('networkidle')` for hydration waits.** Instead of guessing how long SvelteKit hydration takes with a fixed delay, wait for network activity to settle.
- **Scroll settling needs `TIMEOUTS.SCROLL_SETTLE` (150ms).** Between wheel events, virtua needs time to process measurements and scroll corrections. 50ms is too tight for CI; use the constant.
- **The only acceptable raw delay is for wall-clock timing tests** (e.g., cookie timestamp comparison that needs >1s to pass). These must have a comment explaining why.

## Refactoring

- **Keep refactoring PRs small and focused** - Don't let scope creep. If a refactor reveals additional cleanup opportunities, create separate tasks or GitHub issues for them rather than bundling everything together.
- **Verify regressions before fixing** - When a bug is reported during a refactor, first write a failing test that reproduces it, then investigate. Don't dive into code archaeology without a reproducible case.
- **Avoid over-engineering early** - Evaluate whether a complex abstraction provides sufficient value. If each use case has unique logic that differs significantly, simpler focused components may be better than a highly configurable wrapper.
