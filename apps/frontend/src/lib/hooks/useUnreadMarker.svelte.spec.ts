import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import Harness from './UseUnreadMarkerHarness.svelte';

type HarnessAPI = {
  readonly unreadMarkerEventId: string | null;
  noteAwayEvent(eventId: string): void;
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

  it('does not mark the same target as read again on refocus without an away event', async () => {
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

    expect(markAsRead).toHaveBeenCalledOnce();
    rendered.unmount();
  });

  it('marks the same target as read on refocus when an away event was captured', async () => {
    const markAsRead = vi.fn().mockResolvedValue(null);
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
    currentApi.noteAwayEvent('event-2');
    setPresent(true);

    await vi.waitFor(() => expect(markAsRead).toHaveBeenCalledTimes(2));
    expect(markAsRead).toHaveBeenLastCalledWith('room-1', undefined);
    expect(currentApi.unreadMarkerEventId).toBe('event-2');
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
