/** Cross-server navigation to the next conversation that needs notification attention. */
import { goto } from '$app/navigation';
import {
  NotificationAttentionLevel,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import { notificationTarget } from '@chatto/client/server/notifications';
import { serverRegistry } from '$lib/client';
import { m } from '$lib/i18n/messages';
import { notificationPath } from '$lib/notificationPath';
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
};

/** Select Important before Ambient, then a stable room/thread order and its newest occurrence. */
export function nextUnreadSource(servers: UnreadSourceServer[]) {
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
  return candidates[0];
}

/** One app-shell action shared by the shortcut and palette; overlapping invocations are ignored. */
export function createNextUnreadNavigation(appUi: AppUiState): () => Promise<void> {
  let pending = false;
  return async () => {
    if (pending) return;
    pending = true;
    const location = window.location.href;
    let current = true;
    const stores = serverRegistry.servers.flatMap(({ id }) => {
      const store = serverRegistry.tryGetStore(id);
      return store?.isAuthenticated ? [{ id, store }] : [];
    });
    const cleanup = stores.flatMap(({ store }) => [
      store.onSessionEnded(() => {
        current = false;
      }),
      store.onReset((reset) => {
        if (reset.privacy) current = false;
      })
    ]);
    try {
      // Complete the finite view so older Important sources cannot be hidden by pagination.
      await Promise.all(
        stores.map(async ({ store }) => {
          if (!store.notifications.hasLoaded) await store.notifications.fetchPage(0);
          await store.notifications.fetchAllPages();
        })
      );
      if (
        !current ||
        window.location.href !== location ||
        document.querySelector('dialog[open], [aria-modal="true"]')
      )
        return;
      const selected = nextUnreadSource(
        stores.flatMap(({ id, store }) => {
          if (!store.isAuthenticated || serverRegistry.tryGetStore(id) !== store) return [];
          const ui = serverUi(store);
          return [
            {
              serverId: id,
              roomIds: ui.navigation.roomGroups.flatMap((group) => group.roomIds),
              occurrences: ui.attention.occurrences
            }
          ];
        })
      );
      if (!selected) {
        toast.info(m('chat.notifications.no_unread'), 2500);
        return;
      }
      const store = serverRegistry.getStore(selected.serverId);
      const { occurrence, target } = selected;
      prepareUiForNotificationTarget(appUi, selected.serverId, target);
      if (target.roomId && occurrence.eventId) {
        serverUi(store).pendingHighlights.set(
          target.roomId,
          target.threadRootId,
          occurrence.eventId,
          occurrence.id
        );
      }
      await navigateNotification(store, occurrence, () =>
        goto(notificationPath(selected.serverId, occurrence))
      );
    } catch (error) {
      if (current) toastError(error, m('common.error.network'));
    } finally {
      cleanup.forEach((stop) => stop());
      pending = false;
    }
  };
}
