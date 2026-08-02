import { beforeEach, describe, expect, it, vi } from 'vitest';

const { detachSyncedRegistrations } = vi.hoisted(() => ({
  detachSyncedRegistrations: vi.fn()
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    registrations: [],
    subscribeCatalog: vi.fn(),
    detachSyncedRegistrations
  }
}));

import { savePersistedAuthorization } from './persistedAuthorization';
import { AccountDataSync } from './sync.svelte';

beforeEach(() => {
  localStorage.clear();
  detachSyncedRegistrations.mockClear();
});

describe('AccountDataSync account boundary', () => {
  it('clears the Authling grant, TinyBase cache, and previous-account catalogue provenance', async () => {
    savePersistedAuthorization({
      issuer: 'https://id.example',
      clientId: 'https://app.example/oauth/client-metadata.json',
      accessToken: 'token',
      expiresAt: Date.now() + 60_000,
      accountId: 'account-1',
      providerLabel: 'Authling'
    });
    localStorage.setItem('chatto:account-data:tinybase', 'cached account data');
    const sync = new AccountDataSync();
    sync.session.establish({
      issuer: 'https://id.example',
      clientId: 'https://app.example/oauth/client-metadata.json',
      accessToken: 'token',
      expiresAt: Date.now() + 60_000,
      accountId: 'account-1',
      providerLabel: 'Authling'
    });

    await sync.signOut();

    expect(sync.status).toBe('disconnected');
    expect(sync.accountId).toBeNull();
    expect(localStorage.getItem('chatto:account-data:authorization')).toBeNull();
    expect(localStorage.getItem('chatto:account-data:tinybase')).toBeNull();
    expect(detachSyncedRegistrations).toHaveBeenCalledOnce();
  });
});
