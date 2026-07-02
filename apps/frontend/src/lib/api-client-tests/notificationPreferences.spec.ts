import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import {
  getServerNotificationPreference,
  updateRoomNotificationPreference,
  updateServerNotificationPreference
} from '$lib/api-client/notificationPreferences';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  getServerNotificationPreference: vi.fn(),
  updateServerNotificationPreference: vi.fn(),
  updateRoomNotificationPreference: vi.fn()
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

describe('notificationPreferences API', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();

    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.getServerNotificationPreference.mockReset();
    mocks.updateServerNotificationPreference.mockReset();
    mocks.updateRoomNotificationPreference.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getServerNotificationPreference: mocks.getServerNotificationPreference,
      updateServerNotificationPreference: mocks.updateServerNotificationPreference,
      updateRoomNotificationPreference: mocks.updateRoomNotificationPreference
    });
  });

  it('gets and sets server notification preferences with bearer auth', async () => {
    mocks.getServerNotificationPreference.mockResolvedValue({
      level: NotificationLevel.NORMAL,
      effectiveLevel: NotificationLevel.NORMAL
    });
    mocks.updateServerNotificationPreference.mockResolvedValue({
      level: NotificationLevel.ALL_MESSAGES,
      effectiveLevel: NotificationLevel.ALL_MESSAGES
    });

    const config = {
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    };

    await expect(getServerNotificationPreference(config)).resolves.toEqual({
      level: NotificationLevel.NORMAL,
      effectiveLevel: NotificationLevel.NORMAL
    });
    await expect(
      updateServerNotificationPreference(config, NotificationLevel.ALL_MESSAGES)
    ).resolves.toEqual({
      level: NotificationLevel.ALL_MESSAGES,
      effectiveLevel: NotificationLevel.ALL_MESSAGES
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.getServerNotificationPreference).toHaveBeenCalledWith(
      {},
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(mocks.updateServerNotificationPreference).toHaveBeenCalledWith(
      { level: NotificationLevel.ALL_MESSAGES },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
  });

  it('sets room notification levels with bearer auth', async () => {
    mocks.updateRoomNotificationPreference.mockResolvedValue({
      level: NotificationLevel.MUTED,
      effectiveLevel: NotificationLevel.MUTED
    });

    const response = await updateRoomNotificationPreference(
      {
        serverId: 'remote',
        baseUrl: 'https://remote.example.test/api/connect',
        bearerToken: 'remote-token'
      },
      'room-1',
      NotificationLevel.MUTED
    );

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.updateRoomNotificationPreference).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        level: NotificationLevel.MUTED
      },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(response).toEqual({
      level: NotificationLevel.MUTED,
      effectiveLevel: NotificationLevel.MUTED
    });
  });

  it('marks the server authentication stale on unauthenticated Connect errors', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.updateRoomNotificationPreference.mockRejectedValue(err);

    await expect(
      updateRoomNotificationPreference(
        {
          serverId: 'remote',
          baseUrl: 'https://remote.example.test/api/connect',
          bearerToken: 'expired-token'
        },
        'room-1',
        NotificationLevel.MUTED
      )
    ).rejects.toBe(err);

    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });

  it('does not clear authentication for authorization failures', async () => {
    const err = new ConnectError('permission denied', Code.PermissionDenied);
    mocks.updateRoomNotificationPreference.mockRejectedValue(err);

    await expect(
      updateRoomNotificationPreference(
        {
          serverId: 'remote',
          baseUrl: 'https://remote.example.test/api/connect',
          bearerToken: 'remote-token'
        },
        'room-1',
        NotificationLevel.MUTED
      )
    ).rejects.toBe(err);

    expect(mocks.handleAuthenticationRequired).not.toHaveBeenCalled();
  });
});
