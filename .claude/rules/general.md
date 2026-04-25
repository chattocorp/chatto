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

## Code Style & Approach

- **Prefer the simplest possible approach first.** Do not over-engineer solutions. If a fix can be done in 2-3 lines, do not create abstractions, wrappers, or complex architectures. Wait for user feedback before adding complexity.
- **When fixing bugs involving caches or state, prefer minimal, targeted invalidation** over clearing entire caches. Avoid full-page reload flashes or broad cache wipes. Only invalidate the specific stale data.
- **Functions that depend on "which instance" should require an explicit instance ID parameter.** Don't default to a global "current instance" — it creates coupling and timing bugs. Navigation helpers, storage functions, and state lookups all take `instanceId` as the first parameter.

## Lint, Type, and Vet Errors

- **Fix lint/type/vet errors at the source — do NOT silence them.** Disabling a rule (via `eslint-disable`, ESLint config overrides for specific files, `// @ts-expect-error`, `// @ts-ignore`, `//nolint`, etc.) is almost never the right answer. Find and fix the underlying issue.
- **Common anti-patterns to avoid:**
  - Adding `eslint-disable-next-line` to make a "stubborn" rule shut up. There is almost always a code restructuring that satisfies the rule properly.
  - Adding rule overrides in `eslint.config.js` to disable a rule for an entire directory because "it's a convention for that tool". Look for proper ways to satisfy the rule first (e.g., renaming an unused parameter to `_` instead of disabling `no-empty-pattern`).
  - Discarding errors with `_` in Go (`resp, _ := ...`) when `go vet` flags downstream uses. Check the error.
- **Examples of proper fixes (from real cases in this codebase):**
  - `no-empty-pattern` on Playwright `async ({}, testInfo) => …`: rename to `async (_, testInfo) => …` (the `argsIgnorePattern: '^_'` config allows it).
  - `svelte/no-navigation-without-resolve` on `goto(resolve(…) + '?highlight=' + id)`: pass the search string into `resolve()` itself — it accepts `RouteIdWithSearchOrHash`, e.g. `` resolve(`/path/[id]?q=${val}`, params) ``.
  - `httpresponse` vet warning on `resp, _ := client.Post(…)` followed by `defer resp.Body.Close()`: capture the error and `t.Fatalf` on it before the defer.
- **If a rule genuinely cannot be satisfied,** discuss with the user before adding any disable. Include a justification comment AND open a tracking issue. Do not silently disable.

## UI & Frontend Style

- **Don't default to smaller font sizes.** Use the base text size unless there's a clear reason to go smaller (e.g., timestamps, metadata footnotes). Only use `text-xs` or `text-sm` when explicitly asked or when it's an established pattern in the codebase for that specific element type.
- **Keep text and labels aligned.** When elements appear above or below each other in a layout, they should share the same indentation. Use the same flex layout structure (spacers, gaps, padding) rather than calculating offsets manually. If a label sits above a content block, mirror the content block's layout so they line up naturally.
- **Always add `cursor-pointer` to clickable elements.** Buttons, toggles, and other interactive elements must have `cursor-pointer` in their class list. Tailwind CSS v4 does not add this automatically for buttons.
- **Never use `{@html}`.** It bypasses Svelte's XSS protection. Use snippets or components to compose rich content instead. Even for "safe" hardcoded strings — it sets a bad precedent and makes auditing harder.
- **Use `<SkeletonImg>` instead of `<img class="skeleton">`.** The `.skeleton` CSS utility adds a shimmer background that looks wrong behind transparent PNGs and should only show while loading. Use the `SkeletonImg` component (`$lib/ui/SkeletonImg.svelte`) which reactively applies `.skeleton` until `onload` fires. Never use imperative `classList.remove()` in a reactive framework — track state declaratively instead.

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
