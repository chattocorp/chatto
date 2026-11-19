import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { render } from 'vitest-browser-svelte';
import EventListTestHarness from './EventListTestHarness.svelte';
import '../../../../app.css';

// Keep the real virtualizer to exercise row removal at a day boundary.

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

describe('EventList date headers', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-06-17T12:00:00Z'));
    createTestServerScope({
      store: { realtimeSync: { isRecoveringSnapshot: false } },
      serverInfo: { messageEditWindowSeconds: 300 }
    });
  });

  afterEach(() => vi.useRealTimers());

  it.each([
    'update',
    'create',
    'room-delete',
    'deleted-join',
    'missing-join',
    'missing-leave'
  ] as const)('removes the empty day header despite a hidden %s event', async (kind) => {
    const view = render(EventListTestHarness, {
      props: {
        eventIds: ['yesterday'],
        eventKinds: { hidden: kind },
        eventDates: {
          hidden: '2026-06-17T09:00:00Z',
          yesterday: '2026-06-16T12:00:00Z',
          today: '2026-06-17T10:00:00Z'
        },
        scrollToEventId: null,
        hasReachedStart: true
      }
    });
    view.container.style.cssText = 'display: flex; flex-direction: column; height: 600px;';
    await expect.element(view.getByText('yesterday', { exact: true })).toBeVisible();
    await view.rerender({ eventIds: ['yesterday', 'hidden', 'today'] });
    await expect.element(view.getByRole('heading', { name: 'Today', exact: true })).toBeVisible();
    await view.rerender({ eventKinds: { hidden: kind, today: 'deleted' } });
    await expect
      .element(view.getByRole('heading', { name: 'Today', exact: true }))
      .not.toBeInTheDocument();
    await expect
      .element(view.getByRole('heading', { name: 'Yesterday', exact: true }))
      .toBeVisible();
    await expect.element(view.getByText('yesterday', { exact: true })).toBeVisible();
    await view.rerender({ eventKinds: { hidden: kind } });
    await expect.element(view.getByRole('heading', { name: 'Today', exact: true })).toBeVisible();
  });
});
