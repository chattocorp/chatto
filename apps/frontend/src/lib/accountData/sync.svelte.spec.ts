import { beforeEach, describe, expect, it, vi } from 'vitest';

const { authorizeAccountData, detachSyncedRegistrations, getClientConfiguration } = vi.hoisted(
  () => ({
    authorizeAccountData: vi.fn(),
    detachSyncedRegistrations: vi.fn(),
    getClientConfiguration: vi.fn()
  })
);

vi.mock('$lib/clientConfig', () => ({ getClientConfiguration }));
vi.mock('./authorization', () => ({ authorizeAccountData }));

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
  getClientConfiguration.mockReset();
  getClientConfiguration.mockResolvedValue({ version: 1, authling: null });
  authorizeAccountData.mockReset();
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

  it('discards an unbound TinyBase cache before authorizing another account', async () => {
    localStorage.setItem('chatto:account-data:tinybase', 'previous account data');
    authorizeAccountData.mockRejectedValueOnce(new Error('authorization cancelled'));
    const sync = new AccountDataSync();

    await sync.connect();

    expect(authorizeAccountData).toHaveBeenCalledOnce();
    expect(localStorage.getItem('chatto:account-data:tinybase')).toBeNull();
    expect(detachSyncedRegistrations).toHaveBeenCalledOnce();
    expect(sync.status).toBe('error');
  });
});
