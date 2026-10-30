/** Room menu operations shared by the sidebar and entity links. Hosts own menu and dialog state. */
import { goto, pushState } from '$app/navigation';
import { resolve } from '$app/paths';
import { createRoomCommandAPI } from '@chatto/client/api/rooms';
import type { RoomListItem } from '@chatto/client/server/rooms';
import type { ServerScope } from '$lib/state/server/scope.svelte';
import { serverUi } from '$lib/state/server/serverUi';
import { serverIdToSegment } from '$lib/navigation';
import { conversationPath } from './conversationPath';
import { m } from '$lib/i18n/messages';
import { toast } from '$lib/ui/toast';
import { toastError } from '$lib/utils/errorMessage';

/** Open the existing leave-room confirmation through shallow routing. */
export function openLeaveRoomDialog(
  serverId: string,
  room: Pick<RoomListItem, 'id' | 'name'>
): void {
  pushState('', { modal: { type: 'leaveRoom', serverId, roomId: room.id, roomName: room.name } });
}

/** Navigate to the room's management page. */
export function openRoomSettings(serverId: string, roomId: string): void {
  void goto(
    resolve('/chat/[serverId]/manage/rooms/[roomId]', {
      serverId: serverIdToSegment(serverId),
      roomId
    })
  );
}

/** Join a room, or open its invitation. Return success so the host can close its menu. */
export async function joinRoomFromMenu(
  scope: ServerScope,
  room: Pick<RoomListItem, 'id' | 'name' | 'viewerInvitation'>
): Promise<boolean> {
  if (room.viewerInvitation) {
    await goto(conversationPath(scope.serverId, room.id, { store: scope.store }));
    return true;
  }
  const result = await serverUi(scope.store).roomDirectory.joinRoom(room.id);
  if (!scope.isCurrent()) return false;
  if (result.ok) toast.success(m('room.join.success', { room: room.name }));
  else toast.error(m('room.join.failed'));
  return result.ok;
}

/** Archive after host confirmation. Leave the dialog open on failure; suppress stale feedback. */
export async function archiveRoomFromMenu(scope: ServerScope, roomId: string): Promise<boolean> {
  try {
    await scope.connection.getAPI(createRoomCommandAPI).archiveRoom(roomId);
    if (!scope.isCurrent()) return false;
    toast.success(m('room.management.room_archived'));
    return true;
  } catch (error) {
    if (scope.isCurrent()) {
      toastError(
        error,
        m('room.management.archive_room_failed', { error: m('common.error.generic') })
      );
    }
    return false;
  }
}
