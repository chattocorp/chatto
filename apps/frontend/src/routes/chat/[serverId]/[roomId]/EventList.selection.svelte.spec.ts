import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { render } from 'vitest-browser-svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import '../../../../app.css';

// This spec renders the real virtua Virtualizer: the fix depends on its `itemProps`
// and `keepMounted` behavior, which the shared Virtualizer mock does not have.

vi.mock('./RoomEvent.svelte', async () => {
  const { default: RoomEvent } = await import('./EventListRoomEventMock.svelte');
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

function eventText(id: string) {
  return document.querySelector(`[data-event-id="${id}"]`)?.firstChild ?? null;
}

describe('EventList selection', () => {
  beforeEach(() => {
    createTestServerScope({
      store: { realtimeSync: { isRecoveringSnapshot: false } },
      serverInfo: { messageEditWindowSeconds: 300 }
    });
  });

  it('copies a selection that spans more than the rendered window', async () => {
    const eventIds = Array.from({ length: 60 }, (_, i) => `msg-${i}`);
    const { container } = render(EventListTestHarness, {
      props: { eventIds, scrollToEventId: null, hasReachedStart: true }
    });
    // A short viewport, so the virtualizer renders only some of the messages. The app
    // makes only message text selectable; the RoomEvent mock has no such class.
    container.style.cssText =
      'display: flex; flex-direction: column; height: 200px; user-select: text;';

    // The timeline opens at the newest message; the oldest ones are not rendered.
    await vi.waitFor(() => {
      expect(eventText('msg-59')).not.toBeNull();
      expect(eventText('msg-0')).toBeNull();
    });

    // Start the selection at the newest message, as a click before a Shift+click does.
    const anchor = eventText('msg-59')!;
    const selection = window.getSelection()!;
    selection.setBaseAndExtent(
      anchor,
      anchor.textContent!.length,
      anchor,
      anchor.textContent!.length
    );
    await nextFrame();

    const scroller = container.querySelector<HTMLElement>('[data-testid="messages-container"]')!;
    await vi.waitFor(async () => {
      scroller.scrollTop = 0;
      await nextFrame();
      expect(eventText('msg-0')).not.toBeNull();
    });

    expect(document.contains(anchor)).toBe(true);
    selection.setBaseAndExtent(anchor, anchor.textContent!.length, eventText('msg-0')!, 0);

    // The selection now spans all messages, so all of them are mounted and copied.
    await vi.waitFor(() => {
      const copied = selection
        .toString()
        .split('\n')
        .filter((line) => line.startsWith('msg-'));
      expect(copied).toEqual(eventIds);
    });

    selection.removeAllRanges();
  });
});
