import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';

import {
  createPrivilegedModeAPI,
  getCurrentUserViaConnect,
  getViewerStateViaConnect
} from '$lib/api-client/viewer';
import { authenticationRequiredInterceptor } from '$lib/api-client/connect';
import { configureApiClientHooks } from '$lib/api-client/hooks';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getViewer: vi.fn(),
  activatePrivilegedMode: vi.fn(),
  deactivatePrivilegedMode: vi.fn()
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

describe('getCurrentUserViaConnect', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.getViewer.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.activatePrivilegedMode.mockReset();
    mocks.deactivatePrivilegedMode.mockReset();
    mocks.createClient.mockReturnValue({
      getViewer: mocks.getViewer,
      activatePrivilegedMode: mocks.activatePrivilegedMode,
      deactivatePrivilegedMode: mocks.deactivatePrivilegedMode
    });
  });

  it('loads current user state and maps protobuf fields', async () => {
    mocks.getViewer.mockResolvedValue({
      user: {
        profile: {
          id: 'U1',
          login: 'alice',
          displayName: 'Alice',
          avatarUrl: 'https://cdn/avatar.webp',
          timezone: 'Europe/Berlin',
          customStatus: {
            emoji: ':wave:',
            text: 'here',
            expiresAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z'))
          },
          presenceStatus: APIPresenceStatus.AWAY
        },
        hasVerifiedEmail: true,
        hasPassword: true,
        viewerCanDeleteAccount: true,
        lastLoginChange: Timestamp.fromDate(new Date('2026-05-20T09:30:00Z')),
        settings: {
          timezone: 'Europe/Berlin',
          timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
          shareTimezone: true
        }
      }
    });

    const user = await getCurrentUserViaConnect({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith(
      expect.objectContaining({
        baseUrl: 'https://chat.example.test/api/connect',
        useBinaryFormat: true
      })
    );
    expect(mocks.getViewer).toHaveBeenCalledWith(
      {},
      { contextValues: expect.anything(), timeoutMs: 10_000 }
    );
    expect(user).toEqual({
      id: 'U1',
      login: 'alice',
      displayName: 'Alice',
      isBot: false,
      deleted: false,
      avatarUrl: 'https://cdn/avatar.webp',
      bio: null,
      publicTimezone: 'Europe/Berlin',
      customStatus: {
        emoji: ':wave:',
        text: 'here',
        expiresAt: '2026-06-01T12:00:00.000Z'
      },
      presenceStatus: PresenceStatus.AWAY,
      hasVerifiedEmail: true,
      hasPassword: true,
      viewerCanDeleteAccount: true,
      lastLoginChange: '2026-05-20T09:30:00.000Z',
      settings: {
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
        shareTimezone: true
      }
    });
  });

  it('maps unspecified presence as offline', async () => {
    mocks.getViewer.mockResolvedValue({
      user: {
        profile: {
          id: 'U2',
          login: 'bob',
          displayName: 'Bob',
          presenceStatus: APIPresenceStatus.UNSPECIFIED
        },
        hasVerifiedEmail: false,
        settings: { timeFormat: TimeFormat.TIME_FORMAT_UNSPECIFIED }
      }
    });

    const user = await getCurrentUserViaConnect({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    expect(mocks.getViewer).toHaveBeenCalledWith(
      {},
      { contextValues: expect.anything(), timeoutMs: 10_000 }
    );
    expect(user.presenceStatus).toBe(PresenceStatus.OFFLINE);
    expect(user.settings?.timeFormat).toBe(TimeFormat.TIME_FORMAT_AUTO);
    expect(user.publicTimezone).toBeNull();
    expect(user.customStatus).toBeNull();
    expect(user.hasPassword).toBe(false);
    expect(user.viewerCanDeleteAccount).toBe(false);
    expect(user.lastLoginChange).toBeNull();
  });

  it('loads viewer capabilities', async () => {
    mocks.getViewer.mockResolvedValue({
      user: {
        profile: {
          id: 'U3',
          login: 'carol',
          displayName: 'Carol',
          presenceStatus: APIPresenceStatus.ONLINE
        },
        hasVerifiedEmail: true
      },
      capabilities: {
        grants: [
          { capability: 'admin.view', granted: true },
          { capability: 'dm.start', granted: true },
          { capability: 'admin.view-users', granted: true },
          { capability: 'user.manage-accounts', granted: true },
          { capability: 'role.assign', granted: true },
          { capability: 'role.view', granted: true },
          { capability: 'role.manage', granted: false },
          { capability: 'admin.view-system', granted: true },
          { capability: 'admin.view-audit', granted: true },
          { capability: 'user.manage-permissions', granted: true },
          { capability: 'user.invite', granted: true }
        ]
      }
    });

    const signal = new AbortController().signal;
    const viewer = await getViewerStateViaConnect(
      {
        baseUrl: '/api/connect',
        bearerToken: 'token'
      },
      { signal }
    );

    expect(mocks.getViewer).toHaveBeenCalledWith({}, { contextValues: expect.anything(), signal });

    expect(viewer).toEqual(
      expect.objectContaining({
        canViewAdmin: true,
        canStartDMs: true,
        canAdminViewUsers: true,
        canAdminManageAccounts: true,
        canAssignRoles: true,
        canAdminViewRoles: true,
        canAdminManageRoles: false,
        canAdminViewSystem: true,
        canAdminViewAudit: true,
        canManageUserPermissions: true,
        canManageInvites: true
      })
    );
  });

  it('leaves the reaction to a rejected viewer read to the caller', async () => {
    mocks.getViewer.mockResolvedValue({
      user: { profile: { id: 'U1', login: 'alice', displayName: 'Alice' } }
    });
    await getViewerStateViaConnect({
      serverId: 'origin',
      baseUrl: '/api/connect',
      bearerToken: null
    });
    const { contextValues } = mocks.getViewer.mock.calls[0][1];
    const onAuthenticationRequired = vi.fn();
    const err = new ConnectError('session expired', Code.Unauthenticated);
    configureApiClientHooks({ onAuthenticationRequired });
    try {
      const invoke = authenticationRequiredInterceptor({ serverId: 'origin' })(() =>
        Promise.reject(err)
      );
      await expect(invoke({ contextValues } as never)).rejects.toBe(err);
      expect(onAuthenticationRequired).not.toHaveBeenCalled();
    } finally {
      configureApiClientHooks({});
    }
  });

  it('activates and deactivates privileged mode through the viewer service', async () => {
    const active = { available: true, active: true };
    const inactive = { available: true, active: false };
    const activeCapabilities = { grants: [{ capability: 'admin.view-system', granted: true }] };
    const inactiveCapabilities = {
      grants: [{ capability: 'admin.view-system', granted: false }]
    };
    const viewerPermissions = { canManageInvites: true };
    const refreshedViewer = { privilegedMode: inactive, capabilities: inactiveCapabilities };
    mocks.activatePrivilegedMode.mockResolvedValue({
      privilegedMode: active,
      capabilities: activeCapabilities,
      viewerPermissions
    });
    mocks.deactivatePrivilegedMode.mockResolvedValue({
      privilegedMode: inactive,
      capabilities: inactiveCapabilities,
      viewerPermissions
    });
    mocks.getViewer.mockResolvedValue(refreshedViewer);
    const api = createPrivilegedModeAPI({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    await expect(api.activate()).resolves.toEqual({
      privilegedMode: active,
      capabilities: activeCapabilities,
      viewerPermissions
    });
    await expect(api.deactivate()).resolves.toEqual({
      privilegedMode: inactive,
      capabilities: inactiveCapabilities,
      viewerPermissions
    });
    await expect(api.refresh()).resolves.toBe(refreshedViewer);
    expect(mocks.activatePrivilegedMode).toHaveBeenCalledWith({});
    expect(mocks.deactivatePrivilegedMode).toHaveBeenCalledWith({});
    expect(mocks.getViewer).toHaveBeenCalledWith({});
  });
});
