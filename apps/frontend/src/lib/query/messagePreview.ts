import { createRoomTimelineAPI } from '$lib/api-client/roomTimeline';
import { assetUrlForServer } from '$lib/assets/assetUrls';
import type { ExpiringAssetUrl, RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';
import type { MessageAttachmentView } from '$lib/render/messageAttachments';
import { isMessagePostedEvent } from '$lib/render/timelineEvents';
import type { UserAvatarUserView } from '$lib/render/users';
import { unmask } from '$lib/state/room/messages/helpers';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
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
  messageId: string,
  signal?: AbortSignal
): Promise<MessagePreview | null> {
  const page = await connection
    .getAPI(createRoomTimelineAPI)
    .getRoomEventsAround({ roomId, eventId: messageId, limit: 1, signal });
  const event = unmask(page.events).find((item) => item.id === messageId);
  const inner = event?.event;
  if (!event || !isMessagePostedEvent(inner)) return null;
  if (!inner.body && inner.attachments.length === 0) return null;

  return {
    body: inner.body ?? null,
    attachments: inner.attachments.map((attachment: MessageAttachmentView) => ({
      id: attachment.id,
      filename: attachment.filename,
      contentType: attachment.contentType,
      description: attachment.description ?? null,
      thumbnailAssetUrl: normalizeAssetUrl(serverId, attachment.thumbnailAssetUrl),
      videoThumbnailAssetUrl: normalizeAssetUrl(
        serverId,
        attachment.videoProcessing?.thumbnailAssetUrl
      )
    })),
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
      return {
        ...attachment,
        thumbnailAssetUrl: normalizeAssetUrl(serverId, fresh.thumbnailAssetUrl),
        videoThumbnailAssetUrl: normalizeAssetUrl(serverId, fresh.videoThumbnailAssetUrl)
      };
    })
  };
}
