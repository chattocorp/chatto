import type { MessagesStore } from '$lib/state/room/messages.svelte';
import { onRoomMessageMutated } from '$lib/state/room/messageMutationEvents';

type TimelineMutationTarget = Pick<
  MessagesStore,
  'applyLocalMessageDeletion' | 'refreshAnchorForMessageMutation' | 'refreshCurrentWindow'
>;

/**
 * Reconcile local message mutations while a room or thread timeline is mounted.
 * Read the current scope and timeline for each event so reused route components
 * cannot update a previous room or server. The owning effect removes the listener.
 * Deletions apply directly; other mutations refresh only a visible message or echo.
 */
export function useTimelineMutations(
  getTarget: () => { serverId: string; roomId: string; timeline: TimelineMutationTarget }
): void {
  $effect(() =>
    onRoomMessageMutated((detail) => {
      const { serverId, roomId, timeline } = getTarget();
      if (detail.serverId !== serverId || detail.roomId !== roomId) return;
      if (detail.reason === 'message-deleted') {
        timeline.applyLocalMessageDeletion(detail.eventId);
        return;
      }
      const anchorEventId = timeline.refreshAnchorForMessageMutation(detail.eventId);
      if (anchorEventId) void timeline.refreshCurrentWindow(anchorEventId);
    })
  );
}
