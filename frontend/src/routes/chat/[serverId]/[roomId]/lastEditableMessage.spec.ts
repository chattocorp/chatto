import { describe, expect, it } from 'vitest';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { RoomPermissions } from '$lib/state/room';
import { findLastEditableMessage } from './lastEditableMessage';

const nowMs = Date.parse('2026-06-15T12:00:00Z');
const editWindowSeconds = 3 * 60 * 60;

const canEchoRoomPermissions: RoomPermissions = {
  canPostMessage: true,
  canPostInThread: true,
  canAttach: true,
  canReact: true,
  canManageOthersMessage: false,
  canEchoMessage: true,
  canManageRoom: false,
  canBanRoomMembers: false
};

function makeMessageEvent(
  overrides: Partial<{
    id: string;
    actorId: string;
    createdAt: string;
    body: string | null;
    threadRootEventId: string | null;
    echoOfEventId: string | null;
    echoFromThreadRootEventId: string | null;
    channelEchoEventId: string | null;
  }> = {}
): RoomEventViewFragment {
  const actorId = overrides.actorId ?? 'user_self';
  return {
    id: overrides.id ?? 'evt_' + Math.random().toString(36).slice(2),
    createdAt: overrides.createdAt ?? '2026-06-15T11:00:00Z',
    actorId,
    actor: { id: actorId, login: 'tester', avatarUrl: null },
    event: {
      __typename: 'MessagePostedEvent',
      roomId: 'room_test',
      body: 'body' in overrides ? overrides.body : 'hello',
      attachments: [],
      linkPreview: null,
      reactions: [],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: overrides.threadRootEventId ?? null,
      echoOfEventId: overrides.echoOfEventId ?? null,
      echoFromThreadRootEventId: overrides.echoFromThreadRootEventId ?? null,
      channelEchoEventId: overrides.channelEchoEventId ?? null,
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: null
    }
  } as unknown as RoomEventViewFragment;
}

function find(events: RoomEventViewFragment[]) {
  return findLastEditableMessage({
    events,
    currentUserId: 'user_self',
    roomPermissions: canEchoRoomPermissions,
    messageEditWindowSeconds: editWindowSeconds,
    nowMs
  });
}

describe('findLastEditableMessage', () => {
  it('returns the latest own message within the edit window', () => {
    const result = find([
      makeMessageEvent({ id: 'evt_old', body: 'old body', createdAt: '2026-06-15T10:00:00Z' }),
      makeMessageEvent({ id: 'evt_new', body: 'new body', createdAt: '2026-06-15T11:30:00Z' })
    ]);

    expect(result).toMatchObject({
      eventId: 'evt_new',
      body: 'new body',
      threadRootEventId: null,
      channelEchoEventId: null,
      canAddChannelEcho: false
    });
  });

  it('returns null when the latest own message is exactly at the edit-window boundary', () => {
    const result = find([
      makeMessageEvent({
        id: 'evt_boundary',
        body: 'too late',
        createdAt: '2026-06-15T09:00:00Z'
      })
    ]);

    expect(result).toBeNull();
  });

  it('skips other users messages', () => {
    const result = find([
      makeMessageEvent({
        id: 'evt_other',
        actorId: 'user_other',
        body: 'not mine',
        createdAt: '2026-06-15T11:55:00Z'
      }),
      makeMessageEvent({
        id: 'evt_self',
        body: 'mine',
        createdAt: '2026-06-15T11:00:00Z'
      })
    ]);

    expect(result?.eventId).toBe('evt_self');
    expect(result?.body).toBe('mine');
  });

  it('skips deleted messages and falls back to an earlier editable message', () => {
    const result = find([
      makeMessageEvent({
        id: 'evt_editable',
        body: 'still editable',
        createdAt: '2026-06-15T11:00:00Z'
      }),
      makeMessageEvent({
        id: 'evt_deleted',
        body: null,
        createdAt: '2026-06-15T11:55:00Z'
      })
    ]);

    expect(result?.eventId).toBe('evt_editable');
    expect(result?.body).toBe('still editable');
  });

  it('preserves echo metadata for echoed thread replies', () => {
    const result = find([
      makeMessageEvent({
        id: 'evt_echo',
        body: 'echoed reply',
        createdAt: '2026-06-15T11:30:00Z',
        echoOfEventId: 'evt_original_reply',
        echoFromThreadRootEventId: 'evt_thread_root'
      })
    ]);

    expect(result).toMatchObject({
      eventId: 'evt_original_reply',
      body: 'echoed reply',
      threadRootEventId: 'evt_thread_root',
      channelEchoEventId: 'evt_echo',
      canAddChannelEcho: true
    });
  });
});
