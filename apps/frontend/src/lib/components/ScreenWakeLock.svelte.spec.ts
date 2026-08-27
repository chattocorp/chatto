import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ScreenWakeLock from './ScreenWakeLock.svelte';

class MockWakeLockSentinel extends EventTarget implements WakeLockSentinel {
  released = false;
  readonly type = 'screen' as const;

  async release(): Promise<void> {
    if (this.released) return;
    this.released = true;
    this.dispatchEvent(new Event('release'));
  }

  onrelease: ((this: WakeLockSentinel, ev: Event) => unknown) | null = null;
}

describe('ScreenWakeLock', () => {
  const originalWakeLock = Object.getOwnPropertyDescriptor(navigator, 'wakeLock');
  const originalVisibility = Object.getOwnPropertyDescriptor(document, 'visibilityState');
  let visibilityState: DocumentVisibilityState;
  let sentinels: MockWakeLockSentinel[];
  let request: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    visibilityState = 'visible';
    sentinels = [];
    request = vi.fn(async () => {
      const sentinel = new MockWakeLockSentinel();
      sentinels.push(sentinel);
      return sentinel;
    });

    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => visibilityState
    });
    Object.defineProperty(navigator, 'wakeLock', {
      configurable: true,
      value: { request }
    });
  });

  afterEach(() => {
    if (originalWakeLock) {
      Object.defineProperty(navigator, 'wakeLock', originalWakeLock);
    } else {
      Reflect.deleteProperty(navigator, 'wakeLock');
    }
    if (originalVisibility) {
      Object.defineProperty(document, 'visibilityState', originalVisibility);
    }
  });

  it('holds a screen wake lock for its mounted lifetime', async () => {
    const rendered = render(ScreenWakeLock);

    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('screen'));
    expect(sentinels[0].released).toBe(false);

    rendered.unmount();
    await vi.waitFor(() => expect(sentinels[0].released).toBe(true));
  });

  it('reacquires the lock after the document becomes visible again', async () => {
    const rendered = render(ScreenWakeLock);
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));

    visibilityState = 'hidden';
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.waitFor(() => expect(sentinels[0].released).toBe(true));

    visibilityState = 'visible';
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(2));
    expect(sentinels[1].released).toBe(false);

    rendered.unmount();
  });

  it('releases an in-flight lock when unmounted before acquisition completes', async () => {
    let resolveRequest: ((sentinel: MockWakeLockSentinel) => void) | undefined;
    request.mockImplementation(
      () =>
        new Promise<MockWakeLockSentinel>((resolve) => {
          resolveRequest = resolve;
        })
    );
    const rendered = render(ScreenWakeLock);
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));

    rendered.unmount();
    const sentinel = new MockWakeLockSentinel();
    resolveRequest?.(sentinel);

    await vi.waitFor(() => expect(sentinel.released).toBe(true));
  });
});
