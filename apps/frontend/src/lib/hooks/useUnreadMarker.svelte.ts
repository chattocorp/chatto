import { appState } from '$lib/state/globals.svelte';

export type UnreadMarkerWindow = {
  afterTime: string;
  beforeTime: string | number;
};

export type UnreadMarkerEvent = {
  id: string;
  actorId?: string | null;
  createdAt: string;
};

type UseUnreadMarkerOptions<TReadResult> = {
  markAsRead: (targetId: string, upToEventId?: string) => Promise<TReadResult | null>;
  markerWindowFromReadResult: (
    result: TReadResult,
    markedAtMs: number
  ) => UnreadMarkerWindow | null;
  getMarkerEvents: () => readonly UnreadMarkerEvent[];
  getMarkerSkipActorId?: () => string | null | undefined;
};

/**
 * Shared unread separator lifecycle for room and thread timelines.
 *
 * The rendered separator is always a concrete event id. Server read-state
 * timestamp windows are resolved against the owning timeline events. The
 * server read cursor is the source of truth on entry, target changes, and
 * refocus.
 */
export function useUnreadMarker<TReadResult>(
  getTargetId: () => string,
  {
    markAsRead,
    markerWindowFromReadResult,
    getMarkerEvents,
    getMarkerSkipActorId
  }: UseUnreadMarkerOptions<TReadResult>
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

  $effect(() => {
    const markerWindow = unreadMarkerWindow;
    if (!markerWindow) return;

    const afterMs = Date.parse(markerWindow.afterTime);
    const beforeMs =
      typeof markerWindow.beforeTime === 'number'
        ? markerWindow.beforeTime
        : Date.parse(markerWindow.beforeTime);
    const skipActorId = getMarkerSkipActorId?.();

    for (const event of getMarkerEvents()) {
      if (skipActorId && event.actorId === skipActorId) continue;

      const eventMs = Date.parse(event.createdAt);
      if (eventMs > afterMs && eventMs <= beforeMs) {
        setUnreadMarkerEventId(event.id);
        return;
      }
    }
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
