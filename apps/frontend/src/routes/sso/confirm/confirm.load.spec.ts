import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  getPending: vi.fn()
}));

vi.mock('$lib/api/externalIdentities', () => ({
  ExternalIdentityFlowKind: { CREATE_ACCOUNT: 1 },
  createExternalIdentityFlowAPI: () => ({ getPending: mocks.getPending })
}));

import { load } from './+page';

async function loadConfirmation(token?: string) {
  const url = new URL('https://chat.example.test/sso/confirm');
  if (token) url.searchParams.set('token', token);
  return load({ url } as never);
}

describe('SSO confirmation load', () => {
  beforeEach(() => {
    mocks.getPending.mockReset();
  });

  it('requires authentication while preserving the existing-account continuation', async () => {
    mocks.getPending.mockResolvedValue({ kind: 1 });
    const url = new URL('https://chat.example.test/sso/confirm?token=flow&existing=1');
    await expect(
      load({ url, parent: async () => ({ user: null }) } as never)
    ).rejects.toMatchObject({
      status: 302,
      location: '/login?redirect=%2Fsso%2Fconfirm%3Ftoken%3Dflow%26existing%3D1'
    });
  });

  it('offers linking only after an existing account authenticates', async () => {
    const pending = { kind: 1, providerId: 'discord-main' };
    mocks.getPending.mockResolvedValue(pending);
    const url = new URL('https://chat.example.test/sso/confirm?token=flow&existing=1');
    await expect(
      load({ url, parent: async () => ({ user: { id: 'account' } }) } as never)
    ).resolves.toEqual({ token: 'flow', pending, loadError: null, existingAccount: true });
  });

  it('does not redirect expired continuations to login', async () => {
    mocks.getPending.mockResolvedValue(null);
    const parent = vi.fn();
    const url = new URL('https://chat.example.test/sso/confirm?token=expired&existing=1');
    await expect(load({ url, parent } as never)).resolves.toMatchObject({ loadError: 'invalid' });
    expect(parent).not.toHaveBeenCalled();
  });

  it('does not request a pending flow without a token', async () => {
    await expect(loadConfirmation()).resolves.toEqual({
      token: '',
      pending: null,
      loadError: 'invalid'
    });
    expect(mocks.getPending).not.toHaveBeenCalled();
  });

  it('returns the pending flow for a valid token', async () => {
    const pending = {
      kind: 1,
      providerId: 'example',
      providerType: 'oidc',
      providerLabel: 'Example',
      verifiedEmail: null,
      loginHint: 'new-user',
      displayNameHint: 'New User',
      boundUserId: null,
      redirectPath: '/chat/-'
    };
    mocks.getPending.mockResolvedValue(pending);

    await expect(loadConfirmation('confirmation-token')).resolves.toEqual({
      token: 'confirmation-token',
      pending,
      loadError: null
    });
    expect(mocks.getPending).toHaveBeenCalledWith('confirmation-token');
  });

  it('maps a missing pending flow to the invalid state', async () => {
    mocks.getPending.mockResolvedValue(null);

    await expect(loadConfirmation('expired-token')).resolves.toEqual({
      token: 'expired-token',
      pending: null,
      loadError: 'invalid'
    });
  });

  it('sends an unadmitted new identity through the invitation gate', async () => {
    mocks.getPending.mockResolvedValue({ invitationRequired: true });
    await expect(loadConfirmation('verified-flow')).rejects.toMatchObject({
      status: 302,
      location: '/register?external_identity=verified-flow'
    });
  });

  it('maps a not-found API response to the invalid state', async () => {
    mocks.getPending.mockRejectedValue(new ConnectError('not found', Code.NotFound));

    await expect(loadConfirmation('missing-token')).resolves.toEqual({
      token: 'missing-token',
      pending: null,
      loadError: 'invalid'
    });
  });

  it('does not expose unexpected API error details', async () => {
    mocks.getPending.mockRejectedValue(new Error('sensitive backend detail'));

    await expect(loadConfirmation('failed-token')).resolves.toEqual({
      token: 'failed-token',
      pending: null,
      loadError: 'failed'
    });
  });

  it('identifies provider usernames that need operator action', async () => {
    mocks.getPending.mockRejectedValue(
      new ConnectError('unavailable username', Code.FailedPrecondition)
    );
    await expect(loadConfirmation('valid-token')).resolves.toEqual({
      token: 'valid-token',
      pending: null,
      loadError: 'provider_username'
    });
  });
});
