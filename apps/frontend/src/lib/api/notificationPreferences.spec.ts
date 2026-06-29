import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
import {
  getServerNotificationPreference,
  setRoomNotificationLevel,
  setServerNotificationLevel
} from './notificationPreferences';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  getServerNotificationPreference: vi.fn(),
  setServerNotificationLevel: vi.fn(),
  setRoomNotificationLevel: vi.fn()
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

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    handleAuthenticationRequired: mocks.handleAuthenticationRequired
  }
}));

describe('notificationPreferences API', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();
    mocks.getServerNotificationPreference.mockReset();
    mocks.setServerNotificationLevel.mockReset();
    mocks.setRoomNotificationLevel.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getServerNotificationPreference: mocks.getServerNotificationPreference,
      setServerNotificationLevel: mocks.setServerNotificationLevel,
      setRoomNotificationLevel: mocks.setRoomNotificationLevel
    });
  });

  it('gets and sets server notification preferences with bearer auth', async () => {
    mocks.getServerNotificationPreference.mockResolvedValue({
      level: NotificationLevel.NORMAL,
      effectiveLevel: NotificationLevel.NORMAL
    });
    mocks.setServerNotificationLevel.mockResolvedValue({
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
      setServerNotificationLevel(config, NotificationLevel.ALL_MESSAGES)
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
    expect(mocks.setServerNotificationLevel).toHaveBeenCalledWith(
      { level: NotificationLevel.ALL_MESSAGES },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
  });

  it('sets room notification levels with bearer auth', async () => {
    mocks.setRoomNotificationLevel.mockResolvedValue({
      level: NotificationLevel.MUTED,
      effectiveLevel: NotificationLevel.MUTED
    });

    const response = await setRoomNotificationLevel(
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
    expect(mocks.setRoomNotificationLevel).toHaveBeenCalledWith(
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
    mocks.setRoomNotificationLevel.mockRejectedValue(err);

    await expect(
      setRoomNotificationLevel(
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
    mocks.setRoomNotificationLevel.mockRejectedValue(err);

    await expect(
      setRoomNotificationLevel(
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
