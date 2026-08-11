import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { NotificationReason, type NotificationOccurrenceItem } from '$lib/api-client/notifications';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { getToasts, toast } from '$lib/ui/toast';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    servers: [{ id: 'origin', url: 'https://chat.example.test' }],
    stores: new Map<string, unknown>(),
    appUi: { disableRoomCallWideFor: vi.fn() },
    occurrence: {
      id: 'mention-1',
      sourceEventId: 'source-1',
      createdAt: new Date().toISOString(),
      actor: null,
      room: { id: 'room-1', name: 'general' },
      eventId: 'event-1',
      threadRootId: 'thread-1',
      parentEventId: null,
      reasons: [2],
      reasonMatches: [{ reason: 2, intensity: 3 }],
      unread: true
    },
    store: {
      isAuthenticated: true,
      currentUser: {
        user: {
          settings: null as {
            timezone?: string | null;
            timeFormat: TimeFormat;
          } | null
        }
      },
      notifications: {
        viewInvalidationVersion: 0,
        fetchPage: vi.fn(),
        markOccurrenceRead: vi.fn().mockResolvedValue(undefined),
        deleteGroup: vi.fn().mockResolvedValue(undefined)
      },
      pendingHighlights: { set: vi.fn() }
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn()
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    servers: mocks.servers,
    getStore: vi.fn((serverId: string) => mocks.stores.get(serverId)),
    isOriginServer: vi.fn((serverId: string) => serverId === 'origin'),
    getServer: vi.fn((serverId: string) => mocks.servers.find((server) => server.id === serverId))
  }
}));

vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => mocks.appUi
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: number) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

import NotificationsPage from './+page.svelte';

function group(
  id = 'group-1',
  occurrence: NotificationOccurrenceItem = mocks.occurrence as NotificationOccurrenceItem,
  unread = occurrence.unread
) {
  return {
    id,
    occurrences: [occurrence],
    openTarget: occurrence,
    unread,
    occurrenceCount: 1,
    latestAt: occurrence.createdAt,
    reasons: occurrence.reasons
  };
}

function page(groups = [group()], hasMore = false) {
  return {
    groups,
    unreadGroupCount: groups.filter((candidate) => candidate.unread).length,
    roomUnreadGroupCounts: {},
    totalCount: groups.length,
    hasMore
  };
}

