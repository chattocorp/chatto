import { describe, expect, it } from 'vitest';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { RoomEventView } from '$lib/render/types';
import { roomReplyTargetEventId } from './messageReplyTarget';

function messageEvent(id: string, echoOfEventId: string | null = null): RoomEventView {
  return {
    id,
    createdAt: '2026-06-28T12:00:00Z',
    actorId: 'user-1',
    actor: null,
    event: {
      kind: RoomEventKind.MessagePosted,
      roomId: 'room-1',
      body: 'hello',
      attachments: [],
      linkPreview: null,
      reactions: [],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: null,
      echoOfEventId,
      echoFromThreadRootEventId: echoOfEventId ? 'thread-root' : null,
      channelEchoEventId: null,
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: false
    }
  };
}

describe('roomReplyTargetEventId', () => {
  it('uses the visible event id for regular messages', () => {
    expect(roomReplyTargetEventId(messageEvent('message-1'))).toBe('message-1');
  });

  it('uses the original reply id for channel echoes', () => {
    expect(roomReplyTargetEventId(messageEvent('echo-1', 'original-reply-1'))).toBe(
      'original-reply-1'
    );
  });
});
