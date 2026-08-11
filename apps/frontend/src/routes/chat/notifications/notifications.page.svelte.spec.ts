import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  NotificationInboxState,
  NotificationReason,
  NotificationView
} from '$lib/api-client/notifications';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { getToasts, toast } from '$lib/ui/toast';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    servers: [{ id: 'origin', url: 'https://chat.example.test' }],
    stores: new Map<string, unknown>(),
    appUi: {
      disableRoomCallWideFor: vi.fn()
    },
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
      inboxState: 1
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
      serverInfo: {
        name: 'Test Server'
      },
      notifications: {
        fetchView: vi.fn(),
        updateGroup: vi.fn().mockResolvedValue(undefined),
        markOccurrenceRead: vi.fn().mockResolvedValue(undefined),
        moveGroupToDone: vi.fn().mockResolvedValue(undefined),
        restoreGroupToInbox: vi.fn().mockResolvedValue(undefined),
        deleteGroup: vi.fn().mockResolvedValue(undefined)
      },
      pendingHighlights: {
        set: vi.fn()
      }
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

describe('notifications page', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    toast.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    const group = {
      id: 'group-1',
      occurrences: [mocks.occurrence],
      openTarget: mocks.occurrence,
      unread: true,
      occurrenceCount: 1,
      latestAt: mocks.occurrence.createdAt,
      reasons: [2]
    };
    mocks.store.notifications.fetchView.mockImplementation((view: NotificationView) =>
      Promise.resolve({
        groups: view === NotificationView.INBOX ? [group] : [],
        unreadGroupCount: 1,
        roomUnreadGroupCounts: {},
        totalCount: view === NotificationView.INBOX ? 1 : 0,
        hasMore: false
      })
    );
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

  it('queues the unread occurrence to be marked read after the target is displayed', async () => {
    const { container } = render(NotificationsPage);

    await vi.waitFor(() => {
      expect(q(container, '[data-testid="notification-group"] button')).not.toBeNull();
    });
    const item = q(container, '[data-testid="notification-group"] button') as HTMLElement;
    item.click();

    await vi.waitFor(() => {
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'room-1');
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
      );
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'room-1',
        'thread-1',
        'event-1',
        'mention-1'
      );
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-1/thread-1');
      expect(mocks.store.notifications.markOccurrenceRead).not.toHaveBeenCalled();
    });
  });

  it('uses a pointer row target and framed triage buttons', async () => {
    const { container } = render(NotificationsPage);

    await vi.waitFor(() => {
      expect(q(container, '[data-testid="notification-group"] button')).not.toBeNull();
    });
    const rowTarget = q(
      container,
      '[data-testid="notification-group"] > button'
    ) as HTMLButtonElement;
    const doneButton = q(container, 'button[aria-label="Mark done"]') as HTMLButtonElement;
    const deleteButton = q(container, 'button[aria-label="Delete"]') as HTMLButtonElement;

    expect(rowTarget.classList.contains('cursor-pointer')).toBe(true);
    expect(doneButton.classList.contains('btn-secondary')).toBe(true);
    expect(deleteButton.classList.contains('btn-danger-secondary')).toBe(true);
  });

  it('combines Inbox and Done groups and subdues handled rows', async () => {
    const doneOccurrence = {
      ...mocks.occurrence,
      id: 'mention-done',
      inboxState: NotificationInboxState.DONE,
      createdAt: new Date(Date.now() - 60_000).toISOString()
    };
    const inboxPage = await mocks.store.notifications.fetchView(NotificationView.INBOX);
    mocks.store.notifications.fetchView.mockImplementation((view: NotificationView) =>
      Promise.resolve({
        groups:
          view === NotificationView.INBOX
            ? inboxPage.groups
            : [
                {
                  id: 'group-done',
                  occurrences: [doneOccurrence],
                  openTarget: doneOccurrence,
                  unread: false,
                  occurrenceCount: 1,
                  latestAt: doneOccurrence.createdAt,
                  reasons: [2]
                }
              ],
        unreadGroupCount: 1,
        roomUnreadGroupCounts: {},
        totalCount: 1,
        hasMore: false
      })
    );

    const { container } = render(NotificationsPage);

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
    });
    expect(mocks.store.notifications.fetchView).toHaveBeenCalledWith(NotificationView.INBOX);
    expect(mocks.store.notifications.fetchView).toHaveBeenCalledWith(NotificationView.DONE);
    expect(container.textContent).not.toContain('InboxDone');

    const doneRow = q(
      container,
      '[data-testid="notification-group"][data-notification-state="done"]'
    ) as HTMLElement;
    expect(doneRow.classList.contains('opacity-60')).toBe(true);
    const restoreButton = q(doneRow, 'button[aria-label="Move to inbox"]') as HTMLButtonElement;
    expect(restoreButton.querySelector('span')?.classList.contains('icon-[uil--inbox]')).toBe(true);
    restoreButton.click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.restoreGroupToInbox).toHaveBeenCalledWith(
        'group-done',
        NotificationView.DONE
      );
    });
  });

  it('lets one realtime invalidation own the post-mutation list refresh', async () => {
    const { container } = render(NotificationsPage);
    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Mark done"]')).not.toBeNull();
      expect(mocks.store.notifications.fetchView).toHaveBeenCalledTimes(2);
    });

    (q(container, 'button[aria-label="Mark done"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.moveGroupToDone).toHaveBeenCalledWith(
        'group-1',
        NotificationView.INBOX
      );
    });
    expect(mocks.store.notifications.fetchView).toHaveBeenCalledTimes(2);
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
    mocks.store.notifications.fetchView.mockImplementation((view: NotificationView) =>
      Promise.resolve({
        groups:
          view === NotificationView.INBOX
            ? [
                {
                  id: 'followed-thread',
                  occurrences: [occurrence],
                  openTarget: occurrence,
                  unread: true,
                  occurrenceCount: 1,
                  latestAt: occurrence.createdAt,
                  reasons: occurrence.reasons
                }
              ]
            : [],
        unreadGroupCount: 1,
        roomUnreadGroupCounts: {},
        totalCount: view === NotificationView.INBOX ? 1 : 0,
        hasMore: false
      })
    );

    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"]');
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });
    const separators = row.querySelectorAll('[aria-hidden="true"].mx-1\\.5');
    expect(separators.length).toBeGreaterThan(0);
    expect(row.textContent).toContain('Alice');
    expect(row.textContent).toContain('Followed threads');
    expect(row.textContent).not.toMatch(/·\s*1\s*·/);
    expect(row.textContent).not.toContain('chat.example.test');
  });

  it('holds older rows behind a source with an unloaded newer page', async () => {
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
    const groupAt = (id: string, latestAt: string, state: NotificationInboxState) => {
      const occurrence = {
        ...mocks.occurrence,
        id: `${id}-occurrence`,
        createdAt: latestAt,
        inboxState: state
      };
      return {
        id,
        occurrences: [occurrence],
        openTarget: occurrence,
        unread: state === NotificationInboxState.UNREAD,
        occurrenceCount: 1,
        latestAt,
        reasons: [2]
      };
    };
    mocks.store.notifications.fetchView.mockImplementation((view: NotificationView, offset = 0) => {
      if (view === NotificationView.INBOX && offset === 0) {
        return Promise.resolve({
          groups: [groupAt('newest', '2026-08-11T12:00:00Z', NotificationInboxState.UNREAD)],
          unreadGroupCount: 2,
          roomUnreadGroupCounts: {},
          totalCount: 2,
          hasMore: true
        });
      }
      if (view === NotificationView.INBOX) {
        return Promise.resolve({
          groups: [groupAt('middle', '2026-08-11T11:00:00Z', NotificationInboxState.UNREAD)],
          unreadGroupCount: 2,
          roomUnreadGroupCounts: {},
          totalCount: 2,
          hasMore: false
        });
      }
      return Promise.resolve({
        groups: [groupAt('oldest', '2026-08-11T10:00:00Z', NotificationInboxState.DONE)],
        unreadGroupCount: 0,
        roomUnreadGroupCounts: {},
        totalCount: 1,
        hasMore: false
      });
    });

    const { container } = render(NotificationsPage);

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
    });
    expect(container.textContent).not.toContain('10:00');
    intersectionCallback?.(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver
    );
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(3);
    });
    const rows = [...container.querySelectorAll('[data-testid="notification-group"]')];
    expect(rows.map((row) => row.getAttribute('data-notification-state'))).toEqual([
      'inbox',
      'inbox',
      'done'
    ]);
  });

  it('advances each paginated view with its own result', async () => {
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
    const groupAt = (id: string, latestAt: string, state: NotificationInboxState) => {
      const occurrence = {
        ...mocks.occurrence,
        id: `${id}-occurrence`,
        createdAt: latestAt,
        inboxState: state
      };
      return {
        id,
        occurrences: [occurrence],
        openTarget: occurrence,
        unread: state === NotificationInboxState.UNREAD,
        occurrenceCount: 1,
        latestAt,
        reasons: [2]
      };
    };
    mocks.store.notifications.fetchView.mockImplementation((view: NotificationView, offset = 0) => {
      const state =
        view === NotificationView.INBOX
          ? NotificationInboxState.UNREAD
          : NotificationInboxState.DONE;
      const prefix = view === NotificationView.INBOX ? 'inbox' : 'done';
      return Promise.resolve({
        groups: [
          groupAt(
            `${prefix}-${offset}`,
            `2026-08-11T${view === NotificationView.INBOX ? '12' : '11'}:0${offset}:00Z`,
            state
          )
        ],
        unreadGroupCount: 1,
        roomUnreadGroupCounts: {},
        totalCount: 2,
        hasMore: offset === 0
      });
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
      expect(mocks.store.notifications.fetchView).toHaveBeenCalledWith(NotificationView.INBOX, 1);
      expect(mocks.store.notifications.fetchView).toHaveBeenCalledWith(NotificationView.DONE, 1);
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(4);
    });
  });

  it('keeps Inbox and Done subsets of the same conversation as separate rows', async () => {
    const pageFor = (state: NotificationInboxState) => {
      const occurrence = {
        ...mocks.occurrence,
        id: `same-group-${state}`,
        inboxState: state
      };
      return {
        groups: [
          {
            id: 'same-group',
            occurrences: [occurrence],
            openTarget: occurrence,
            unread: state === NotificationInboxState.UNREAD,
            occurrenceCount: 1,
            latestAt: occurrence.createdAt,
            reasons: [2]
          }
        ],
        unreadGroupCount: state === NotificationInboxState.UNREAD ? 1 : 0,
        roomUnreadGroupCounts: {},
        totalCount: 1,
        hasMore: false
      };
    };
    mocks.store.notifications.fetchView.mockImplementation((view: NotificationView) =>
      Promise.resolve(
        pageFor(
          view === NotificationView.INBOX
            ? NotificationInboxState.UNREAD
            : NotificationInboxState.DONE
        )
      )
    );

    const { container } = render(NotificationsPage);
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
    });
  });

  it('renders a retry state instead of an empty inbox when any source fails', async () => {
    mocks.store.notifications.fetchView.mockImplementation((view: NotificationView) => {
      if (view === NotificationView.DONE) return Promise.reject(new Error('offline'));
      return Promise.resolve({
        groups: [],
        unreadGroupCount: 0,
        roomUnreadGroupCounts: {},
        totalCount: 0,
        hasMore: false
      });
    });

    const { container } = render(NotificationsPage);

    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Try Again"]')).not.toBeNull();
    });
    expect(container.textContent).toContain('Network error. Please try again.');
    expect(container.textContent).not.toContain('You’re all caught up');
  });

  it('preserves fulfilled server results when another server fails', async () => {
    const remoteStore = {
      ...mocks.store,
      notifications: {
        ...mocks.store.notifications,
        fetchView: vi.fn().mockRejectedValue(new Error('remote offline'))
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
    expect(container.textContent).not.toContain('You’re all caught up');
  });

  it('fences row opening while triage is pending and reports mutation failures', async () => {
    let rejectMutation: ((reason?: unknown) => void) | undefined;
    mocks.store.notifications.moveGroupToDone.mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectMutation = reject;
        })
    );
    const { container } = render(NotificationsPage);
    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Mark done"]')).not.toBeNull();
    });
    const doneButton = q(container, 'button[aria-label="Mark done"]') as HTMLButtonElement;
    const rowButton = q(
      container,
      '[data-testid="notification-group"] > button'
    ) as HTMLButtonElement;
    doneButton.click();
    await vi.waitFor(() => expect(rowButton.disabled).toBe(true));
    rowButton.click();
    expect(mocks.goto).not.toHaveBeenCalled();

    rejectMutation?.(new Error('offline'));
    await vi.waitFor(() => {
      expect(getToasts().at(-1)?.message).toBe('Network error. Please try again.');
      expect(rowButton.disabled).toBe(false);
    });
  });

  it('formats old notifications with their source server viewer settings', async () => {
    const createdAt = '2025-04-27T00:30:00Z';
    mocks.store.currentUser.user.settings = {
      timezone: 'UTC',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
    };
    const localOccurrence = { ...mocks.occurrence, createdAt };
    mocks.store.notifications.fetchView.mockResolvedValue({
      groups: [
        {
          id: 'local',
          occurrences: [localOccurrence],
          openTarget: localOccurrence,
          unread: true,
          occurrenceCount: 1,
          latestAt: createdAt,
          reasons: [2]
        }
      ],
      unreadGroupCount: 1,
      roomUnreadGroupCounts: {},
      totalCount: 1,
      hasMore: false
    });

    const remoteStore = {
      ...mocks.store,
      currentUser: {
        user: {
          settings: {
            timezone: 'Pacific/Honolulu',
            timeFormat: TimeFormat.TIME_FORMAT_12_HOUR
          }
        }
      },
      serverInfo: { name: 'Remote Server' },
      notifications: {
        ...mocks.store.notifications,
        fetchView: vi.fn().mockResolvedValue({
          groups: [
            {
              id: 'remote',
              occurrences: [
                { ...mocks.occurrence, id: 'mention-remote', createdAt, summary: 'Remote mention' }
              ],
              openTarget: {
                ...mocks.occurrence,
                id: 'mention-remote',
                createdAt,
                summary: 'Remote mention'
              },
              unread: true,
              occurrenceCount: 1,
              latestAt: createdAt,
              reasons: [2]
            }
          ],
          unreadGroupCount: 1,
          roomUnreadGroupCounts: {},
          totalCount: 1,
          hasMore: false
        })
      }
    };
    mocks.servers.push({ id: 'remote', url: 'https://remote.example.test' });
    mocks.stores.set('remote', remoteStore);

    const { container } = render(NotificationsPage);

    await expect.element(container).toHaveTextContent('27 Apr 2025');
    await expect.element(container).toHaveTextContent('26 Apr 2025');
  });
});
