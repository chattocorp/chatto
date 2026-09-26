import { createRoomTimelineAPI } from '$lib/api-client/roomTimeline';
import { assetUrlForServer } from '$lib/assets/assetUrls';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import type { MessageAttachmentView } from '$lib/render/messageAttachments';
import { isMessagePostedEvent } from '$lib/render/timelineEvents';
import type { UserAvatarUserView } from '$lib/render/users';
import { unmask } from '$lib/state/room/messages/helpers';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { Query, QueryKey } from '@tanstack/svelte-query';
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
 * never shows a preview that the previous session loaded.
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
 * Returns null when the message does not exist, is not a posted message, or
 * has neither a body nor attachments.
 */
export async function fetchMessagePreview(
  serverId: string,
  connection: MessagePreviewConnection,
  roomId: string,
  messageId: string
): Promise<MessagePreview | null> {
  const page = await connection
    .getAPI(createRoomTimelineAPI)
    .getRoomEventsAround({ roomId, eventId: messageId, limit: 1 });
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

type PreviewMatch = (
  roomId: unknown,
  eventId: unknown,
  data: MessagePreview | null | undefined
) => boolean;

function previewQueryKeys(serverId: string, match: PreviewMatch): QueryKey[] {
  return queryClient
    .getQueryCache()
    .findAll({
      predicate: (query: Query) => {
        const key = query.queryKey;
        return (
          key[0] === 'server' &&
          key[1] === serverId &&
          key[4] === 'message-preview' &&
          match(key[5], key[6], query.state.data as MessagePreview | null | undefined)
        );
      }
    })
    .map((query) => query.queryKey);
}

/**
 * Hide matching previews at once, then load them again with current access.
 * Cancel reads in flight so a late response cannot restore a preview. A
 * cancelled read restores the manually set null, not the older data.
 */
function purgePreviews(serverId: string, match: PreviewMatch): void {
  for (const queryKey of previewQueryKeys(serverId, match)) {
    const filters = { queryKey, exact: true };
    queryClient.setQueryData(queryKey, null);
    void queryClient.cancelQueries(filters).then(() => queryClient.invalidateQueries(filters));
  }
}

registerMessagePreviewQueryCache({
  refreshMessage(serverId, roomId, eventId) {
    for (const queryKey of previewQueryKeys(
      serverId,
      (room, event) => room === roomId && event === eventId
    )) {
      void queryClient.invalidateQueries({ queryKey, exact: true });
    }
  },
  purgeMessage(serverId, roomId, eventId) {
    purgePreviews(serverId, (room, event) => room === roomId && event === eventId);
  },
  purgeRoom(serverId, roomId) {
    purgePreviews(serverId, (room) => room === roomId);
  },
  purgeAuthor(serverId, userId) {
    purgePreviews(serverId, (_room, _event, data) => data?.actor?.id === userId);
  }
});
