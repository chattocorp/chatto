import { beforeEach, describe, expect, it } from 'vitest';
import type { AccountDataAuthorization } from '$lib/accountData/authorization';
import { AuthlingSession } from './session.svelte';

const authorization: AccountDataAuthorization = {
  issuer: 'https://id.example',
  clientId: 'https://app.example/oauth/client-metadata.json',
  accessToken: 'token',
  expiresAt: Date.now() + 60_000,
  accountId: 'account-1',
  providerLabel: 'Authling'
};

beforeEach(() => {
  localStorage.clear();
});

describe('AuthlingSession', () => {
  it('owns connection state and restores its matching persisted grant', () => {
    const session = new AuthlingSession();
    session.beginConnecting();
    session.establish(authorization);

    expect(session.status).toBe('connected');
    expect(session.accountId).toBe('account-1');

    const restored = new AuthlingSession();
    expect(
      restored.restore({ issuer: authorization.issuer, clientId: authorization.clientId })
    ).toEqual(authorization);
    expect(restored.accountId).toBe('account-1');
  });

  it('clears account identity and the persisted grant at the account boundary', () => {
    const session = new AuthlingSession();
    session.establish(authorization);

    session.reset();

    expect(session.status).toBe('disconnected');
    expect(session.accountId).toBeNull();
    expect(session.providerLabel).toBeNull();
    expect(localStorage.getItem('chatto:account-data:authorization')).toBeNull();
  });
});
