import { describe, it, expect } from 'vitest';

/**
 * CurrentUserState class structure tests.
 *
 * The class itself depends on Svelte 5 reactive state and is exercised
 * end-to-end through `ServerStateStore`, which constructs one per
 * registered server. Component-level tests should mount real components
 * and read the state via `serverRegistry.getStore(id).currentUser` rather
 * than instantiating `CurrentUserState` directly.
 */
describe('CurrentUserState', () => {
  it('exports the class', async () => {
    const module = await import('./currentUser.svelte');
    expect(module.CurrentUserState).toBeDefined();
    expect(typeof module.CurrentUserState).toBe('function');
  });
});
