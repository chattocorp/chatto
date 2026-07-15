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
 * timestamp windows are resolved once by the timeline pane. The server read
 * cursor is the source of truth on entry, target changes, and refocus.
 */
export function useUnreadMarker<TReadResult>(
  getTargetId: () => string,
  { markAsRead, markerWindowFromReadResult }: UseUnreadMarkerOptions<TReadResult>
) {
  let unreadMarkerEventId = $state<string | null>(null);
  let unreadMarkerWindow = $state<UnreadMarkerWindow | null>(null);

  let lastFiredTargetId = '';
  let wasPresent = false;
  let readMarkerGeneration = 0;

  async function markTargetAsRead(targetId: string, upToEventId?: string) {
    return markAsRead(targetId, upToEventId);
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
  }

  $effect(() => {
    const targetId = getTargetId();
    const present = appState.isPresent;

    if (!present) {
      wasPresent = false;
      return;
    }

    const isTargetChange = lastFiredTargetId !== targetId;

    if (wasPresent && !isTargetChange) return;

    wasPresent = true;
    lastFiredTargetId = targetId;

    if (isTargetChange) {
      clearUnreadMarker();
    }

    const markedAtMs = Date.now();
    const generation = ++readMarkerGeneration;
    markAsRead(targetId).then((result) => {
      if (generation !== readMarkerGeneration) return;
      if (getTargetId() !== targetId || !result) return;

      unreadMarkerEventId = null;
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
    setUnreadMarkerEventId,
    clearUnreadMarker
  };
}
