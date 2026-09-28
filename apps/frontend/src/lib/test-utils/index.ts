/**
 * Shared test utilities for Vitest specs. Import from `$lib/test-utils`.
 *
 * Conventions:
 * - `q(container, sel)` — querySelector with `HTMLElement` cast for `expect.element()`
 * - `testSnippet(html)` — build a `Snippet` for component children/slot props
 * - `fakeServer(routes)` — API config backed by an in-memory fake Connect server
 * - `mockService(Service)` — typed `vi.fn` handlers for every method of a service
 * - `receivedRequest(handler)` / `receivedContext(handler)` — what a mock handler received
 *
 * See `apps/frontend/AGENTS.md` for the full convention.
 */
export { q } from './q';
export { testSnippet } from './snippet';
export {
  fakeServer,
  mockService,
  receivedContext,
  receivedRequest,
  type FakeServerRoutes,
  type MockService
} from './fakeServer';
