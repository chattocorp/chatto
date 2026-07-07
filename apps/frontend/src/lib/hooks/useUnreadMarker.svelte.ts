import { appState } from '$lib/state/globals.svelte';

export type UnreadMarkerWindow = {
  afterTime: string;
  beforeTime: string | number;
};

type UseUnreadMarkerOptions<TReadResult> = {
  markAsRead: (targetId: string, upToEventId?: string) => Promise<TReadResult | null>;
  markerWindowFromReadResult: (
    result: TReadResult,
    markedAtMs: number
  ) => UnreadMarkerWindow | null;
};

/**
 * Shared unread separator lifecycle for room and thread timelines.
 *
 * The rendered separator is always a concrete event id. Server read-state
 * timestamp windows are resolved once by the timeline pane, and same-target
 * refocuses only reveal deferred event ids captured while the user was away.
 */
export function useUnreadMarker<TReadResult>(
  getTargetId: () => string,
  { markAsRead, markerWindowFromReadResult }: UseUnreadMarkerOptions<TReadResult>
) {
  let unreadMarkerEventId = $state<string | null>(null);
  let unreadMarkerWindow = $state<UnreadMarkerWindow | null>(null);
  let pendingAwayEventId: string | null = null;

  async function markTargetAsRead(targetId: string, upToEventId?: string) {
    return markAsRead(targetId, upToEventId);
  }

  function noteAwayEvent(eventId: string) {
    if (unreadMarkerEventId !== null || pendingAwayEventId !== null) return;
    pendingAwayEventId = eventId;
  }

  function setUnreadMarkerEventId(eventId: string | null) {
    unreadMarkerEventId = eventId;
    if (eventId !== null) {
      unreadMarkerWindow = null;
    }
  }

  function clearUnreadMarker() {
    unreadMarkerEventId = null;
    unreadMarkerWindow = null;
    pendingAwayEventId = null;
  }

  let lastFiredTargetId = '';
  let wasPresent = false;

  $effect(() => {
    const targetId = getTargetId();
    const present = appState.isPresent;

    if (!present) {
      wasPresent = false;
      return;
    }

    const isTargetChange = lastFiredTargetId !== targetId;
    const awayEventId = pendingAwayEventId;

    if (wasPresent && !isTargetChange) return;

    wasPresent = true;
    lastFiredTargetId = targetId;

    if (isTargetChange) {
      clearUnreadMarker();
    } else if (awayEventId) {
      setUnreadMarkerEventId(awayEventId);
      pendingAwayEventId = null;
    } else {
      pendingAwayEventId = null;
      return;
    }

    const markedAtMs = Date.now();
    markTargetAsRead(targetId).then((result) => {
      if (getTargetId() !== targetId || !result) return;
      if (!isTargetChange) return;

      unreadMarkerWindow = markerWindowFromReadResult(result, markedAtMs);
    });
  });

  return {
    get unreadMarkerEventId() {
      return unreadMarkerEventId;
    },
    get unreadMarkerWindow() {
      return unreadMarkerWindow;
    },
    markAsRead: markTargetAsRead,
    noteAwayEvent,
    setUnreadMarkerEventId,
    clearUnreadMarker
  };
}
