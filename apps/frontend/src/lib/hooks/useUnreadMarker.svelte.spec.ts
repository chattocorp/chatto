import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import Harness from './UseUnreadMarkerHarness.svelte';

type HarnessAPI = {
  readonly unreadMarkerEventId: string | null;
  readonly unreadMarkerWindow: {
    afterTime: string;
    beforeTime: string | number;
  } | null;
  markAsRead(targetId: string, upToEventId?: string): Promise<unknown>;
  setUnreadMarkerEventId(eventId: string | null): void;
};

function getApi(api: HarnessAPI | undefined): HarnessAPI {
  if (!api) {
    throw new Error('Unread marker harness API was not initialized');
  }
  return api;
}

function setVisibility(value: DocumentVisibilityState): void {
  Object.defineProperty(document, 'visibilityState', {
    value,
    writable: true,
    configurable: true
  });
  document.dispatchEvent(new Event('visibilitychange'));
}

function setPresent(present: boolean): void {
  window.dispatchEvent(new Event(present ? 'focus' : 'blur'));
  setVisibility(present ? 'visible' : 'hidden');
  flushSync();
}

describe('useUnreadMarker', () => {
  beforeEach(() => {
    setPresent(true);
  });

  afterEach(() => {
    setPresent(true);
    vi.restoreAllMocks();
  });

  it('marks the same target as read again on refocus', async () => {
    const markAsRead = vi.fn().mockResolvedValue(null);

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: () => {}
      }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    setPresent(false);
    setPresent(true);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    expect(markAsRead).toHaveBeenLastCalledWith('room-1', undefined);
    rendered.unmount();
  });

  it('uses the read-state window returned on refocus', async () => {
    const markedAtMs = Date.UTC(2026, 6, 8, 10, 0, 30);
    vi.spyOn(Date, 'now').mockReturnValue(markedAtMs);
    const markAsRead = vi
      .fn()
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({
        previousLastReadAt: '2026-07-08T09:00:00.000Z',
        lastReadAt: '2026-07-08T10:00:00.000Z'
      });
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    setPresent(false);
    const currentApi = getApi(api);
    setPresent(true);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(currentApi.unreadMarkerWindow).toEqual({
        afterTime: '2026-07-08T09:00:00.000Z',
        beforeTime: markedAtMs
      })
    );
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('clears the marker when refocus returns no previous read state', async () => {
    const markedAtMs = Date.UTC(2026, 6, 8, 10, 0, 30);
    vi.spyOn(Date, 'now').mockReturnValue(markedAtMs);
    const markAsRead = vi
      .fn()
      .mockResolvedValueOnce({
        previousLastReadAt: '2026-07-08T09:00:00.000Z',
        lastReadAt: '2026-07-08T10:00:00.000Z'
      })
      .mockResolvedValueOnce({
        previousLastReadAt: null,
        lastReadAt: '2026-07-08T10:05:00.000Z'
      });
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    const currentApi = getApi(api);
    await vi.waitFor(() =>
      expect(currentApi.unreadMarkerWindow).toEqual({
        afterTime: '2026-07-08T09:00:00.000Z',
        beforeTime: markedAtMs
      })
    );

    currentApi.setUnreadMarkerEventId('event-2');
    setPresent(false);
    setPresent(true);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(currentApi.unreadMarkerWindow).toBeNull());
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('does not create a marker window when the read cursor did not advance', async () => {
    const markAsRead = vi.fn().mockResolvedValueOnce({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T09:00:00.000Z'
    });
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    const currentApi = getApi(api);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());
    expect(currentApi.unreadMarkerWindow).toBeNull();
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('preserves a pending refocus marker after a newer explicit read', async () => {
    const markedAtMs = Date.UTC(2026, 6, 8, 10, 0, 30);
    vi.spyOn(Date, 'now').mockReturnValue(markedAtMs);
    let resolveRefocus!: (value: {
      previousLastReadAt: string;
      lastReadAt: string;
    }) => void;
    const refocusRead = new Promise<{
      previousLastReadAt: string;
      lastReadAt: string;
    }>((resolve) => {
      resolveRefocus = resolve;
    });
    const markAsRead = vi
      .fn()
      .mockResolvedValueOnce(null)
      .mockReturnValueOnce(refocusRead)
      .mockResolvedValueOnce(null);
    let api: HarnessAPI | undefined;

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: (nextApi: HarnessAPI) => {
          api = nextApi;
        }
      }
    });
    flushSync();
    const currentApi = getApi(api);
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    setPresent(false);
    setPresent(true);
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));

    await currentApi.markAsRead('room-1', 'event-2');
    expect(markAsRead).toHaveBeenCalledTimes(3);

    resolveRefocus({
      previousLastReadAt: '2026-07-08T09:00:00.000Z',
      lastReadAt: '2026-07-08T10:00:00.000Z'
    });
    await Promise.resolve();
    flushSync();

    expect(currentApi.unreadMarkerWindow).toEqual({
      afterTime: '2026-07-08T09:00:00.000Z',
      beforeTime: markedAtMs
    });
    expect(currentApi.unreadMarkerEventId).toBeNull();
    rendered.unmount();
  });

  it('marks a new target as read when the target changes', async () => {
    const markAsRead = vi.fn().mockResolvedValue(null);

    const rendered = render(Harness, {
      props: {
        targetId: 'room-1',
        markAsRead,
        onReady: () => {}
      }
    });
    flushSync();
    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledOnce());

    await rendered.rerender({
      targetId: 'room-2',
      markAsRead,
      onReady: () => {}
    });
    flushSync();

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    expect(markAsRead).toHaveBeenLastCalledWith('room-2', undefined);
    rendered.unmount();
  });
});
