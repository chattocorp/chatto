/**
 * Shared test utilities for Vitest specs. Import from `$lib/test-utils`.
 *
 * Conventions:
 * - `q(container, sel)` — querySelector with `HTMLElement` cast for `expect.element()`
 * - `testSnippet(html)` — build a `Snippet` for component children/slot props
 *
 * See `apps/frontend/AGENTS.md` for the full convention.
 */
export { q } from './q';
export { testSnippet } from './snippet';
