import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { DisconnectEligibility } from '@chatto/api-types/api/v1/external_identities_pb';
import { ExternalIdentityAuthService } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_connect';
import { createExternalIdentityAPI, createExternalIdentityFlowAPI } from './externalIdentities';
import { ExternalIdentityFlowKind } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';
import { UsernameChoiceAvailability } from '@chatto/client/api/actionAvailability';
import {
  fakeServer,
  mockService,
  receivedContext,
  receivedRequest
} from '@chatto/client/testing/fakeServer';

const flow = mockService(ExternalIdentityAuthService);
const account = mockService(MyAccountService);

function config() {
  return fakeServer(
    (router) =>
      router.service(ExternalIdentityAuthService, flow).service(MyAccountService, account),
    { baseUrl: 'https://remote.example.test/api/connect' }
  );
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe('createExternalIdentityFlowAPI', () => {
  it.each([
    undefined,
    UsernameChoiceAvailability.UNSPECIFIED,
    UsernameChoiceAvailability.ALLOWED,
    99 as UsernameChoiceAvailability
  ])('preserves optional username choice state %s', async (chooseUsername) => {
    flow.getPendingExternalIdentity.mockReturnValue({
      pending: { kind: ExternalIdentityFlowKind.CREATE_ACCOUNT, chooseUsername }
    });
    const pending = await createExternalIdentityFlowAPI(config()).getPending('token-1');
    expect(pending).not.toBeNull();
    expect(pending?.chooseUsername).toBe(chooseUsername);
  });

  it('maps pending external identity metadata', async () => {
    flow.getPendingExternalIdentity.mockReturnValue({
      pending: {
        kind: ExternalIdentityFlowKind.CREATE_ACCOUNT,
        providerId: 'github-main',
        providerType: 'github',
        providerLabel: 'GitHub',
        verifiedEmail: '',
        loginHint: 'octo',
        displayNameHint: 'Octo',
        boundUserId: '',
        redirectPath: '/chat/-/settings/account'
      }
    });

    const api = createExternalIdentityFlowAPI(config());
    await expect(api.getPending('token-1')).resolves.toEqual({
      kind: ExternalIdentityFlowKind.CREATE_ACCOUNT,
      providerId: 'github-main',
      providerType: 'github',
      providerLabel: 'GitHub',
      verifiedEmail: null,
      loginHint: 'octo',
      chooseUsername: undefined,
      displayNameHint: 'Octo',
      boundUserId: null,
      redirectPath: '/chat/-/settings/account',
      invitationRequired: false
    });
    expect(receivedRequest(flow.getPendingExternalIdentity)).toMatchObject({ token: 'token-1' });
  });

  it('sends the editable username and display name when creating an account', async () => {
    flow.createExternalIdentityAccount.mockReturnValue({
      userId: 'user-1',
      login: 'octo',
      token: 'session-token',
      refreshToken: 'refresh-token',
      expiresIn: 900n,
      refreshTokenExpiresIn: 7_776_000n
    });
    const api = createExternalIdentityFlowAPI(config());

    await expect(
      api.createAccount({ token: 'flow-token', login: 'octo', displayName: 'Octo Person' })
    ).resolves.toEqual({
      userId: 'user-1',
      login: 'octo'
    });
    expect(receivedRequest(flow.createExternalIdentityAccount)).toMatchObject({
      token: 'flow-token',
      login: 'octo',
      displayName: 'Octo Person'
    });
    expect(
      receivedContext(flow.createExternalIdentityAccount)?.requestHeader.get(
        'X-Chatto-Authentication-Mode'
      )
    ).toBe('cookie');
  });
});

describe('createExternalIdentityAPI', () => {
  it.each([
    DisconnectEligibility.ALLOWED,
    DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD,
    DisconnectEligibility.DISABLED_BY_OPERATOR,
    DisconnectEligibility.UNSPECIFIED,
    99
  ])('preserves disconnect eligibility: %s', async (disconnectEligibility) => {
    account.listExternalIdentities.mockReturnValue({
      linkedIdentities: [{ subjectHash: 'identity', disconnectEligibility }]
    });
    const result = await createExternalIdentityAPI(config()).list();
    expect(result.linkedIdentities[0]?.disconnectEligibility).toBe(disconnectEligibility);
  });

  it('lists providers and resolves provider links against the server origin', async () => {
    account.listExternalIdentities.mockReturnValue({
      providers: [
        {
          provider: {
            id: 'github-main',
            type: 'github',
            label: 'GitHub',
            loginUrl: '/auth/providers/github-main'
          },
          linkUrl: '/auth/providers/github-main?intent=link',
          linked: true,
          linkedIdentitySubjectHash: 'abc123'
        }
      ],
      linkedIdentities: [
        {
          providerId: 'github-main',
          providerType: 'github',
          providerLabel: 'GitHub',
          subjectHash: 'abc123'
        }
      ]
    });

    const api = createExternalIdentityAPI(config());

    await expect(api.list()).resolves.toEqual({
      providers: [
        {
          id: 'github-main',
          type: 'github',
          label: 'GitHub',
          loginUrl: 'https://remote.example.test/auth/providers/github-main',
          linkUrl: 'https://remote.example.test/auth/providers/github-main?intent=link',
          linked: true,
          linkedIdentitySubjectHash: 'abc123'
        }
      ],
      linkedIdentities: [
        {
          providerId: 'github-main',
          providerType: 'github',
          providerLabel: 'GitHub',
          subjectHash: 'abc123',
          disconnectEligibility: DisconnectEligibility.UNSPECIFIED
        }
      ]
    });
    expect(account.listExternalIdentities).toHaveBeenCalledOnce();
  });

  it('passes cancellation through when listing identities', async () => {
    await expect(
      createExternalIdentityAPI(config()).list({ signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('rejects provider rows without shared provider metadata', async () => {
    account.listExternalIdentities.mockReturnValue({
      providers: [{ linkUrl: '/auth/providers/github-main?intent=link' }],
      linkedIdentities: []
    });

    const api = createExternalIdentityAPI(config());

    await expect(api.list()).rejects.toThrow(
      'external identity provider response did not include provider metadata'
    );
  });

  it('propagates Connect errors', async () => {
    account.listExternalIdentities.mockImplementation(() => {
      throw new ConnectError('nope', Code.Unauthenticated);
    });

    await expect(createExternalIdentityAPI(config()).list()).rejects.toMatchObject({
      code: Code.Unauthenticated,
      rawMessage: 'nope'
    });
  });

  it('starts provider linking', async () => {
    account.startExternalIdentityLink.mockReturnValue({
      startUrl: 'https://remote.example.test/auth/providers/github-main?intent=link&link_start=tok'
    });

    const api = createExternalIdentityAPI(config());

    await expect(
      api.startLink({ providerId: 'github-main', redirectPath: '/chat/-/settings/account' })
    ).resolves.toBe(
      'https://remote.example.test/auth/providers/github-main?intent=link&link_start=tok'
    );
    expect(receivedRequest(account.startExternalIdentityLink)).toMatchObject({
      providerId: 'github-main',
      redirectPath: '/chat/-/settings/account'
    });
  });

  it('rejects an unsafe provider-link navigation URL from a remote server', async () => {
    account.startExternalIdentityLink.mockReturnValue({
      startUrl: 'javascript:alert(document.domain)'
    });

    const api = createExternalIdentityAPI(config());

    await expect(
      api.startLink({ providerId: 'github-main', redirectPath: '/chat/-/settings/account' })
    ).rejects.toThrow('External identity link returned an unsafe URL.');
  });

  it('disconnects a linked identity', async () => {
    account.disconnectExternalIdentity.mockReturnValue({});

    const api = createExternalIdentityAPI(config());

    await expect(api.disconnect('abc123', 'current-password')).resolves.toBeUndefined();
    expect(receivedRequest(account.disconnectExternalIdentity)).toMatchObject({
      subjectHash: 'abc123',
      currentPassword: 'current-password'
    });
  });
});
