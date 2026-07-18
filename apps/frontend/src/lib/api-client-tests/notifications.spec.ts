import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { PresenceStatus } from '$lib/api-client/renderTypes';
import { createNotificationAPI, NotificationItemKind } from '$lib/api-client/notifications';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listNotifications: vi.fn(),
  listRoomNotifications: vi.fn(),
  hasNotifications: vi.fn(),
  listRoomNotificationCounts: vi.fn(),
  dismissNotification: vi.fn(),
  dismissAllNotifications: vi.fn()
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

describe('createNotificationAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listNotifications.mockReset();
    mocks.listRoomNotifications.mockReset();
    mocks.hasNotifications.mockReset();
    mocks.listRoomNotificationCounts.mockReset();
    mocks.dismissNotification.mockReset();
    mocks.dismissAllNotifications.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listNotifications: mocks.listNotifications,
      listRoomNotifications: mocks.listRoomNotifications,
      hasNotifications: mocks.hasNotifications,
      listRoomNotificationCounts: mocks.listRoomNotificationCounts,
      dismissNotification: mocks.dismissNotification,
      dismissAllNotifications: mocks.dismissAllNotifications
    });
  });

  it('maps notification pages and sends bearer auth', async () => {
    mocks.listNotifications.mockResolvedValue({
      page: { totalCount: 2n, hasMore: true },
      notifications: [
        {
          id: 'n1',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
          actor: {
            id: 'u1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false,
            avatarUrl: 'https://cdn/avatar.webp',
            presenceStatus: APIPresenceStatus.OFFLINE
          },
          kind: {
            case: 'mention',
            value: {
              room: { id: 'room-1', name: 'general' },
              eventId: 'event-1',
              threadRootEventId: 'thread-1'
            }
          }
        }
      ]
    });

    const api = createNotificationAPI({
      baseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    });
    const page = await api.listNotifications(50);

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.com/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.listNotifications).toHaveBeenCalledWith(
      { page: { limit: 50, offset: 0 } },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(page).toEqual({
      totalCount: 2,
      hasMore: true,
      items: [
        {
          kind: NotificationItemKind.Mention,
          id: 'n1',
          createdAt: '2026-06-01T12:00:00.000Z',
          actor: {
            id: 'u1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false,
            avatarUrl: 'https://cdn/avatar.webp',
            presenceStatus: PresenceStatus.Offline,
            customStatus: null
          },
          summary: 'Alice mentioned you',
          mentionRoom: { id: 'room-1', name: 'general' },
          mentionEventId: 'event-1',
          mentionInThread: 'thread-1'
        }
      ]
    });
  });

  it('maps room notification reads and dismiss mutations without auth headers', async () => {
    mocks.listRoomNotifications.mockResolvedValue({
      page: { totalCount: 1n, hasMore: false },
      notifications: [
        {
          id: 'n2',
          kind: {
            case: 'directMessage',
            value: { room: { id: 'dm-1', name: 'Alice' }, eventId: 'event-2' }
          }
        }
      ]
    });
    mocks.hasNotifications.mockResolvedValue({ hasNotifications: true });
    mocks.listRoomNotificationCounts.mockResolvedValue({
      roomCounts: [
        { roomId: 'room-1', totalCount: 2 },
        { roomId: 'dm-1', totalCount: 1 }
      ]
    });
    mocks.dismissNotification.mockResolvedValue({ dismissed: true });
    mocks.dismissAllNotifications.mockResolvedValue({ dismissedCount: 3 });

    const api = createNotificationAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listRoomNotifications('dm-1')).resolves.toMatchObject({
      totalCount: 1,
      items: [
        {
          kind: NotificationItemKind.DirectMessage,
          room: { id: 'dm-1' }
        }
      ]
    });
    await expect(api.hasNotifications()).resolves.toBe(true);
    await expect(api.listNotificationCounts()).resolves.toEqual({ 'room-1': 2, 'dm-1': 1 });
    await expect(api.dismissNotification('n2')).resolves.toBe(true);
    await expect(api.dismissAllNotifications()).resolves.toBe(3);

    expect(mocks.listRoomNotifications).toHaveBeenCalledWith(
      { roomId: 'dm-1', page: { limit: 1, offset: 0 } },
      { headers: undefined }
    );
  });

});
