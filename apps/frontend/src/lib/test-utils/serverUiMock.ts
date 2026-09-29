/**
 * A replacement for `$lib/state/server/serverUi` in `vi.mock` for specs whose
 * store mocks also carry the UI state, such as `voiceCall` or `navigation`:
 *
 * ```ts
 * vi.mock('$lib/state/server/serverUi', async () =>
 *   (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
 * );
 * ```
 *
 * `createTestServerScope` needs no mock: its fake store already serves as its
 * UI state.
 */
export const serverUiIsStore = {
  serverUi: (store: unknown) => store,
  setServerUiForTests: () => {}
};
