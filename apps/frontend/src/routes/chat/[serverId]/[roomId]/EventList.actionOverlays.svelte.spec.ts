import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { render } from 'vitest-browser-svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import '../../../../app.css';

// This spec renders the real virtua Virtualizer, so rows really unmount when they leave
// the rendered range.

vi.mock('./RoomEvent.svelte', async () => {
  const { default: RoomEvent } = await import('./EventListActionOverlayRoomEventMock.svelte');
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

function actionSheet() {
  return Array.from(document.querySelectorAll('dialog[open] button')).find(
    (item) => item.textContent?.trim() === 'Copy message link'
  );
}

describe('EventList message action overlays', () => {
  beforeEach(() => {
    createTestServerScope({
      store: { realtimeSync: { isRecoveringSnapshot: false } },
      serverInfo: { messageEditWindowSeconds: 300 }
    });
  });

  it('closes the overlay of the message that the user opened last', async () => {
    render(EventListTestHarness, {
      props: { eventIds: ['msg-0', 'msg-1'], scrollToEventId: null, hasReachedStart: true }
    });
    await vi.waitFor(() => expect(row('msg-1')).not.toBeNull());

    row('msg-0')!.click();
    await vi.waitFor(() => expect(actionSheet()).toBeTruthy());
    row('msg-1')!.click();
    await vi.waitFor(() => expect(document.querySelectorAll('dialog[open]')).toHaveLength(1));

    document.querySelector<HTMLDialogElement>('dialog[open]')!.close();
    await vi.waitFor(() => expect(actionSheet()).toBeUndefined());

    // The state closed too, so the next request opens a new sheet.
    row('msg-0')!.click();
    await vi.waitFor(() => expect(actionSheet()).toBeTruthy());
  });

  it('keeps an overlay open after its row unmounts and closes it when the message leaves', async () => {
    const eventIds = Array.from({ length: 60 }, (_, i) => `msg-${i}`);
    const rendered = render(EventListTestHarness, {
      props: { eventIds, scrollToEventId: null, hasReachedStart: true }
    });
    rendered.container.style.cssText = 'display: flex; flex-direction: column; height: 200px;';
    await vi.waitFor(() => expect(row('msg-59')).not.toBeNull());

    row('msg-59')!.click();
    await vi.waitFor(() => expect(actionSheet()).toBeTruthy());

    // The timeline follows its bottom, so each attempt scrolls to the top again.
    const scroller = rendered.container.querySelector<HTMLElement>(
      '[data-testid="messages-container"]'
    )!;
    await vi.waitFor(async () => {
      scroller.scrollTop = 0;
      await nextFrame();
      expect(row('msg-0')).not.toBeNull();
      expect(row('msg-59')).toBeNull();
    });
    expect(actionSheet()).toBeTruthy();

    await rendered.rerender({ eventIds: eventIds.slice(0, -1) });
    await vi.waitFor(() => expect(actionSheet()).toBeUndefined());
  });
});
