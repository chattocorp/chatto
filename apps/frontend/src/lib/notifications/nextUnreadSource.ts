/** Select and open unread conversations from local frontend state. */
import { goto } from '$app/navigation';
import {
  NotificationAttentionLevel,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import { notificationTarget } from '@chatto/client/server/notifications';
import { serverRegistry } from '$lib/client';
import { m } from '$lib/i18n/messages';
import { notificationPath } from '$lib/notificationPath';
import { conversationPath } from '$lib/navigation/conversationPath';
import type { AppUiState } from '$lib/state/appUi.svelte';
import { serverUi } from '$lib/state/server/serverUi';
import { toast } from '$lib/ui/toast';
import { toastError } from '$lib/utils/errorMessage';
import { navigateNotification, prepareUiForNotificationTarget } from './notificationNavigationUi';

/** Sources arrive in server order; room IDs follow sidebar group order. */
export type UnreadSourceServer = {
  serverId: string;
  roomIds: string[];
  occurrences: NotificationOccurrenceItem[];
  unreadRoomIds: string[];
};

/** A notification jump, or a badge-only room with no message target. */
export type UnreadSource = {
  serverId: string;
  roomId: string;
  threadRootId: string | null;
  occurrence: NotificationOccurrenceItem | null;
};

/** Select Important, then Ambient, then badge-only rooms, in stable sidebar order. */
export function nextUnreadSource(servers: UnreadSourceServer[]): UnreadSource | undefined {
  const candidates = servers.flatMap((server, serverIndex) =>
    server.occurrences
      .filter((row) => row.unread && row.targetSupported && row.room)
      .map((occurrence) => {
        const target = notificationTarget(occurrence);
        const roomIndex = server.roomIds.indexOf(target.roomId!);
        return {
          serverId: server.serverId,
          serverIndex,
          roomIndex: roomIndex < 0 ? server.roomIds.length : roomIndex,
          occurrence,
          target
        };
      })
  );
  candidates.sort(
    (a, b) =>
      Number(b.occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT) -
        Number(a.occurrence.attentionLevel === NotificationAttentionLevel.IMPORTANT) ||
      a.serverIndex - b.serverIndex ||
      a.roomIndex - b.roomIndex ||
      a.target.roomId!.localeCompare(b.target.roomId!) ||
      (a.target.threadRootId ?? '').localeCompare(b.target.threadRootId ?? '') ||
      b.occurrence.createdAt.localeCompare(a.occurrence.createdAt) ||
      a.occurrence.id.localeCompare(b.occurrence.id)
  );
  const first = candidates[0];
  if (first) {
    return {
      serverId: first.serverId,
      roomId: first.target.roomId!,
      threadRootId: first.target.threadRootId,
      occurrence: first.occurrence
    };
  }
  for (const server of servers) {
    const roomId = server.unreadRoomIds[0];
    if (roomId) return { serverId: server.serverId, roomId, threadRootId: null, occurrence: null };
  }
}

/** Snapshot eligible sources once per invocation; this does not load server data. */
function localUnreadSources(): UnreadSourceServer[] {
  const sources: UnreadSourceServer[] = [];
  for (const { id } of serverRegistry.servers) {
    const store = serverRegistry.tryGetStore(id);
    if (!store?.isAuthenticated) continue;
    const ui = serverUi(store);
    const roomIds = ui.navigation.roomGroups.flatMap((group) => group.roomIds);
    const badgeRoomIds = new Set([...roomIds, ...store.projection.rooms.keys()]);
    sources.push({
      serverId: id,
      roomIds,
      // The indicator helper limits its list to 50; navigation uses all local rows.
      occurrences: store.notifications.occurrences.filter((row) =>
        ui.attention.needsAttention(row)
      ),
      unreadRoomIds: [...badgeRoomIds].filter(
        (roomId) => ui.roomUnread.roomIsUnread(roomId) && !ui.readViews.covers(roomId)
      )
    });
  }
  return sources;
}

/** One app-shell action shared by the shortcut and palette; overlapping invocations are ignored. */
export function createNextUnreadNavigation(appUi: AppUiState): () => Promise<void> {
  let pending = false;
  return async () => {
    if (pending || document.querySelector('dialog[open], [aria-modal="true"]')) return;
    pending = true;
    try {
      const selected = nextUnreadSource(localUnreadSources());
      if (!selected) {
        toast.info(m('chat.notifications.no_unread'), 2500);
        return;
      }
      const { serverId, roomId, threadRootId, occurrence } = selected;
      prepareUiForNotificationTarget(appUi, serverId, selected);
      if (!occurrence) {
        await goto(conversationPath(serverId, roomId));
        return;
      }
      const store = serverRegistry.getStore(serverId);
      if (occurrence.eventId) {
        serverUi(store).pendingHighlights.set(
          roomId,
          threadRootId,
          occurrence.eventId,
          occurrence.id
        );
      }
      await navigateNotification(store, occurrence, () =>
        goto(notificationPath(serverId, occurrence))
      );
    } catch (error) {
      toastError(error, m('common.error.network'));
    } finally {
      pending = false;
    }
  };
}
