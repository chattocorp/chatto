import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  NotificationPolicyService,
  NotificationService
} from '@chatto/api-types/api/v1/notifications_connect';
import { NotificationOccurrence } from '@chatto/api-types/api/v1/notifications_pb';
import {
  createNotificationAPI,
  NotificationAttentionLevel,
  NotificationDeliveryMode,
  NotificationSignalKind,
  notificationOccurrence,
  notificationPolicyScopeKey
} from '../notifications.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const notifications = mockService(NotificationService);
const policies = mockService(NotificationPolicyService);

function notificationAPI() {
  return createNotificationAPI(
    fakeServer((router) =>
      router
        .service(NotificationService, notifications)
        .service(NotificationPolicyService, policies)
    )
  );
}

const PUSH = NotificationDeliveryMode.PUSH_NOTIFICATION;
const effective = {
  directMessages: PUSH,
  roomMessages: PUSH,
  directMentions: PUSH,
  replies: PUSH,
  roleMentions: PUSH,
  hereMentions: PUSH,
  allMentions: PUSH,
  followedThreads: PUSH,
  followedRooms: PUSH,
  reactions: PUSH
};

describe('createNotificationAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('lists occurrences, marks one read, and deletes all', async () => {
    notifications.listNotificationOccurrences.mockReturnValue({
      occurrences: [{ id: 'N1', unread: true }],
      unreadCount: 1
    });
    notifications.markNotificationRead.mockReturnValueOnce({
      occurrence: { id: 'N1', unread: false }
    });
    notifications.markNotificationRead.mockReturnValueOnce({});
    notifications.deleteAllNotificationOccurrences.mockReturnValue({ deletedCount: 4 });
    const api = notificationAPI();

    await expect(api.listNotificationOccurrences()).resolves.toMatchObject({
      occurrences: [{ id: 'N1', unread: true }],
      unreadCount: 1
    });
    expect(receivedRequest(notifications.listNotificationOccurrences)).toMatchObject({
      page: { limit: 50, offset: 0 }
    });
    await expect(api.markNotificationRead('N1')).resolves.toMatchObject({
      id: 'N1',
      unread: false
    });
    await expect(api.markNotificationRead('N1')).rejects.toThrow('was not returned');
    await expect(api.deleteAllNotificationOccurrences()).resolves.toBe(4);
  });

  it('reads the server policy and rejects a policy without effective modes', async () => {
    policies.getNotificationPolicy.mockReturnValueOnce({ policy: { policy: { effective } } });
    policies.getNotificationPolicy.mockReturnValueOnce({ policy: { policy: {} } });
    const api = notificationAPI();

    await expect(api.getNotificationPolicy()).resolves.toMatchObject({
      overrides: { directMessages: null, reactions: null },
      effective
    });
    expect(receivedRequest(policies.getNotificationPolicy)).toMatchObject({
      scope: { scope: { case: 'server' } }
    });
    await expect(api.getNotificationPolicy()).rejects.toThrow(
      'missing an effective direct_messages mode'
    );
  });

  it('updates a room policy and rejects an empty patch', async () => {
    policies.updateNotificationPolicy.mockReturnValue({ policy: { policy: { effective } } });
    const api = notificationAPI();

    await api.updateNotificationPolicy({ reactions: null, replies: PUSH }, 'R1');
    expect(receivedRequest(policies.updateNotificationPolicy)).toMatchObject({
      scope: { scope: { case: 'roomId', value: 'R1' } },
      overrides: { replies: PUSH },
      updateMask: { paths: ['replies', 'reactions'] }
    });
    await expect(api.updateNotificationPolicy({})).rejects.toThrow('update is empty');
    await expect(api.updateScopedNotificationPolicy({ kind: 'server' }, {})).rejects.toThrow(
      'update is empty'
    );
  });

  it('reads scoped policies and rejects answers without a policy or scope', async () => {
    policies.getNotificationPolicy.mockReturnValueOnce({
      policy: { scope: { scope: { case: 'server', value: {} } }, policy: { effective } }
    });
    policies.getNotificationPolicy.mockReturnValueOnce({});
    policies.getNotificationPolicy.mockReturnValueOnce({ policy: { policy: { effective } } });
    const api = notificationAPI();

    await expect(api.getScopedNotificationPolicy({ kind: 'server' })).resolves.toMatchObject({
      scope: { kind: 'server' }
    });
    await expect(api.getScopedNotificationPolicy({ kind: 'server' })).rejects.toThrow(
      'was not returned'
    );
    await expect(api.getScopedNotificationPolicy({ kind: 'server' })).rejects.toThrow(
      'scope is missing'
    );
  });
});

describe('notification occurrence mapping', () => {
  const message = { room: { id: 'R1', name: 'general' }, eventId: 'E1' };

  it.each([
    ['directMessageReceived', NotificationSignalKind.DIRECT_MESSAGE],
    ['replyReceived', NotificationSignalKind.REPLY],
    ['roleMentionReceived', NotificationSignalKind.ROLE_MENTION],
    ['hereMentionReceived', NotificationSignalKind.HERE],
    ['allMentionReceived', NotificationSignalKind.ALL]
  ] as const)('maps %s', (signal, kind) => {
    const item = notificationOccurrence(
      new NotificationOccurrence({
        id: 'N1',
        signal: { kind: { case: signal, value: { message } } }
      })
    );
    expect(item).toMatchObject({
      signalKind: kind,
      targetSupported: true,
      room: { id: 'R1', name: 'general' },
      eventId: 'E1',
      reactionEmoji: null
    });
  });

  it('maps a signal without a message and an occurrence without timestamps', () => {
    const item = notificationOccurrence(
      new NotificationOccurrence({
        id: 'N1',
        attentionLevel: NotificationAttentionLevel.AMBIENT,
        signal: { kind: { case: 'reactionReceived', value: {} } }
      })
    );
    expect(item).toMatchObject({
      signalKind: NotificationSignalKind.REACTION,
      room: null,
      eventId: '',
      threadRootId: null,
      reactionEmoji: null,
      actor: null,
      attentionLevel: NotificationAttentionLevel.AMBIENT,
      createdAt: new Date(0).toISOString(),
      expiresAt: new Date(0).toISOString()
    });

    const timed = notificationOccurrence(
      new NotificationOccurrence({
        id: 'N2',
        expiresAt: Timestamp.fromDate(new Date('2026-09-30T00:00:00Z')),
        actor: { id: 'U1', login: 'ada' }
      })
    );
    expect(timed).toMatchObject({
      expiresAt: '2026-09-30T00:00:00.000Z',
      actor: { id: 'U1', login: 'ada' }
    });
  });

  it('keys policy scopes', () => {
    expect(notificationPolicyScopeKey({ kind: 'server' })).toBe('server');
    expect(notificationPolicyScopeKey({ kind: 'room', id: 'R1' })).toBe('room:R1');
    expect(notificationPolicyScopeKey({ kind: 'roomGroup', id: 'G1' })).toBe('roomGroup:G1');
  });
});