describe('notifications page', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    toast.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.store.notifications.viewInvalidationVersion = 0;
    mocks.store.notifications.fetchPage.mockResolvedValue(page());
    mocks.store.notifications.deleteGroup.mockResolvedValue(undefined);
    mocks.servers.splice(0, mocks.servers.length, {
      id: 'origin',
      url: 'https://chat.example.test'
    });
    mocks.stores.clear();
    mocks.stores.set('origin', mocks.store);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('queues an unread occurrence to be marked read after its target is displayed', async () => {
    const { container } = render(NotificationsPage);
    const item = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"] > button');
      expect(element).not.toBeNull();
      return element as HTMLButtonElement;
    });

    item.click();

    await vi.waitFor(() => {
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'room-1');
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'room-1',
        'thread-1',
        'event-1',
        'mention-1'
      );
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-1/thread-1');
    });
    expect(mocks.store.notifications.markOccurrenceRead).not.toHaveBeenCalled();
  });

  it('uses the notification orange and exposes only the delete action', async () => {
    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"]');
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });
    const rowTarget = q(row, ':scope > button') as HTMLButtonElement;
    const deleteButton = q(row, 'button[aria-label="Delete"]') as HTMLButtonElement;

    expect(row.classList.contains('cursor-pointer')).toBe(true);
    expect(row.classList.contains('bg-attention/5')).toBe(true);
    expect(q(row, '.bg-attention')).not.toBeNull();
    expect(rowTarget.classList.contains('cursor-pointer')).toBe(true);
    expect(deleteButton.classList.contains('btn-danger-secondary')).toBe(true);
    expect(row.querySelectorAll('button')).toHaveLength(2);
    expect(row.textContent).not.toContain('Move to inbox');
    expect(row.textContent).not.toContain('Mark done');
  });

  it('renders read and unread notifications in one list', async () => {
    const readOccurrence = {
      ...mocks.occurrence,
      id: 'read-1',
      unread: false,
      createdAt: new Date(Date.now() - 60_000).toISOString()
    };
    mocks.store.notifications.fetchPage.mockResolvedValue(
      page([group('unread', mocks.occurrence, true), group('read', readOccurrence, false)])
    );

    const { container } = render(NotificationsPage);
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
    });

    expect(mocks.store.notifications.fetchPage).toHaveBeenCalledTimes(1);
    const readRow = q(container, '[data-notification-state="read"]') as HTMLElement;
    expect(readRow.classList.contains('bg-attention/5')).toBe(false);
    expect(q(readRow, '.bg-attention')).toBeNull();
  });

  it('spaces title metadata and omits the single-occurrence counter', async () => {
    const actor = {
      id: 'alice',
      login: 'alice',
      displayName: 'Alice',
      deleted: false,
      avatarUrl: null,
      presenceStatus: 1,
      customStatus: null
    };
    const occurrence = {
      ...mocks.occurrence,
      actor,
      reasons: [NotificationReason.FOLLOWED_THREAD],
      reasonMatches: [{ reason: NotificationReason.FOLLOWED_THREAD, intensity: 2 }]
    };
    mocks.store.notifications.fetchPage.mockResolvedValue(page([group('followed', occurrence)]));

    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"]');
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });

    expect(row.querySelectorAll('[aria-hidden="true"].mx-1\\.5').length).toBeGreaterThan(0);
    expect(row.textContent).toContain('Alice');
    expect(row.textContent).toContain('Followed threads');
    expect(row.textContent).not.toMatch(/·\s*1\s*·/);
  });

  it('preserves a healthy server result when another server fails', async () => {
    const remoteStore = {
      ...mocks.store,
      notifications: {
        ...mocks.store.notifications,
        fetchPage: vi.fn().mockRejectedValue(new Error('remote offline'))
      }
    };
    mocks.servers.push({ id: 'remote', url: 'https://remote.example.test' });
    mocks.stores.set('remote', remoteStore);

    const { container } = render(NotificationsPage);

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
      expect(container.querySelector('[role="alert"]')).not.toBeNull();
    });
    expect(container.textContent).toContain('Network error. Please try again.');
    expect(container.textContent).toContain('chat.example.test');
  });

  it('fences row opening while dismissal is pending and reports failures', async () => {
    let rejectMutation: ((reason?: unknown) => void) | undefined;
    mocks.store.notifications.deleteGroup.mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectMutation = reject;
        })
    );
    const { container } = render(NotificationsPage);
    const deleteButton = await vi.waitFor(() => {
      const element = q(container, 'button[aria-label="Delete"]');
      expect(element).not.toBeNull();
      return element as HTMLButtonElement;
    });
    const rowButton = q(
      container,
      '[data-testid="notification-group"] > button'
    ) as HTMLButtonElement;

    deleteButton.click();
    await vi.waitFor(() => expect(rowButton.disabled).toBe(true));
    rowButton.click();
    expect(mocks.goto).not.toHaveBeenCalled();

    rejectMutation?.(new Error('offline'));
    await vi.waitFor(() => {
      expect(getToasts().at(-1)?.message).toBe('Network error. Please try again.');
      expect(rowButton.disabled).toBe(false);
    });
  });

  it('loads the next page from each server independently', async () => {
    let intersectionCallback: IntersectionObserverCallback | undefined;
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(callback: IntersectionObserverCallback) {
          intersectionCallback = callback;
        }
        observe() {}
        unobserve() {}
        disconnect() {}
        takeRecords() {
          return [];
        }
        root = null;
        rootMargin = '';
        thresholds = [];
      }
    );
    mocks.store.notifications.fetchPage.mockImplementation((offset = 0) => {
      const occurrence = {
        ...mocks.occurrence,
        id: `notification-${offset}`,
        createdAt: `2026-08-11T12:0${offset}:00Z`
      };
      return Promise.resolve(page([group(`group-${offset}`, occurrence)], offset === 0));
    });

    const { container } = render(NotificationsPage);
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
    });
    intersectionCallback?.(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver
    );
    await vi.waitFor(() => {
      expect(mocks.store.notifications.fetchPage).toHaveBeenCalledWith(1);
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
    });
  });
});
