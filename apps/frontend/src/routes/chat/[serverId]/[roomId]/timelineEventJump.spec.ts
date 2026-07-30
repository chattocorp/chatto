import { describe, expect, it, vi } from 'vitest';
import { startTimelineEventJump, timelineEventSelector } from './timelineEventJump';

type JumpHarness = ReturnType<typeof createJumpHarness>;

function createTarget() {
  let animationEnd: (() => void) | null = null;
  const target = {
    classList: {
      add: vi.fn(),
      remove: vi.fn()
    },
    addEventListener: vi.fn(
      (type: string, callback: () => void, options?: AddEventListenerOptions | boolean) => {
        if (type === 'animationend') animationEnd = callback;
        expect(options).toEqual({ once: true });
      }
    )
  } as unknown as HTMLElement;

  return {
    target,
    finishAnimation: () => animationEnd?.()
  };
}

function createJumpHarness({
  targetIndex = 2,
  distanceFromBottom = 25
}: {
  targetIndex?: number;
  distanceFromBottom?: number | null;
} = {}) {
  const frames: FrameRequestCallback[] = [];
  const timers: Array<() => void> = [];
  const scrollToIndex = vi.fn();
  const onSettle = vi.fn();
  const onComplete = vi.fn();
  const querySelector = vi.fn<() => HTMLElement | null>(() => null);
  const scope = { querySelector } as unknown as ParentNode;

  return {
    frames,
    timers,
    scrollToIndex,
    onSettle,
    onComplete,
    querySelector,
    scope,
    options: {
      targetEventId: 'message:1',
      afterRender: () => Promise.resolve(),
      getTargetIndex: () => targetIndex,
      scrollToIndex,
      getScope: () => scope,
      measureDistanceFromBottom: () => distanceFromBottom,
      onSettle,
      onComplete,
      requestFrame: (callback: FrameRequestCallback) => frames.push(callback),
      schedule: (callback: () => void) => timers.push(callback),
      escapeSelectorValue: (value: string) => value.replace(':', '\\:')
    },
    runNextFrame() {
      const frame = frames.shift();
      expect(frame).toBeDefined();
      frame?.(0);
    },
    runNextTimer() {
      const timer = timers.shift();
      expect(timer).toBeDefined();
      timer?.();
    }
  };
}

async function start(harness: JumpHarness) {
  const attempt = startTimelineEventJump(harness.options);
  await Promise.resolve();
  return attempt;
}

describe('startTimelineEventJump', () => {
  it('scrolls, highlights, settles, and completes a mounted target', async () => {
    const harness = createJumpHarness();
    const target = createTarget();
    harness.querySelector.mockReturnValue(target.target);

    await start(harness);
    harness.runNextFrame();

    expect(harness.scrollToIndex).toHaveBeenCalledExactlyOnceWith(2);
    expect(harness.querySelector).toHaveBeenCalledExactlyOnceWith('[data-event-id="message\\:1"]');
    expect(target.target.classList.add).toHaveBeenCalledExactlyOnceWith('highlight-flash');
    expect(harness.onComplete).not.toHaveBeenCalled();

    harness.runNextTimer();

    expect(harness.onSettle).toHaveBeenCalledExactlyOnceWith(25);
    expect(harness.onComplete).toHaveBeenCalledExactlyOnceWith(true);

    target.finishAnimation();
    expect(target.target.classList.remove).toHaveBeenCalledExactlyOnceWith('highlight-flash');
  });

  it('retries until the target is mounted', async () => {
    const harness = createJumpHarness();
    const target = createTarget();
    harness.querySelector
      .mockReturnValueOnce(null)
      .mockReturnValueOnce(null)
      .mockReturnValue(target.target);

    await start(harness);
    harness.runNextFrame();
    harness.runNextFrame();
    harness.runNextFrame();
    harness.runNextTimer();

    expect(harness.querySelector).toHaveBeenCalledTimes(3);
    expect(harness.scrollToIndex).toHaveBeenCalledTimes(3);
    expect(harness.onComplete).toHaveBeenCalledExactlyOnceWith(true);
  });

  it('reports failure after the bounded retry budget', async () => {
    const harness = createJumpHarness();

    const attempt = startTimelineEventJump({
      ...harness.options,
      maxRetries: 2
    });
    await Promise.resolve();
    harness.runNextFrame();
    harness.runNextFrame();
    harness.runNextFrame();

    expect(harness.querySelector).toHaveBeenCalledTimes(3);
    expect(harness.onComplete).toHaveBeenCalledExactlyOnceWith(false);
    expect(harness.timers).toHaveLength(0);
    attempt.cancel();
  });

  it('does not scroll or complete after cancellation', async () => {
    const harness = createJumpHarness();
    const attempt = await start(harness);

    attempt.cancel();
    harness.runNextFrame();

    expect(harness.scrollToIndex).not.toHaveBeenCalled();
    expect(harness.querySelector).not.toHaveBeenCalled();
    expect(harness.onComplete).not.toHaveBeenCalled();
  });

  it('fences settling and completion when cancelled after highlighting', async () => {
    const harness = createJumpHarness();
    const target = createTarget();
    harness.querySelector.mockReturnValue(target.target);
    const attempt = await start(harness);
    harness.runNextFrame();

    attempt.cancel();
    harness.runNextTimer();

    expect(harness.onSettle).not.toHaveBeenCalled();
    expect(harness.onComplete).not.toHaveBeenCalled();
  });

  it('does not complete when the virtualizer cannot be measured while settling', async () => {
    const harness = createJumpHarness({ distanceFromBottom: null });
    const target = createTarget();
    harness.querySelector.mockReturnValue(target.target);

    await start(harness);
    harness.runNextFrame();
    harness.runNextTimer();

    expect(harness.onSettle).not.toHaveBeenCalled();
    expect(harness.onComplete).not.toHaveBeenCalled();
  });
});

describe('timelineEventSelector', () => {
  it('escapes event IDs for scoped DOM lookup', () => {
    expect(timelineEventSelector('message:1', (value) => value.replace(':', '\\:'))).toBe(
      '[data-event-id="message\\:1"]'
    );
  });
});
