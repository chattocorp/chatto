type TimelineEventJumpAttempt = {
  cancel: () => void;
};

type TimelineEventJumpOptions = {
  targetEventId: string;
  afterRender: () => Promise<void>;
  getTargetIndex: () => number;
  scrollToIndex: (index: number) => void;
  getScope: () => ParentNode;
  measureDistanceFromBottom: () => number | null;
  onSettle: (distanceFromBottom: number) => void;
  onComplete?: (landed: boolean) => void;
  maxRetries?: number;
  settleDelayMs?: number;
  requestFrame?: (callback: FrameRequestCallback) => unknown;
  schedule?: (callback: () => void, delayMs: number) => unknown;
  escapeSelectorValue?: (value: string) => string;
};

export function timelineEventSelector(
  eventId: string,
  escapeSelectorValue: (value: string) => string = CSS.escape
): string {
  return `[data-event-id="${escapeSelectorValue(eventId)}"]`;
}

/**
 * Runs one bounded, cancellable jump to a virtualized timeline event.
 *
 * The target may take several animation frames to become indexed and mounted
 * after a timeline-window replacement. Completion is reported only after the
 * successful scroll has settled, and cancellation fences all later callbacks.
 */
export function startTimelineEventJump({
  targetEventId,
  afterRender,
  getTargetIndex,
  scrollToIndex,
  getScope,
  measureDistanceFromBottom,
  onSettle,
  onComplete,
  maxRetries = 60,
  settleDelayMs = 200,
  requestFrame = (callback) => requestAnimationFrame(callback),
  schedule = (callback, delayMs) => setTimeout(callback, delayMs),
  escapeSelectorValue
}: TimelineEventJumpOptions): TimelineEventJumpAttempt {
  let cancelled = false;
  let completed = false;
  let retries = 0;
  const selector = timelineEventSelector(targetEventId, escapeSelectorValue);

  function complete(landed: boolean): void {
    if (cancelled || completed) return;
    if (!landed) {
      completed = true;
      onComplete?.(false);
      return;
    }

    schedule(() => {
      if (cancelled || completed) return;
      const distanceFromBottom = measureDistanceFromBottom();
      if (distanceFromBottom === null) return;
      onSettle(distanceFromBottom);
      completed = true;
      onComplete?.(true);
    }, settleDelayMs);
  }

  function tryScrollAndHighlight(): void {
    if (cancelled || completed) return;

    const targetIndex = getTargetIndex();
    if (targetIndex !== -1) scrollToIndex(targetIndex);

    const target = getScope().querySelector<HTMLElement>(selector);
    if (target) {
      target.classList.add('highlight-flash');
      target.addEventListener('animationend', () => target.classList.remove('highlight-flash'), {
        once: true
      });
      complete(true);
      return;
    }

    if (retries >= maxRetries) {
      complete(false);
      return;
    }
    retries += 1;
    requestFrame(tryScrollAndHighlight);
  }

  void afterRender().then(() => {
    if (!cancelled) requestFrame(tryScrollAndHighlight);
  });

  return {
    cancel: () => {
      cancelled = true;
    }
  };
}
