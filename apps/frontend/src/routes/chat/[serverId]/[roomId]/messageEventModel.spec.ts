import { describe, expect, it } from 'vitest';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimelineEventKind, type MessagePostedPayload } from '$lib/render/timelineEvents';
import {
  buildMessageReplyPreview,
  canEditMessage,
  embeddedMessageLinks,
  isDeletedMessage,
  resolveMessageEventReferences
} from './messageEventModel';

function message(overrides: Partial<MessagePostedPayload> = {}): MessagePostedPayload {
  return {
    kind: TimelineEventKind.MessagePosted,
    roomId: 'room-1',
    body: 'Hello',
    attachments: [],
    reactions: [],
    replyCount: 0,
    threadParticipants: [],
    ...overrides
  };
}

describe('resolveMessageEventReferences', () => {
  it('uses the visible event for normal room messages', () => {
    expect(
      resolveMessageEventReferences(
        'event-1',
        message({ threadRootEventId: 'thread-1', channelEchoEventId: 'echo-1' })
      )
    ).toEqual({
      isEcho: false,
      editEventId: 'event-1',
      editThreadRootEventId: 'thread-1',
      editChannelEchoEventId: 'echo-1',
      threadRootEventId: 'event-1'
    });
  });

  it('maps channel echoes back to their source thread message', () => {
    expect(
      resolveMessageEventReferences(
        'echo-1',
        message({
          echoOfEventId: 'thread-message-1',
          echoFromThreadRootEventId: 'thread-1'
        })
      )
    ).toEqual({
      isEcho: true,
      editEventId: 'thread-message-1',
      editThreadRootEventId: 'thread-1',
      editChannelEchoEventId: 'echo-1',
      threadRootEventId: 'thread-1'
    });
  });
});

describe('canEditMessage', () => {
  it('allows authors inside the edit window and moderators outside it', () => {
    const createdAt = '2026-07-29T12:00:00.000Z';
    const now = new Date('2026-07-29T12:05:00.000Z').getTime();

    expect(
      canEditMessage({
        isAuthor: true,
        createdAt,
        now,
        editWindowSeconds: 600,
        canManageOthersMessage: false
      })
    ).toBe(true);
    expect(
      canEditMessage({
        isAuthor: true,
        createdAt,
        now,
        editWindowSeconds: 60,
        canManageOthersMessage: false
      })
    ).toBe(false);
    expect(
      canEditMessage({
        isAuthor: false,
        createdAt,
        now,
        editWindowSeconds: 60,
        canManageOthersMessage: true
      })
    ).toBe(true);
  });
});

describe('embeddedMessageLinks', () => {
  it('keeps only the first five Chatto message links', () => {
    const body = Array.from(
      { length: 7 },
      (_, index) => `https://chat.example.test/chat/server/room/m/message-${index}`
    ).join(' ');

    expect(embeddedMessageLinks(body).map((link) => link.messageId)).toEqual([
      'message-0',
      'message-1',
      'message-2',
      'message-3',
      'message-4'
    ]);
  });
});

describe('isDeletedMessage', () => {
  it('keeps attachment-only messages visible and tombstones empty messages', () => {
    expect(isDeletedMessage(message({ body: null }))).toBe(true);
    expect(
      isDeletedMessage(
        message({
          body: null,
          attachments: [{ id: 'attachment-1' } as MessagePostedPayload['attachments'][number]]
        })
      )
    ).toBe(false);
  });
});

describe('buildMessageReplyPreview', () => {
  it('distinguishes missing, active, and deleted reply targets', () => {
    expect(
      buildMessageReplyPreview({
        target: undefined,
        missingName: 'a message',
        deletedName: 'Deleted user',
        getDisplayName: (actor) => actor.displayName
      })
    ).toEqual({ name: 'a message', body: null, actor: null, deleted: false });

    const activeActor = {
      id: 'user-1',
      login: 'ada',
      displayName: 'Ada',
      avatarUrl: '',
      deleted: false,
      presenceStatus: PresenceStatus.ONLINE
    };
    expect(
      buildMessageReplyPreview({
        target: {
          id: 'reply-1',
          createdAt: '2026-07-29T12:00:00.000Z',
          actor: activeActor,
          event: message({ body: 'Original' })
        },
        missingName: 'a message',
        deletedName: 'Deleted user',
        getDisplayName: (actor) => actor.displayName
      })
    ).toEqual({ name: 'Ada', body: 'Original', actor: activeActor, deleted: false });

    expect(
      buildMessageReplyPreview({
        target: {
          id: 'reply-2',
          createdAt: '2026-07-29T12:00:00.000Z',
          actor: { ...activeActor, deleted: true },
          event: message({ body: null })
        },
        missingName: 'a message',
        deletedName: 'Deleted user',
        getDisplayName: (actor) => actor.displayName
      })
    ).toEqual({ name: 'Deleted user', body: null, actor: null, deleted: true });
  });
});
