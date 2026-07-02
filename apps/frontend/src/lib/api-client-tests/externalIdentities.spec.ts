import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import {
  createExternalIdentityAPI,
  createExternalIdentityFlowAPI
} from '$lib/api-client/externalIdentities';
import { ExternalIdentityFlowKind } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  getPendingExternalIdentity: vi.fn(),
  createExternalIdentityAccount: vi.fn(),
  cancelExternalIdentityFlow: vi.fn(),
  confirmExternalIdentityLink: vi.fn(),
  listExternalIdentities: vi.fn(),
  startExternalIdentityLink: vi.fn(),
  linkExternalIdentity: vi.fn(),
  disconnectExternalIdentity: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createExternalIdentityFlowAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.getPendingExternalIdentity.mockReset();
    mocks.createExternalIdentityAccount.mockReset();
    mocks.cancelExternalIdentityFlow.mockReset();
    mocks.confirmExternalIdentityLink.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getPendingExternalIdentity: mocks.getPendingExternalIdentity,
      createExternalIdentityAccount: mocks.createExternalIdentityAccount,
      cancelExternalIdentityFlow: mocks.cancelExternalIdentityFlow,
      confirmExternalIdentityLink: mocks.confirmExternalIdentityLink
    });
  });

  it('maps pending external identity metadata', async () => {
    mocks.getPendingExternalIdentity.mockResolvedValue({
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

    const api = createExternalIdentityFlowAPI({
      baseUrl: 'https://origin.example.test/api/connect'
    });
    await expect(api.getPending('token-1')).resolves.toEqual({
      kind: ExternalIdentityFlowKind.CREATE_ACCOUNT,
      providerId: 'github-main',
      providerType: 'github',
      providerLabel: 'GitHub',
      verifiedEmail: null,
      loginHint: 'octo',
      displayNameHint: 'Octo',
      boundUserId: null,
      redirectPath: '/chat/-/settings/account'
    });
    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://origin.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.getPendingExternalIdentity).toHaveBeenCalledWith({ token: 'token-1' });
  });
});

describe('createExternalIdentityAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();
    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.listExternalIdentities.mockReset();
    mocks.startExternalIdentityLink.mockReset();
    mocks.linkExternalIdentity.mockReset();
    mocks.disconnectExternalIdentity.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listExternalIdentities: mocks.listExternalIdentities,
      startExternalIdentityLink: mocks.startExternalIdentityLink,
      linkExternalIdentity: mocks.linkExternalIdentity,
      disconnectExternalIdentity: mocks.disconnectExternalIdentity
    });
  });

  it('lists providers and resolves provider links against the server origin', async () => {
    mocks.listExternalIdentities.mockResolvedValue({
      providers: [
        {
          id: 'github-main',
          type: 'github',
          label: 'GitHub',
          loginUrl: '/auth/providers/github-main',
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

    const api = createExternalIdentityAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'token'
    });

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
          subjectHash: 'abc123'
        }
      ]
    });
    expect(mocks.listExternalIdentities).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('notifies the registry when authenticated calls are rejected', async () => {
    const err = new ConnectError('nope', Code.Unauthenticated);
    mocks.listExternalIdentities.mockRejectedValue(err);

    const api = createExternalIdentityAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'stale'
    });

    await expect(api.list()).rejects.toBe(err);
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });

  it('starts provider linking with bearer auth', async () => {
    mocks.startExternalIdentityLink.mockResolvedValue({
      startUrl: 'https://remote.example.test/auth/providers/github-main?intent=link&link_start=tok'
    });

    const api = createExternalIdentityAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'token'
    });

    await expect(
      api.startLink({ providerId: 'github-main', redirectPath: '/chat/-/settings/account' })
    ).resolves.toBe(
      'https://remote.example.test/auth/providers/github-main?intent=link&link_start=tok'
    );
    expect(mocks.startExternalIdentityLink).toHaveBeenCalledWith(
      { providerId: 'github-main', redirectPath: '/chat/-/settings/account' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('disconnects a linked identity with bearer auth', async () => {
    mocks.disconnectExternalIdentity.mockResolvedValue({ disconnected: true });

    const api = createExternalIdentityAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.disconnect('abc123', 'current-password')).resolves.toBeUndefined();
    expect(mocks.disconnectExternalIdentity).toHaveBeenCalledWith(
      { subjectHash: 'abc123', currentPassword: 'current-password' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });
});
