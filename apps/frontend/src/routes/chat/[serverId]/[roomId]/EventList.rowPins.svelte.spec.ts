import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { render } from 'vitest-browser-svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import '../../../../app.css';

// This spec renders the real virtua Virtualizer: the fix depends on its `keepMounted`
// behavior, which the shared Virtualizer mock does not have.

vi.mock('./RoomEvent.svelte', async () => {
  const { default: RoomEvent } = await import('./EventListPinningRoomEventMock.svelte');
  return { default: RoomEvent };
});

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server-1'
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_userId: string, fallback: string | null) => fallback,
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

const nextFrame = () => new Promise((resolve) => requestAnimationFrame(resolve));

function row(id: string) {
  return document.querySelector<HTMLButtonElement>(`[data-event-id="${id}"]`);
}

describe('EventList row pins', () => {
  beforeEach(() => {
    createTestServerScope({
      store: { realtimeSync: { isRecoveringSnapshot: false } },
      serverInfo: { messageEditWindowSeconds: 300 }
    });
  });

  it('keeps a row with an open overlay mounted after it leaves the viewport', async () => {
    const eventIds = Array.from({ length: 60 }, (_, i) => `msg-${i}`);
    const { container } = render(EventListTestHarness, {
      props: { eventIds, scrollToEventId: null, hasReachedStart: true }
    });
    container.style.cssText = 'display: flex; flex-direction: column; height: 200px;';

    await vi.waitFor(() => {
      expect(row('msg-59')).not.toBeNull();
      expect(row('msg-0')).toBeNull();
    });

    // Open the overlay of the newest row, then scroll it far out of the rendered range.
    // The timeline follows its bottom, so each attempt scrolls to the top again.
    const pinned = row('msg-59')!;
    pinned.click();
    const scroller = container.querySelector<HTMLElement>('[data-testid="messages-container"]')!;
    const scrollToTop = async () => {
      scroller.scrollTop = 0;
      await nextFrame();
      expect(row('msg-0')).not.toBeNull();
    };
    await vi.waitFor(async () => {
      await scrollToTop();
      expect(row('msg-58')).toBeNull();
      expect(row('msg-59')).toBe(pinned);
    });

    // Closing the overlay releases the row.
    pinned.click();
    await vi.waitFor(async () => {
      await scrollToTop();
      expect(row('msg-59')).toBeNull();
    });
  });
});
