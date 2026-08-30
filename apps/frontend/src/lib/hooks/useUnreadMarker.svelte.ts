import { Code, ConnectError } from '$lib/api-client/connect';
import { appState } from '$lib/state/globals.svelte';
import { onDestroy } from 'svelte';

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
  markAsRead: (
    targetId: string,
    upToEventId: string | undefined,
    signal: AbortSignal
  ) => Promise<TReadResult>;
  markerWindowFromReadResult: (
    result: TReadResult,
    markedAtMs: number
  ) => UnreadMarkerWindow | null;
  getMarkerEvents: () => readonly UnreadMarkerEvent[];
  getMarkerSkipActorId?: () => string | null | undefined;
  canMarkAsRead?: () => boolean;
  onMarkAsReadError?: (error: unknown) => void;
};

type ReadAttempt = {
  generation: number;
  targetId: string;
  upToEventId?: string;
  markedAtMs: number;
  retryCount: number;
  timer: ReturnType<typeof setTimeout> | null;
  inFlight: boolean;
  retryWhenSettled: boolean;
  errorReported: boolean;
  updatesMarker: boolean;
  markerGeneration: number;
  controller: AbortController;
};

const INITIAL_RETRY_DELAY_MS = 500;
const MAX_RETRY_DELAY_MS = 30_000;

/** Return true when another read attempt can succeed without a user change. */
export function isTransientReadError(error: unknown): boolean {
  if (error instanceof TypeError) return true;
  if (
    typeof DOMException !== 'undefined' &&
    error instanceof DOMException &&
    ['AbortError', 'NetworkError', 'TimeoutError'].includes(error.name)
  ) {
    return true;
  }
  if (!(error instanceof ConnectError)) return false;

  switch (error.code) {
    case Code.Canceled:
    case Code.Unknown:
    case Code.DeadlineExceeded:
    case Code.ResourceExhausted:
    case Code.Aborted:
    case Code.Internal:
    case Code.Unavailable:
      return true;
    default:
      return false;
  }
}

/**
 * Shared unread separator lifecycle for room and thread timelines.
 *
 * A visible target is marked as read on entry and after the app returns to the
 * foreground. Focus still controls reads for messages that arrive while the
 * target stays open. Failed transient requests retry while the target stays
 * visible and readable.
 *
 * The rendered separator is always a concrete event id. Server read-state
 * timestamp windows are resolved against the owning timeline events. The
 * server read cursor remains the source of truth.
 */
