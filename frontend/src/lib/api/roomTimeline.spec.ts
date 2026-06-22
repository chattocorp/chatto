import { describe, expect, it } from 'vitest';
import { Timestamp } from '@bufbuild/protobuf';
import {
  RoomTimelineAssetUrl,
  RoomTimelineAttachment,
  RoomTimelineEvent,
  RoomTimelineMessagePosted,
  RoomTimelinePage,
  RoomTimelineRoomEvent,
  RoomTimelineUser,
  RoomTimelineVideoProcessing,
  RoomTimelineVideoProcessingStatus,
  RoomTimelineVideoVariant
} from '$lib/pb/chatto/api/v1/room_timeline_pb';
import { roomTimelinePageToEventConnectionPage } from './roomTimeline';

describe('roomTimelinePageToEventConnectionPage', () => {
  it('maps hydrated protobuf room timeline pages into the message render shape', () => {
    const page = new RoomTimelinePage({
      startCursor: 'seq:10',
      endCursor: 'seq:11',
      hasOlder: true,
      hasNewer: false,
      includes: {
        users: {
          u1: new RoomTimelineUser({
            id: 'u1',
            login: 'alice',
            displayName: 'Alice',
            avatarUrl: '/avatars/u1',
            deleted: false
          }),
          u2: new RoomTimelineUser({
            id: 'u2',
            login: 'bob',
            displayName: 'Bob',
            deleted: false
          })
        }
      },
      events: [
        new RoomTimelineEvent({
          id: 'm1',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
          actorId: 'u1',
          event: {
            case: 'messagePosted',
            value: new RoomTimelineMessagePosted({
              roomId: 'room-1',
              body: 'hello',
              bodyPresent: true,
              attachments: [
                new RoomTimelineAttachment({
                  id: 'a-video',
                  filename: 'clip.mp4',
                  contentType: 'video/mp4',
                  width: 1280,
                  height: 720,
                  assetUrl: new RoomTimelineAssetUrl({
                    url: '/assets/files/a-video',
                    expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                  }),
                  thumbnailAssetUrl: new RoomTimelineAssetUrl({
                    url: '/assets/files/a-video/image/960x800/contain',
                    expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                  }),
                  videoProcessing: new RoomTimelineVideoProcessing({
                    status: RoomTimelineVideoProcessingStatus.COMPLETED,
                    durationMs: 1234n,
                    width: 1280,
                    height: 720,
                    sourceAvailable: true,
                    thumbnailAssetUrl: new RoomTimelineAssetUrl({
                      url: '/assets/files/a-thumb',
                      expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                    }),
                    variants: [
                      new RoomTimelineVideoVariant({
                        quality: '720p',
                        width: 1280,
                        height: 720,
                        size: 4567n,
                        assetUrl: new RoomTimelineAssetUrl({
                          url: '/assets/files/a-variant',
                          expiresAt: Timestamp.fromDate(new Date('2026-06-01T13:00:00Z'))
                        })
                      })
                    ]
                  })
                })
              ],
              replyCount: 1,
              threadParticipantUserIds: ['u2'],
              viewerIsFollowingThread: true,
              viewerIsFollowingThreadPresent: true,
              reactions: [
                {
                  emoji: 'thumbsup',
                  count: 2,
                  hasReacted: true,
                  userIds: ['u1', 'u2']
                }
              ]
            })
          }
        }),
        new RoomTimelineEvent({
          id: 'join1',
          createdAt: Timestamp.fromDate(new Date('2026-06-01T12:00:01Z')),
          actorId: 'u2',
          event: {
            case: 'userJoinedRoom',
            value: new RoomTimelineRoomEvent({ roomId: 'room-1' })
          }
        })
      ]
    });

    const mapped = roomTimelinePageToEventConnectionPage(page);

    expect(mapped.startCursor).toBe('seq:10');
    expect(mapped.hasOlder).toBe(true);
    expect(mapped.events).toHaveLength(2);
    expect(mapped.events[0]).toMatchObject({
      id: 'm1',
      createdAt: '2026-06-01T12:00:00.000Z',
      actor: { id: 'u1', displayName: 'Alice', avatarUrl: '/avatars/u1' },
      event: {
        __typename: 'MessagePostedEvent',
        body: 'hello',
        attachments: [
          {
            id: 'a-video',
            filename: 'clip.mp4',
            contentType: 'video/mp4',
            videoProcessing: {
              status: 'COMPLETED',
              durationMs: 1234,
              width: 1280,
              height: 720,
              sourceAvailable: true,
              thumbnailAssetUrl: { url: '/assets/files/a-thumb' },
              variants: [
                {
                  quality: '720p',
                  width: 1280,
                  height: 720,
                  size: 4567,
                  assetUrl: { url: '/assets/files/a-variant' }
                }
              ]
            }
          }
        ],
        reactions: [
          {
            emoji: 'thumbsup',
            count: 2,
            hasReacted: true,
            users: [
              { id: 'u1', displayName: 'Alice' },
              { id: 'u2', displayName: 'Bob' }
            ]
          }
        ],
        threadParticipants: [{ id: 'u2', displayName: 'Bob' }],
        viewerIsFollowingThread: true
      }
    });
    expect(mapped.events[1]).toMatchObject({
      id: 'join1',
      event: { __typename: 'UserJoinedRoomEvent', roomId: 'room-1' }
    });
  });
});
