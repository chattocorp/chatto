import { createRoomTimelineAPI } from '$lib/api-client/roomTimeline';
import { assetUrlForServer } from '$lib/assets/assetUrls';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import type { MessageAttachmentView } from '$lib/render/messageAttachments';
import { isMessagePostedEvent } from '$lib/render/timelineEvents';
import type { UserAvatarUserView } from '$lib/render/users';
import { unmask } from '$lib/state/room/messages/helpers';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { Query } from '@tanstack/svelte-query';
import { registerMessagePreviewQueryCache } from './cacheRegistry';
import { queryClient } from './client';
import { serverSessionQueryRoot } from './keys';

type MessagePreviewConnection = Pick<ServerConnection, 'queryScope' | 'getAPI'>;

/** One attachment summary in a message link preview. */
export interface MessagePreviewAttachment {
  id: string;
  filename: string;
  contentType: string;
  description: string | null;
  thumbnailAssetUrl: ExpiringAssetUrl | null;
  videoThumbnailAssetUrl: ExpiringAssetUrl | null;
  /** The thumbnail to display: the video thumbnail for videos, else the image thumbnail. */
  thumbnailUrl: string | null;
}

/** The linked message content that a preview card renders. */
export interface MessagePreview {
  body: string | null;
  attachments: MessagePreviewAttachment[];
  actor: UserAvatarUserView | null;
}

/**
 * Key for one linked message preview.
 *
 * The key is scoped to the connection session, so a sign-out or account change
 * never shows a preview that the previous session loaded. Previews are not
 * cached after their card unmounts: the room timeline owns message content,
 * and the card owns refreshed thumbnail URLs.
 */
export function messagePreviewQueryKey(
  serverId: string,
  connection: Pick<ServerConnection, 'queryScope'>,
  roomId: string,
  messageId: string
) {
  return [
    ...serverSessionQueryRoot(serverId, connection),
    'message-preview',
    roomId,
    messageId
  ] as const;
}

function normalizeAssetUrl(
  serverId: string,
  value: ExpiringAssetUrl | null | undefined
): ExpiringAssetUrl | null {
  if (!value) return null;
  return { ...value, url: assetUrlForServer(serverId, value.url) ?? value.url };
}

function withThumbnailUrls(
  serverId: string,
  attachment: Omit<
    MessagePreviewAttachment,
    'thumbnailAssetUrl' | 'videoThumbnailAssetUrl' | 'thumbnailUrl'
  >,
  thumbnail: ExpiringAssetUrl | null | undefined,
  videoThumbnail: ExpiringAssetUrl | null | undefined
): MessagePreviewAttachment {
  const thumbnailAssetUrl = normalizeAssetUrl(serverId, thumbnail);
  const videoThumbnailAssetUrl = normalizeAssetUrl(serverId, videoThumbnail);
  const displayed = attachment.contentType.startsWith('video/')
    ? (videoThumbnailAssetUrl ?? thumbnailAssetUrl)
    : thumbnailAssetUrl;
  return {
    ...attachment,
    thumbnailAssetUrl,
    videoThumbnailAssetUrl,
    thumbnailUrl: displayed?.url ?? null
  };
}

/**
 * Load the preview content of a linked message.
 *
 * Pass the store's `minimumReadCursor` as `minimumCursor`, so the read includes
 * every edit and retraction that the client has already received.
 * Returns null when the message does not exist, is not a posted message, or
 * has neither a body nor attachments.
 */
export async function fetchMessagePreview(
  serverId: string,
  connection: MessagePreviewConnection,
  target: { roomId: string; messageId: string; minimumCursor?: string; signal?: AbortSignal }
): Promise<MessagePreview | null> {
  const { roomId, messageId, minimumCursor, signal } = target;
  const page = await connection
    .getAPI(createRoomTimelineAPI)
    .getRoomEventsAround({ roomId, eventId: messageId, limit: 1, minimumCursor, signal });
  const event = unmask(page.events).find((item) => item.id === messageId);
  const inner = event?.event;
  if (!event || !isMessagePostedEvent(inner)) return null;
  if (!inner.body && inner.attachments.length === 0) return null;

  return {
    body: inner.body ?? null,
    attachments: inner.attachments.map((attachment: MessageAttachmentView) =>
      withThumbnailUrls(
        serverId,
        {
          id: attachment.id,
          filename: attachment.filename,
          contentType: attachment.contentType,
          description: attachment.description ?? null
        },
        attachment.thumbnailAssetUrl,
        attachment.videoProcessing?.thumbnailAssetUrl
      )
    ),
    actor: event.actor ?? null
  };
}

/** Apply refreshed signed thumbnail URLs to a loaded preview. */
export function withRefreshedPreviewUrls(
  serverId: string,
  preview: MessagePreview,
  freshUrls: ReadonlyMap<string, RefreshedAttachmentUrls>
): MessagePreview {
  return {
    ...preview,
    attachments: preview.attachments.map((attachment) => {
      const fresh = freshUrls.get(attachment.id);
      if (!fresh) return attachment;
      return withThumbnailUrls(
        serverId,
        attachment,
        fresh.thumbnailAssetUrl,
        fresh.videoThumbnailAssetUrl
      );
    })
  };
}

/** Reload every mounted preview of a server; each keeps its data while it reloads. */
function refreshMessagePreviews(serverId: string): void {
  const filters = {
    predicate: (query: Query) => {
      const key = query.queryKey;
      return key[0] === 'server' && key[1] === serverId && key[4] === 'message-preview';
    }
  };
  // Cancel reads that started before the refresh so they cannot answer it.
  void queryClient.cancelQueries(filters).then(() => queryClient.invalidateQueries(filters));
}

registerMessagePreviewQueryCache({ refresh: refreshMessagePreviews });
