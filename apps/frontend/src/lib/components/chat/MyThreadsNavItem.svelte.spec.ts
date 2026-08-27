import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';

const mocks = vi.hoisted(() => ({
  unreadOccurrences: [] as Array<{ room: null; eventId: string; threadRootId: string | null }>,
  threadViewerStates: new Map<string, { isFollowing?: boolean; hasUnread?: boolean }>()
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
      projection: { threadViewerStates: mocks.threadViewerStates }
    }
  })
}));

import MyThreadsNavItem from './MyThreadsNavItem.svelte';

describe('MyThreadsNavItem', () => {
  beforeEach(async () => {
    mocks.unreadOccurrences = [];
    mocks.threadViewerStates.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('uses a neutral dot for unread followed-thread Badge state', () => {
    mocks.threadViewerStates.set('room-1\u0000root-1', { isFollowing: true, hasUnread: true });

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const dot = container.querySelector('[data-testid="my-threads-unread-dot"]');
    expect(dot?.classList).toContain('bg-neutral-action');
  });

  it('uses notification orange when a notification occurrence also exists', () => {
    mocks.threadViewerStates.set('room-1\u0000root-1', { isFollowing: true, hasUnread: true });
    mocks.unreadOccurrences = [{ room: null, eventId: 'reply-1', threadRootId: 'root-1' }];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const dot = container.querySelector('[data-testid="my-threads-unread-dot"]');
    expect(dot?.classList).toContain('bg-attention');
  });

  it('ignores Badge state for a thread that is not followed', () => {
    mocks.threadViewerStates.set('room-1\u0000root-1', { isFollowing: false, hasUnread: true });

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    expect(container.querySelector('[data-testid="my-threads-unread-dot"]')).toBeNull();
  });
});
