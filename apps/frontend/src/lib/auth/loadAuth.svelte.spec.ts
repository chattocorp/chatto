import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  load: vi.fn(),
  clear: vi.fn(),
  store: { currentUser: { user: undefined as { id: string } | undefined } }
}));
import { loadCurrentUser as loadWith } from './loadAuth';
import type { ServerRegistry } from '@chatto/client/server/registry';

const registry = {
  originServer: { id: 'origin' },
  getStore: () => ({ currentUser: { load: mocks.load } }),
  tryGetStore: () => mocks.store,
  clearOriginAuthentication: mocks.clear
} as unknown as ServerRegistry;
const loadCurrentUser = () => loadWith(registry);
import {
  beginExplicitSignOutRedirect,
  cancelExplicitSignOutRedirect
} from '@chatto/client/auth/signOut';

describe('route account loading', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.load.mockResolvedValue(undefined);
    mocks.store = { currentUser: { user: undefined } };
    cancelExplicitSignOutRedirect();
  });

  it('reads the shared account, including data recovered without a route load', async () => {
    mocks.store.currentUser.user = { id: 'U1' };
    expect(await loadCurrentUser()).toEqual({ id: 'U1' });
    expect(mocks.load).toHaveBeenCalledOnce();
  });

  it('reads the replacement store after an account switch', async () => {
    mocks.store.currentUser.user = { id: 'U1' };
    mocks.load.mockImplementationOnce(async () => {
      mocks.store = { currentUser: { user: { id: 'U2' } } };
    });
    expect(await loadCurrentUser()).toEqual({ id: 'U2' });
  });

  it('has no route cache that can restore an account after the owner clears it', async () => {
    mocks.store.currentUser.user = { id: 'U1' };
    expect(await loadCurrentUser()).toEqual({ id: 'U1' });
    mocks.store.currentUser.user = undefined;
    expect(await loadCurrentUser()).toBeNull();
  });

  it('does not request an account during explicit sign-out', async () => {
    beginExplicitSignOutRedirect();
    try {
      expect(await loadCurrentUser()).toBeNull();
      expect(mocks.load).not.toHaveBeenCalled();
      expect(mocks.clear).toHaveBeenCalledOnce();
    } finally {
      cancelExplicitSignOutRedirect();
    }
  });

  it('does not return a retained account when sign-out starts during loading', async () => {
    mocks.store.currentUser.user = { id: 'U1' };
    mocks.load.mockImplementationOnce(async () => {
      beginExplicitSignOutRedirect();
    });
    try {
      expect(await loadCurrentUser()).toBeNull();
      expect(mocks.clear).toHaveBeenCalledOnce();
    } finally {
      cancelExplicitSignOutRedirect();
    }
  });
});
