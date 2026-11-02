import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import '../../../../app.css';

vi.mock('./RoomEvent.svelte', async () => ({
  default: (await import('./EventListDiagramRoomEventMock.svelte')).default
}));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: () => 'server-1' }));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

it('settles diagram rows in a thread', async () => {
  createTestServerScope({
    store: { realtimeSync: { isRecoveringSnapshot: false } },
    serverInfo: { messageEditWindowSeconds: 300 }
  });
  const view = render(EventListTestHarness, {
    eventIds: Array.from({ length: 30 }, (_, i) => `msg-${i}`),
    scrollToEventId: null,
    hasReachedStart: true,
    permalinkThreadRootEventId: 'thread-root'
  });
  view.container.style.cssText = 'display:flex;flex-direction:column;height:400px;width:360px';
  const scroller = view.container.querySelector<HTMLElement>('[data-testid="messages-container"]')!;
  await expect
    .poll(() => scroller.querySelector('[data-event-id="msg-29"] img')?.getAttribute('src'))
    .toBeTruthy();
  for (const top of [scroller.scrollHeight, 0, 1500, 3000, scroller.scrollHeight]) {
    scroller.dispatchEvent(new WheelEvent('wheel', { deltaY: -1000, bubbles: true }));
    scroller.scrollTop = top;
    const samples = [];
    for (let i = 0; i < 90; i++) {
      await new Promise(requestAnimationFrame);
      samples.push(
        `${scroller.scrollHeight}/${scroller.scrollTop}/${scroller.querySelectorAll('img').length}`
      );
    }
    expect(new Set(samples.slice(-30)).size, samples.slice(-20).join('\n')).toBe(1);
  }
});
