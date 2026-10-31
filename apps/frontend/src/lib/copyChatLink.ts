/** Clipboard actions and user feedback for server and conversation links. */
import { buildRoomLinkURL, buildMessageLinkURL } from '$lib/chatLinks';
import { toast } from '$lib/ui/toast';
import { m } from '$lib/i18n/messages';
import { serverRegistry } from '$lib/client';

/** Copy the server's public root URL, independent of the current frontend route. */
export async function copyServerLinkToClipboard(serverId: string): Promise<void> {
  try {
    const server = serverRegistry.getServer(serverId);
    if (!server) throw new Error('Unknown server');
    await navigator.clipboard.writeText(new URL('/', server.url).href);
    toast.success(m('room.message.actions.link_copied'));
  } catch {
    toast.error(m('room.message.actions.copy_link_failed'));
  }
}

/** Copy a room URL and report clipboard or URL-generation failures. */
export async function copyRoomLinkToClipboard(serverId: string, roomId: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(buildRoomLinkURL(serverId, roomId));
    toast.success(m('room.message.actions.link_copied'));
  } catch {
    toast.error(m('room.message.actions.copy_link_failed'));
  }
}

/** Copy a message URL and report clipboard or URL-generation failures. */
export async function copyMessageLinkToClipboard(
  serverId: string,
  roomId: string,
  messageId: string,
  threadRootEventId?: string | null
): Promise<void> {
  try {
    await navigator.clipboard.writeText(
      buildMessageLinkURL(serverId, roomId, messageId, threadRootEventId)
    );
    toast.success(m('room.message.actions.message_link_copied'));
  } catch {
    toast.error(m('room.message.actions.copy_link_failed'));
  }
}
