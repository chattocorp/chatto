import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';

import {
  createPrivilegedModeAPI,
  getCurrentUserViaConnect,
  getViewerStateViaConnect
} from '$lib/api-client/viewer';
import type { ConnectAPIConfig } from '$lib/api-client/connect';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { fakeServer, mockService, receivedContext } from '$lib/test-utils';

const mocks = mockService(ViewerService);

function config(extra: Partial<ConnectAPIConfig> = {}) {
  return fakeServer((router) => router.service(ViewerService, mocks), extra);
}

describe('getCurrentUserViaConnect', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('loads current user state and maps protobuf fields', async () => {
    mocks.getViewer.mockReturnValue({
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

    const user = await getCurrentUserViaConnect(config());

    expect(receivedContext(mocks.getViewer)?.timeoutMs()).toBeGreaterThan(9_000);
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
    mocks.getViewer.mockReturnValue({
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

    const user = await getCurrentUserViaConnect(config());

    expect(user.presenceStatus).toBe(PresenceStatus.OFFLINE);
    expect(user.settings?.timeFormat).toBe(TimeFormat.TIME_FORMAT_AUTO);
    expect(user.publicTimezone).toBeNull();
    expect(user.customStatus).toBeNull();
    expect(user.hasPassword).toBe(false);
    expect(user.viewerCanDeleteAccount).toBe(false);
    expect(user.lastLoginChange).toBeNull();
  });

  it('loads viewer capabilities', async () => {
    mocks.getViewer.mockReturnValue({
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

    const viewer = await getViewerStateViaConnect(config());

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
    mocks.getViewer.mockImplementation(() => {
      throw new ConnectError('session expired', Code.Unauthenticated);
    });
    const onAuthenticationRequired = vi.fn();
    configureApiClientHooks({ onAuthenticationRequired });
    try {
      await expect(getViewerStateViaConnect(config({ serverId: 'origin' }))).rejects.toMatchObject({
        code: Code.Unauthenticated
      });
      expect(onAuthenticationRequired).not.toHaveBeenCalled();
    } finally {
      configureApiClientHooks({});
    }
  });

  it('cancels a viewer read with the caller signal', async () => {
    await expect(
      getViewerStateViaConnect(config(), { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('activates and deactivates privileged mode through the viewer service', async () => {
    const active = { available: true, active: true };
    const inactive = { available: true, active: false };
    const activeCapabilities = { grants: [{ capability: 'admin.view-system', granted: true }] };
    const inactiveCapabilities = {
      grants: [{ capability: 'admin.view-system', granted: false }]
    };
    const viewerPermissions = { permissions: [{ permission: 'invite.create', granted: true }] };
    const refreshedViewer = { privilegedMode: inactive, capabilities: inactiveCapabilities };
    mocks.activatePrivilegedMode.mockReturnValue({
      privilegedMode: active,
      capabilities: activeCapabilities,
      viewerPermissions
    });
    mocks.deactivatePrivilegedMode.mockReturnValue({
      privilegedMode: inactive,
      capabilities: inactiveCapabilities,
      viewerPermissions
    });
    mocks.getViewer.mockReturnValue(refreshedViewer);
    const api = createPrivilegedModeAPI(config());

    await expect(api.activate()).resolves.toMatchObject({
      privilegedMode: active,
      capabilities: activeCapabilities,
      viewerPermissions
    });
    await expect(api.deactivate()).resolves.toMatchObject({
      privilegedMode: inactive,
      capabilities: inactiveCapabilities,
      viewerPermissions
    });
    await expect(api.refresh()).resolves.toMatchObject(refreshedViewer);
    expect(mocks.activatePrivilegedMode).toHaveBeenCalledOnce();
    expect(mocks.deactivatePrivilegedMode).toHaveBeenCalledOnce();
    expect(mocks.getViewer).toHaveBeenCalledOnce();
  });
});
