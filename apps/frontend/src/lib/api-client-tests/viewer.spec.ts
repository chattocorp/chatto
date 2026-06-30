import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NotificationLevel as APINotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat as APITimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { NotificationLevel, PresenceStatus, TimeFormat } from '@chatto/api-client/renderTypes';
import { getCurrentUserViaConnect, getViewerStateViaConnect } from '@chatto/api-client/viewer';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getViewer: vi.fn()
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
    mocks.createClient.mockReturnValue({ getViewer: mocks.getViewer });
  });

  it('loads current user state with bearer auth and maps protobuf fields', async () => {
    mocks.getViewer.mockResolvedValue({
      user: {
        profile: {
          user: {
            id: 'U1',
            login: 'alice',
            displayName: 'Alice',
            avatarUrl: 'https://cdn/avatar.webp'
          },
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
          timeFormat: APITimeFormat.TIME_FORMAT_24_HOUR
        }
      },
      roomNotificationPreferences: []
    });

    const user = await getCurrentUserViaConnect({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.getViewer).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(user).toEqual({
      id: 'U1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: 'https://cdn/avatar.webp',
      customStatus: {
        emoji: ':wave:',
        text: 'here',
        expiresAt: '2026-06-01T12:00:00.000Z'
      },
      presenceStatus: PresenceStatus.Away,
      hasVerifiedEmail: true,
      hasPassword: true,
      viewerCanDeleteAccount: true,
      lastLoginChange: '2026-05-20T09:30:00.000Z',
      settings: {
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TwentyFourHour
      }
    });
  });

  it('omits auth headers and maps unspecified presence as offline', async () => {
    mocks.getViewer.mockResolvedValue({
      user: {
        profile: {
          user: {
            id: 'U2',
            login: 'bob',
            displayName: 'Bob'
          },
          presenceStatus: APIPresenceStatus.UNSPECIFIED
        },
        hasVerifiedEmail: false,
        settings: { timeFormat: APITimeFormat.TIME_FORMAT_UNSPECIFIED }
      },
      roomNotificationPreferences: []
    });

    const user = await getCurrentUserViaConnect({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    expect(mocks.getViewer).toHaveBeenCalledWith({}, { headers: undefined });
    expect(user.presenceStatus).toBe(PresenceStatus.Offline);
    expect(user.settings?.timeFormat).toBe(TimeFormat.Auto);
    expect(user.customStatus).toBeNull();
    expect(user.hasPassword).toBe(false);
    expect(user.viewerCanDeleteAccount).toBe(false);
    expect(user.lastLoginChange).toBeNull();
  });

  it('loads viewer capabilities and notification preferences', async () => {
    mocks.getViewer.mockResolvedValue({
      user: {
        profile: {
          user: {
            id: 'U3',
            login: 'carol',
            displayName: 'Carol'
          },
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
          { capability: 'user.manage-permissions', granted: true }
        ]
      },
      serverNotificationPreference: {
        level: APINotificationLevel.ALL_MESSAGES,
        effectiveLevel: APINotificationLevel.ALL_MESSAGES
      },
      roomNotificationPreferences: [
        {
          roomId: 'room-1',
          level: APINotificationLevel.MUTED,
          effectiveLevel: APINotificationLevel.MUTED
        },
        {
          roomId: 'room-2',
          level: APINotificationLevel.DEFAULT,
          effectiveLevel: APINotificationLevel.NORMAL
        }
      ]
    });

    const viewer = await getViewerStateViaConnect({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

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
        serverNotificationPreference: {
          level: NotificationLevel.AllMessages,
          effectiveLevel: NotificationLevel.AllMessages
        },
        roomNotificationPreferences: [
          {
            roomId: 'room-1',
            level: NotificationLevel.Muted,
            effectiveLevel: NotificationLevel.Muted
          },
          {
            roomId: 'room-2',
            level: NotificationLevel.Default,
            effectiveLevel: NotificationLevel.Normal
          }
        ]
      })
    );
  });
});
