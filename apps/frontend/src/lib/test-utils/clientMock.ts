/**
 * Default exports for a `vi.mock('$lib/client', …)` factory. A spec that
 * replaces some parts of the frontend client spreads these first, so the
 * mocked module still provides every export:
 *
 * ```ts
 * vi.mock('$lib/client', async () => ({
 *   ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
 *   serverRegistry: { getStore: () => store }
 * }));
 * ```
 *
 * The defaults do nothing. This module must not import `$lib/client`.
 */
export const clientMockDefaults = {
  client: {
    start: () => {},
    stop: () => {},
    setActiveServer: () => {}
  },
  serverRegistry: {},
  serverConnectionManager: {},
  eventBusManager: {
    getBus: () => undefined
  }
};
