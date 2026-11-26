import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  NotificationAttentionLevel,
  NotificationSignalKind
} from '@chatto/client/api/notifications';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = {
  unreadOccurrences: [] as ReturnType<typeof occurrence>[],
  threadFollowStates: new Map<string, boolean>()
};

vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: (path: string) => path
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

import MyThreadsNavItem from './MyThreadsNavItem.svelte';

describe('MyThreadsNavItem', () => {
  beforeEach(async () => {
    mocks.unreadOccurrences = [];
    mocks.threadFollowStates.clear();
    createTestServerScope({
      store: {
        loadedThreadFollowState: (roomId: string, threadRootEventId: string) =>
          mocks.threadFollowStates.get(`${roomId}\u0000${threadRootEventId}`) ?? null
      },
      ui: {
        attention: {
          get occurrences() {
            return mocks.unreadOccurrences;
          }
        }
      }
    });
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('shows an Important count badge for a followed-thread notification', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', true);
    mocks.unreadOccurrences = [
      occurrence('reply-1', 'root-1', NotificationAttentionLevel.IMPORTANT)
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const badge = await waitForTestId(container, 'my-threads-notification-badge');
    expect(badge.textContent).toBe('1');
    expect(badge.classList).toContain('bg-attention');
    expect(container.querySelector('a')?.textContent).toContain('1 notification');
    expect(container.querySelector('a')?.textContent).not.toContain('1 notifications');
  });

  it('counts only the Ambient reply when an Important reaction targets the same thread', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', true);
    mocks.unreadOccurrences = [
      occurrence('reply-1', 'root-1', NotificationAttentionLevel.AMBIENT),
      occurrence(
        'reaction',
        'root-1',
        NotificationAttentionLevel.IMPORTANT,
        NotificationSignalKind.REACTION
      )
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const badge = await waitForTestId(container, 'my-threads-notification-badge');
    expect(badge.textContent).toBe('1');
    expect(badge.classList).toContain('bg-text');
  });

  it('hides the badge when a reaction is the only followed-thread notification', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', true);
    mocks.unreadOccurrences = [
      occurrence(
        'reaction',
        'root-1',
        NotificationAttentionLevel.AMBIENT,
        NotificationSignalKind.REACTION
      )
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    expect(container.querySelector('[data-testid="my-threads-notification-badge"]')).toBeNull();
  });

  it('counts notification occurrences across followed threads only', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', true);
    mocks.threadFollowStates.set('room-1\u0000root-2', true);
    mocks.threadFollowStates.set('room-1\u0000root-3', false);
    mocks.unreadOccurrences = [
      occurrence('reply-1', 'root-1', NotificationAttentionLevel.AMBIENT),
      occurrence('reply-2', 'root-1', NotificationAttentionLevel.AMBIENT),
      occurrence('reply-3', 'root-2', NotificationAttentionLevel.IMPORTANT),
      occurrence('reply-4', 'root-3', NotificationAttentionLevel.IMPORTANT)
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    const badge = await waitForTestId(container, 'my-threads-notification-badge');
    expect(badge.textContent).toBe('3');
    expect(badge.classList).toContain('bg-attention');
    expect(container.querySelector('a')?.textContent).toContain('3 notifications');
  });

  it('ignores notification attention for a thread that is not followed', async () => {
    mocks.threadFollowStates.set('room-1\u0000root-1', false);
    mocks.unreadOccurrences = [
      occurrence('reply-1', 'root-1', NotificationAttentionLevel.IMPORTANT)
    ];

    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    expect(container.querySelector('[data-testid="my-threads-notification-badge"]')).toBeNull();
  });

  it('does not show a badge without unread notifications', async () => {
    const { container } = render(MyThreadsNavItem, { props: { active: false } });

    expect(container.querySelector('[data-testid="my-threads-notification-badge"]')).toBeNull();
  });

  it('marks the active route semantically for the shared sidebar item treatment', async () => {
    const { container } = render(MyThreadsNavItem, { props: { active: true } });

    const link = container.querySelector('a');
    await expect.element(link).toHaveAttribute('aria-current', 'page');
    expect(link?.classList.contains('sidebar-item')).toBe(true);
    expect(link?.classList.contains('bg-surface')).toBe(false);
  });
});

function occurrence(
  eventId: string,
  threadRootId: string,
  attentionLevel: NotificationAttentionLevel,
  signalKind: NotificationSignalKind = NotificationSignalKind.REPLY
) {
  return { room: { id: 'room-1' }, eventId, threadRootId, attentionLevel, signalKind };
}

async function waitForTestId(container: HTMLElement, testid: string): Promise<Element> {
  let element: Element | null = null;
  await vi.waitFor(() => {
    element = container.querySelector(`[data-testid="${testid}"]`);
    expect(element).not.toBeNull();
  });
  return element!;
}