export function useUnreadMarker<TReadResult>(
  getTargetId: () => string,
  {
    markAsRead,
    markerWindowFromReadResult,
    getMarkerEvents,
    getMarkerSkipActorId,
    canMarkAsRead = () => true,
    onMarkAsReadError
  }: UseUnreadMarkerOptions<TReadResult>
) {
  let unreadMarkerEventId = $state<string | null>(null);
  let unreadMarkerWindow = $state<UnreadMarkerWindow | null>(null);

  let lifecycleAttempt: ReadAttempt | null = null;
  let explicitAttempt: ReadAttempt | null = null;
  let lifecycleGeneration = 0;
  let explicitGeneration = 0;
  let markerGeneration = 0;
  let destroyed = false;

  let initialized = false;
  let lastTargetId = '';
  let wasPresent = false;
  let wasReadable = false;
  let lastForegroundRevision = 0;
  let lastOnlineRevision = 0;

  function isCurrentAttempt(attempt: ReadAttempt): boolean {
    const currentAttempt = attempt.updatesMarker ? lifecycleAttempt : explicitAttempt;
    const currentGeneration = attempt.updatesMarker ? lifecycleGeneration : explicitGeneration;
    return (
      !destroyed &&
      currentAttempt === attempt &&
      currentGeneration === attempt.generation &&
      getTargetId() === attempt.targetId &&
      appState.isVisible &&
      canMarkAsRead()
    );
  }

  function cancelAttempt(attempt: ReadAttempt | null) {
    if (attempt?.timer !== null && attempt?.timer !== undefined) {
      clearTimeout(attempt.timer);
    }
    attempt?.controller.abort();
  }

  function cancelLifecycleAttempt() {
    cancelAttempt(lifecycleAttempt);
    lifecycleAttempt = null;
    lifecycleGeneration += 1;
  }

  function cancelExplicitAttempt() {
    cancelAttempt(explicitAttempt);
    explicitAttempt = null;
    explicitGeneration += 1;
  }

  function cancelAllAttempts() {
    cancelLifecycleAttempt();
    cancelExplicitAttempt();
  }

  function retryDelay(retryCount: number): number {
    return Math.min(INITIAL_RETRY_DELAY_MS * 2 ** retryCount, MAX_RETRY_DELAY_MS);
  }

  function scheduleRetry(attempt: ReadAttempt, immediately = false) {
    if (!isCurrentAttempt(attempt)) return;

    const delay = immediately ? 0 : retryDelay(attempt.retryCount);
    attempt.retryCount += 1;
    attempt.timer = setTimeout(() => {
      attempt.timer = null;
      void runAttempt(attempt);
    }, delay);
  }

  function finishAttempt(attempt: ReadAttempt) {
    if (attempt.updatesMarker) {
      if (lifecycleAttempt === attempt) lifecycleAttempt = null;
    } else if (explicitAttempt === attempt) {
      explicitAttempt = null;
    }
  }

  async function runAttempt(attempt: ReadAttempt): Promise<TReadResult | null> {
    if (!isCurrentAttempt(attempt) || attempt.inFlight) return null;

    attempt.inFlight = true;
    try {
      const result = await markAsRead(
        attempt.targetId,
        attempt.upToEventId,
        attempt.controller.signal
      );
      if (!isCurrentAttempt(attempt)) return null;

      if (attempt.updatesMarker && attempt.markerGeneration === markerGeneration) {
        unreadMarkerEventId = null;
        unreadMarkerWindow = markerWindowFromReadResult(result, attempt.markedAtMs);
      }
      finishAttempt(attempt);
      return result;
    } catch (error) {
      if (!isCurrentAttempt(attempt)) return null;

      if (!attempt.errorReported) {
        attempt.errorReported = true;
        onMarkAsReadError?.(error);
      }
      if (isTransientReadError(error)) {
        scheduleRetry(attempt, attempt.retryWhenSettled);
      } else {
        finishAttempt(attempt);
      }
      return null;
    } finally {
      attempt.inFlight = false;
      attempt.retryWhenSettled = false;
    }
  }

  function createAttempt(
    targetId: string,
    upToEventId: string | undefined,
    updatesMarker: boolean
  ): ReadAttempt {
    return {
      generation: updatesMarker ? ++lifecycleGeneration : ++explicitGeneration,
      targetId,
      upToEventId,
      markedAtMs: Date.now(),
      retryCount: 0,
      timer: null,
      inFlight: false,
      retryWhenSettled: false,
      errorReported: false,
      updatesMarker,
      markerGeneration: updatesMarker ? ++markerGeneration : markerGeneration,
      controller: new AbortController()
    };
  }

  function startLifecycleAttempt(targetId: string) {
    cancelLifecycleAttempt();
    const attempt = createAttempt(targetId, undefined, true);
    lifecycleAttempt = attempt;
    void runAttempt(attempt);
  }

  async function markTargetAsRead(targetId: string, upToEventId?: string) {
    if (!appState.isVisible || !canMarkAsRead() || getTargetId() !== targetId) return null;

    cancelExplicitAttempt();
    const attempt = createAttempt(targetId, upToEventId, false);
    explicitAttempt = attempt;
    return runAttempt(attempt);
  }

  function retryAttemptNow(attempt: ReadAttempt | null) {
    if (!attempt || !isCurrentAttempt(attempt)) return;
    if (attempt.inFlight) {
      attempt.retryWhenSettled = true;
      return;
    }
    if (attempt.timer !== null) {
      clearTimeout(attempt.timer);
      attempt.timer = null;
      void runAttempt(attempt);
    }
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
    const visible = appState.isVisible;
    const present = appState.isPresent;
    const readable = canMarkAsRead();
    const foregroundRevision = appState.foregroundRevision;
    const onlineRevision = appState.onlineRevision;

    const targetChanged = initialized && targetId !== lastTargetId;
    const foregroundChanged = initialized && foregroundRevision !== lastForegroundRevision;
    const onlineChanged = initialized && onlineRevision !== lastOnlineRevision;
    const becamePresent = initialized && !wasPresent && present;
    const becameReadable = initialized && !wasReadable && readable;

    if (targetChanged) {
      cancelAllAttempts();
      clearUnreadMarker();
    }

    if (!visible || !readable || !targetId) {
      cancelAllAttempts();
    } else if (
      !initialized ||
      targetChanged ||
      foregroundChanged ||
      becamePresent ||
      becameReadable
    ) {
      startLifecycleAttempt(targetId);
    }

    if (foregroundChanged && visible && readable) {
      retryAttemptNow(explicitAttempt);
    }

    if (onlineChanged && visible && readable) {
      retryAttemptNow(lifecycleAttempt);
      retryAttemptNow(explicitAttempt);
    }

    initialized = true;
    lastTargetId = targetId;
    wasPresent = present;
    wasReadable = readable;
    lastForegroundRevision = foregroundRevision;
    lastOnlineRevision = onlineRevision;
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

  onDestroy(() => {
    destroyed = true;
    cancelAllAttempts();
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
