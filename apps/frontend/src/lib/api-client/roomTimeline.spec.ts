import { MessageSchema } from '@chatto/api-types/api/v1/message_types_pb';
import {
  LinkPreviewSchema,
  SocialPostAuthorSchema,
  SocialPostPreviewSchema
} from '@chatto/api-types/api/v1/link_previews_pb';
import { describe, expect, it } from 'vitest';
import {
  RoomTimelineEventSchema,
  RoomTimelineThreadingModeChangedEventSchema
} from '@chatto/api-types/api/v1/room_timeline_pb';
import { RoomThreadingMode } from '$lib/roomThreading';
import { UserSchema } from '@chatto/api-types/api/v1/users_pb';
import { TimelineEventKind } from '$lib/render/timelineEvents';
import {
  messagePostedPayload,
  messageToTimelineEvent,
  roomTimelineEventToView
} from './roomTimeline';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { create } from '@bufbuild/protobuf';

describe('roomTimelineEventToView', () => {
  it('keeps a missing author unresolved and preserves an explicit deleted reference', () => {
    const message = create(MessageSchema, { id: 'message-1', actorId: 'author', roomId: 'room-1' });

    expect(messageToTimelineEvent(message, {})?.actor).toBeNull();
    expect(
      messageToTimelineEvent(message, {
        author: create(UserSchema, { id: 'author', deleted: true })
      })?.actor?.deleted
    ).toBe(true);
  });

  it('maps a Threading Mode change into a renderable system event', () => {
    const event = create(RoomTimelineEventSchema, {
      id: 'mode-change',
      actorId: 'admin',
      event: {
        case: 'roomThreadingModeChanged',
        value: create(RoomTimelineThreadingModeChangedEventSchema, {
          roomId: 'room-1',
          threadingMode: RoomThreadingMode.ENCOURAGED
        })
      }
    });

    expect(roomTimelineEventToView(event, {})).toMatchObject({
      id: 'mode-change',
      actorId: 'admin',
      event: {
        kind: TimelineEventKind.RoomThreadingModeChanged,
        roomId: 'room-1',
        threadingMode: RoomThreadingMode.ENCOURAGED
      }
    });
  });
});

describe('messagePostedPayload', () => {
  it('preserves resolved reply authority and distinguishes absent authority', () => {
    for (const canReplyInThread of [true, false, undefined]) {
      expect(
        messagePostedPayload(create(MessageSchema, { viewerState: { canReplyInThread } }), {})
          .canReplyInThread
      ).toBe(canReplyInThread);
    }
    expect(messagePostedPayload(create(MessageSchema), {}).canReplyInThread).toBeUndefined();
  });
  it('maps current pin state', () => {
    expect(messagePostedPayload(create(MessageSchema, { pinned: true }), {}).pinned).toBe(true);
  });

  it('preserves an explicitly created empty thread', () => {
    const message = create(MessageSchema, { thread: { replyCount: 0 } });

    expect(messagePostedPayload(message, {})).toMatchObject({
      threadExists: true,
      replyCount: 0
    });
  });

  it('maps deleted_at to the exact ISO timestamp', () => {
    const deletedAt = timestampFromDate(new Date('2026-07-10T10:11:12.345Z'));

    expect(messagePostedPayload(create(MessageSchema, { deletedAt }), {}).deletedAt).toBe(
      '2026-07-10T10:11:12.345Z'
    );
  });

  it('keeps deletedAt null when the server omits the metadata', () => {
    expect(messagePostedPayload(create(MessageSchema), {}).deletedAt).toBeNull();
  });

  it('maps one quoted social post', () => {
    const message = create(MessageSchema, {
      linkPreview: create(LinkPreviewSchema, {
        url: 'https://bsky.app/profile/outer.example/post/outer',
        socialPost: create(SocialPostPreviewSchema, {
          provider: 'bluesky',
          author: create(SocialPostAuthorSchema, { handle: 'outer.example' }),
          text: 'Outer words.',
          quotedPost: create(SocialPostPreviewSchema, {
            provider: 'bluesky',
            url: 'https://bsky.app/profile/quoted.example/post/quoted',
            author: create(SocialPostAuthorSchema, { handle: 'quoted.example' }),
            text: 'Quoted words.'
          })
        })
      })
    });

    expect(messagePostedPayload(message, {}).linkPreview?.socialPost?.quotedPost).toMatchObject({
      provider: 'bluesky',
      url: 'https://bsky.app/profile/quoted.example/post/quoted',
      text: 'Quoted words.'
    });
  });
});
