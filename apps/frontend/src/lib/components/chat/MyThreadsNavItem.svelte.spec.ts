import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';

const mocks = vi.hoisted(() => ({
  unreadOccurrences: [] as Array<{
    room: { id: string } | null;
    eventId: string;
    threadRootId: string | null;
    attentionLevel: number;
  }>,
  threadFollowStates: new Map<string, boolean>(),
  hasUnreadFollowedThread: false
}));

vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: (path: string) => path
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'server-1',
    store: {
      notifications: {
        get unreadOccurrences() {
          return mocks.unreadOccurrences;
        }
      },
      loadedThreadFollowState: (roomId: string, threadRootEventId: string) =>
        mocks.threadFollowStates.get(`${roomId}\u0000${threadRootEventId}`) ?? null,
      hasUnreadFollowedThreadInLoadedRooms: () => mocks.hasUnreadFollowedThread
    }
  })
}));

import MyThreadsNavItem from './MyThreadsNavItem.svelte';

describe('MyThreadsNavItem', () => {
  beforeEach(async () => {
    mocks.unreadOccurrences = [];
    mocks.threadFollowStates.clear();
    mocks.hasUnreadFollowedThread = false;
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('uses a neutral dot for unread replies', async () => {
    mocks.hasUnreadFollowedThread = true;

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const dot = await waitForDot(container);
    expect(dot?.classList).toContain('bg-neutral-action');
  });

  it('uses notification orange when a notification occurrence also exists', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', true);
    mocks.unreadOccurrences = [
      {
        room: { id: 'room-1' },
        eventId: 'reply-1',
        threadRootId: 'root-1',
        attentionLevel: 2
      }
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const dot = await waitForDot(container);
    expect(dot?.classList).toContain('bg-attention');
  });

  it('uses a neutral dot for an Ambient notification occurrence', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', true);
    mocks.unreadOccurrences = [
      {
        room: { id: 'room-1' },
        eventId: 'reply-1',
        threadRootId: 'root-1',
        attentionLevel: 1
      }
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const dot = await waitForDot(container);
    expect(dot?.classList).toContain('bg-neutral-action');
  });

  it('ignores notification attention for a thread that is not followed', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', false);
    mocks.unreadOccurrences = [
      {
        room: { id: 'room-1' },
        eventId: 'reply-1',
        threadRootId: 'root-1',
        attentionLevel: 2
      }
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    expect(container.querySelector('[data-testid="my-threads-unread-dot"]')).toBeNull();
  });

  it('does not show unread reply state when no thread is followed', async () => {
    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    expect(container.querySelector('[data-testid="my-threads-unread-dot"]')).toBeNull();
  });

  it('marks the active route semantically for the shared sidebar item treatment', async () => {
    const { container } = render(MyThreadsNavItem, { props: { active: true } });

    const link = container.querySelector('a');
    await expect.element(link).toHaveAttribute('aria-current', 'page');
    expect(link?.classList.contains('sidebar-item')).toBe(true);
    expect(link?.classList.contains('bg-surface')).toBe(false);
  });
});

async function waitForDot(container: HTMLElement): Promise<Element> {
  let dot: Element | null = null;
  await vi.waitFor(() => {
    dot = container.querySelector('[data-testid="my-threads-unread-dot"]');
    expect(dot).not.toBeNull();
  });
  return dot!;
}
