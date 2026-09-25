// SPDX-License-Identifier: Apache-2.0

/** Resource payload schemas. The storage envelope owns the version and applied checkpoint. */
import { z } from 'zod';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { RoomThreadingMode } from '$lib/roomThreading';
import { VideoProcessingStatus } from '$lib/render/messageAttachments';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import type { MessageSearchStatus } from '$lib/api-client/messageSearch';
import { NotificationAttentionLevel } from '@chatto/api-types/api/v1/notifications_pb';
import {
  NotificationSignalKind,
  type NotificationOccurrencePage
} from '$lib/api-client/notifications';

const text = z.string().nullish();
const number = z.number().finite().nullish();
const flag = z.boolean().nullish();
const asset = z.object({ url: z.string(), expiresAt: z.string() }).nullish();
const user = z.object({
  id: z.string(),
  login: z.string(),
  displayName: z.string(),
  deleted: z.boolean(),
  isBot: z.boolean().optional(),
  avatarUrl: text,
  presenceStatus: z.enum(PresenceStatus),
  customStatus: z.object({ emoji: z.string(), text: z.string(), expiresAt: text }).nullish()
});

/** The same notification page used by the live store, including sidebar counts. */
export const notificationSnapshotSchema: z.ZodType<NotificationOccurrencePage> = z.object({
  occurrences: z.array(
    z.object({
      id: z.string(),
      createdAt: z.string(),
      actor: user
        .extend({
          avatarUrl: z.string().nullable(),
          bio: text,
          timezone: text,
          bot: z.object({ ownerUserId: z.string() }).optional(),
          customStatus: z
            .object({ emoji: z.string(), text: z.string(), expiresAt: z.string().nullable() })
            .nullish()
        })
        .nullable(),
      signalKind: z.enum(NotificationSignalKind),
      targetSupported: z.boolean(),
      room: z.object({ id: z.string(), name: z.string() }).nullable(),
      eventId: z.string(),
      threadRootId: z.string().nullable(),
      attentionLevel: z.enum(NotificationAttentionLevel),
      unread: z.boolean(),
      reactionEmoji: text,
      expiresAt: z.string().optional()
    })
  ),
  consumedCount: z.number().optional(),
  totalCount: z.number(),
  hasMore: z.boolean(),
  unreadCount: z.number(),
  importantUnreadCount: z.number(),
  roomUnreadCounts: z.record(z.string(), z.number()),
  roomImportantUnreadCounts: z.record(z.string(), z.number()),
  nextExpiryAt: text
});
const attachment = z.object({
  id: z.string(),
  filename: z.string(),
  contentType: z.string(),
  description: text,
  width: z.number().finite(),
  height: z.number().finite(),
  assetUrl: asset,
  thumbnailAssetUrl: asset,
  videoProcessing: z
    .object({
      status: z.enum(VideoProcessingStatus),
      durationMs: z.union([z.number(), z.string()]).nullish(),
      width: number,
      height: number,
      thumbnailAssetUrl: asset,
      sourceAvailable: z.boolean(),
      variants: z.array(
        z.object({
          quality: z.string(),
          width: z.number(),
          height: z.number(),
          size: z.number(),
          assetUrl: asset
        })
      ),
      hlsMasterPlaylistUrl: asset,
      reasonCode: text
    })
    .nullish()
});
const externalLink = z.object({ url: z.string(), title: text, description: text, imageUrl: text });
const socialPost = z.object({
  provider: z.string(),
  url: text,
  author: z.object({ displayName: z.string(), handle: z.string(), avatarUrl: text }).nullish(),
  text: z.string(),
  publishedAt: text,
  externalLink: externalLink.nullish(),
  contentWarning: text,
  images: z.array(z.object({ url: z.string(), alt: text, width: number, height: number })),
  get quotedPost() {
    return socialPost.nullish();
  }
});
const message = z.object({
  kind: z.literal('messagePosted'),
  roomId: z.string(),
  body: z.string().nullable(),
  attachments: z.array(attachment),
  linkPreview: externalLink
    .extend({ siteName: text, embedType: text, embedId: text, socialPost: socialPost.nullish() })
    .nullish(),
  reactions: z.array(
    z.object({
      emoji: z.string(),
      count: z.number(),
      hasReacted: z.boolean(),
      users: z.array(
        z.object({
          id: z.string(),
          displayName: z.string(),
          isBot: z.boolean().optional(),
          deleted: z.boolean().optional()
        })
      )
    })
  ),
  updatedAt: text,
  inReplyTo: text,
  threadRootEventId: text,
  echoOfEventId: text,
  echoFromThreadRootEventId: text,
  channelEchoEventId: text,
  deletedAt: text,
  pinned: z.boolean().optional(),
  threadExists: z.boolean().optional(),
  canReplyInThread: z.boolean().optional(),
  replyCount: z.number(),
  lastReplyAt: text,
  threadParticipantCount: z.number().optional(),
  threadParticipants: z.array(user),
  viewerIsFollowingThread: flag,
  viewerHasUnreadThread: flag
});

/** Validate the full render shape before any persisted row reaches a component. */
export const timelineSnapshotSchema: z.ZodType<TimelineEventView[]> = z.array(
  z.object({
    id: z.string(),
    createdAt: z.string(),
    actorId: text,
    actor: user.nullish(),
    actorResolution: z.enum(['loading', 'unavailable', 'deleted']).optional(),
    event: z.union([
      message,
      z.object({
        kind: z.enum(['callStarted', 'callEnded']),
        roomId: z.string(),
        callId: z.string()
      }),
      z.object({
        kind: z.enum([
          'roomArchived',
          'roomCreated',
          'roomDeleted',
          'roomUnarchived',
          'roomUpdated',
          'userJoinedRoom',
          'userLeftRoom'
        ]),
        roomId: z.string()
      }),
      z.object({
        kind: z.literal('roomThreadingModeChanged'),
        roomId: z.string(),
        threadingMode: z.enum(RoomThreadingMode)
      })
    ])
  })
);

/** Protobuf resources use their own validated JSON codec; timelines use the render codec above. */
export type ServerPresentationSnapshot = {
  server: string;
  serverVersion?: string;
  activeCalls?: string[];
  searchStatus?: MessageSearchStatus;
  roomGroups: string[];
  users: string[];
  viewer?: string;
  runtime?: string;
  motd?: string;
  notifications?: NotificationOccurrencePage;
};
