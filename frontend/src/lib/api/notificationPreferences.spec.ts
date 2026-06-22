import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NotificationLevel } from '$lib/pb/chatto/api/v1/notification_preferences_pb';
import { setRoomNotificationLevel } from './notificationPreferences';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
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

describe('setRoomNotificationLevel', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();
    mocks.setRoomNotificationLevel.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      setRoomNotificationLevel: mocks.setRoomNotificationLevel
    });
  });

  it('sends binary ConnectRPC requests with bearer auth', async () => {
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
