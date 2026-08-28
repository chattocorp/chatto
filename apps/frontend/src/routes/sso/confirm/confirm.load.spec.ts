import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  getPending: vi.fn()
}));

vi.mock('$lib/api-client/externalIdentities', () => ({
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
});
