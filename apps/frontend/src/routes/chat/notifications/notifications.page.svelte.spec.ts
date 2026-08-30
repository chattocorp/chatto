import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  NotificationAttentionLevel,
  NotificationSignalKind,
  type NotificationOccurrenceItem
} from '$lib/api-client/notifications';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { getToasts, toast } from '$lib/ui/toast';
import { NotificationStore } from '$lib/state/server/notifications.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    pushNotifications: {
      enablePushOnAllServers: vi.fn(),
      getPermission: vi.fn(),
      getPushCapability: vi.fn(),
      getPushRegistrationTargets: vi.fn()
    },
    servers: [{ id: 'origin', url: 'https://chat.example.test' }],
    stores: new Map<string, unknown>(),
    appUi: { disableRoomCallWideFor: vi.fn() },
    occurrence: {
      id: 'mention-1',
      createdAt: new Date().toISOString(),
      actor: null,
      room: { id: 'room-1', name: 'general' },
      eventId: 'event-1',
      threadRootId: 'thread-1',
      signalKind: 'directMentionReceived' as const,
      targetSupported: true,
      attentionLevel: 2 as const,
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
        revokedRoomIds: new Set<string>(),
        scrubbedUserIds: new Set<string>(),
        occurrences: [] as NotificationOccurrenceItem[],
        consumedCount: 0,
        totalCount: 0,
        hasMore: false,
        hasLoaded: false,
        error: null as string | null,
        fetch: vi.fn(),
        fetchPage: vi.fn(),
        markOccurrenceRead: vi.fn().mockResolvedValue(undefined),
        deleteOccurrences: vi.fn().mockResolvedValue(undefined),
        deleteAllOccurrences: vi.fn().mockResolvedValue(undefined)
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

vi.mock('$lib/notifications/pushNotifications', () => ({
  enablePushOnAllServers: mocks.pushNotifications.enablePushOnAllServers,
  getPermission: mocks.pushNotifications.getPermission,
  getPushCapability: mocks.pushNotifications.getPushCapability,
  getPushRegistrationTargets: mocks.pushNotifications.getPushRegistrationTargets
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: number) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

import NotificationsPage from './+page.svelte';

function page(
  occurrences: NotificationOccurrenceItem[] = [mocks.occurrence as NotificationOccurrenceItem],
  hasMore = false
) {
  return {
    occurrences,
    unreadCount: occurrences.filter((candidate) => candidate.unread).length,
    importantUnreadCount: occurrences.filter(
      (candidate) =>
        candidate.unread && candidate.attentionLevel === NotificationAttentionLevel.IMPORTANT
    ).length,
    roomUnreadCounts: {},
    roomImportantUnreadCounts: {},
    totalCount: occurrences.length,
    hasMore,
    nextExpiryAt: null
  };
}

function retainProjection(
  occurrences: NotificationOccurrenceItem[] = [mocks.occurrence as NotificationOccurrenceItem],
  hasMore = false,
  consumedCount = occurrences.length
) {
  mocks.store.notifications.occurrences = occurrences;
  mocks.store.notifications.consumedCount = consumedCount;
  mocks.store.notifications.totalCount = occurrences.length;
  mocks.store.notifications.hasMore = hasMore;
  mocks.store.notifications.hasLoaded = true;
  mocks.store.notifications.error = null;
}

describe('notifications page', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    toast.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.store.notifications.revokedRoomIds.clear();
    mocks.store.notifications.scrubbedUserIds.clear();
    retainProjection();
    mocks.store.notifications.fetch.mockResolvedValue(undefined);
    mocks.store.notifications.fetchPage.mockResolvedValue(page());
    mocks.store.notifications.deleteOccurrences.mockResolvedValue(undefined);
    mocks.store.notifications.deleteAllOccurrences.mockResolvedValue(undefined);
    mocks.store.currentUser.user.settings = null;
    mocks.servers.splice(0, mocks.servers.length, {
      id: 'origin',
      url: 'https://chat.example.test'
    });
    mocks.stores.clear();
    mocks.stores.set('origin', mocks.store);
    mocks.pushNotifications.getPermission.mockReturnValue('granted');
    mocks.pushNotifications.getPushCapability.mockReturnValue('supported');
    mocks.pushNotifications.getPushRegistrationTargets.mockReturnValue([
      { serverId: 'origin', userId: 'user-1', vapidPublicKey: 'vapid-key' }
    ]);
    mocks.pushNotifications.enablePushOnAllServers.mockResolvedValue({
      permission: 'granted',
      registrations: [
        {
          serverId: 'origin',
          userId: 'user-1',
          vapidPublicKey: 'vapid-key',
          registered: true
        }
      ]
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('offers explicit push activation while browser permission is unset', async () => {
    mocks.pushNotifications.getPermission.mockReturnValue('default');

    const { container } = render(NotificationsPage);
    const enableButton = await vi.waitFor(() => {
      const button = Array.from(container.querySelectorAll('button')).find(
        (candidate) => candidate.textContent?.trim() === 'Enable push notifications'
      );
      expect(button).toBeDefined();
      return button as HTMLButtonElement;
    });

    enableButton.click();

    await vi.waitFor(() => {
      expect(mocks.pushNotifications.enablePushOnAllServers).toHaveBeenCalledOnce();
      expect(getToasts().at(-1)?.message).toBe('Push notifications enabled');
    });
    expect(container.textContent).not.toContain('Enable push notifications');
  });

  it('reports a partial push registration failure without offering permission again', async () => {
    mocks.pushNotifications.getPermission.mockReturnValue('default');
    mocks.pushNotifications.enablePushOnAllServers.mockResolvedValue({
      permission: 'granted',
      registrations: [
        {
          serverId: 'origin',
          userId: 'user-1',
          vapidPublicKey: 'vapid-key',
          registered: true
        },
        {
          serverId: 'remote',
          userId: 'user-2',
          vapidPublicKey: 'remote-vapid-key',
          registered: false
        }
      ]
    });

    const { container } = render(NotificationsPage);
    const enableButton = await vi.waitFor(() => {
      const button = Array.from(container.querySelectorAll('button')).find(
        (candidate) => candidate.textContent?.trim() === 'Enable push notifications'
      );
      expect(button).toBeDefined();
      return button as HTMLButtonElement;
    });
    enableButton.click();

    await vi.waitFor(() => {
      expect(getToasts().at(-1)?.message).toBe('Failed to enable push notifications');
    });
    expect(container.textContent).not.toContain('Enable push notifications');
  });

  it.each([
    ['granted', 'supported', 1],
    ['denied', 'supported', 1],
    ['default', 'unsupported', 1],
    ['default', 'supported', 0]
  ] as const)(
    'hides push activation for permission %s, capability %s, and %i targets',
    async (permission, capability, targetCount) => {
      mocks.pushNotifications.getPermission.mockReturnValue(permission);
      mocks.pushNotifications.getPushCapability.mockReturnValue(capability);
      mocks.pushNotifications.getPushRegistrationTargets.mockReturnValue(
        targetCount === 0
          ? []
          : [{ serverId: 'origin', userId: 'user-1', vapidPublicKey: 'vapid-key' }]
      );

      const { container } = render(NotificationsPage);

      await vi.waitFor(() => {
        expect(container.textContent).not.toContain('Enable push notifications');
      });
    }
  );

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

  it('uses the notification orange and reveals a quiet delete action from the row', async () => {
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
    expect(q(row, '[data-testid="notification-unread-dot"]')).toBeNull();
    expect(rowTarget.classList.contains('cursor-pointer')).toBe(true);
    expect(deleteButton.classList.contains('icon-action')).toBe(true);
    expect(deleteButton.parentElement?.classList.contains('hover-reveal-action')).toBe(true);
    expect(row.querySelectorAll('button')).toHaveLength(2);
    expect(row.textContent).not.toContain('Move to inbox');
    expect(row.textContent).not.toContain('Mark done');
  });

  it('renders an unsupported future target as non-navigating but dismissible activity', async () => {
    const unsupported = {
      ...mocks.occurrence,
      id: 'future-target-1',
      targetSupported: false,
      room: null,
      eventId: ''
    } as NotificationOccurrenceItem;
    retainProjection([unsupported]);

    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"]');
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });
    const rowTarget = q(row, ':scope > button') as HTMLButtonElement;
    const deleteButton = q(row, 'button[aria-label="Delete"]') as HTMLButtonElement;

    expect(rowTarget.disabled).toBe(true);
    expect(row.textContent).toContain('New activity');
    deleteButton.click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.deleteOccurrences).toHaveBeenCalledWith(
        ['future-target-1'],
        { unread: 1, importantUnread: 1, roomId: null }
      );
      expect(q(container, '[data-testid="notification-group"]')).toBeNull();
    });
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('renders read and unread notifications in one list', async () => {
    const readOccurrence = {
      ...mocks.occurrence,
      id: 'read-1',
      unread: false,
      createdAt: new Date(Date.now() - 60_000).toISOString()
    };
    retainProjection([mocks.occurrence as NotificationOccurrenceItem, readOccurrence]);

    const { container } = render(NotificationsPage);
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
    });

    expect(mocks.store.notifications.fetchPage).not.toHaveBeenCalled();
    const readRow = q(container, '[data-notification-state="read"]') as HTMLElement;
    const unreadRow = q(container, '[data-notification-state="unread"]') as HTMLElement;
    const readTarget = q(readRow, ':scope > button') as HTMLButtonElement;
    const unreadTarget = q(unreadRow, ':scope > button') as HTMLButtonElement;
    const readContent = q(readRow, '[data-testid="notification-content"]') as HTMLElement;
    const unreadContent = q(unreadRow, '[data-testid="notification-content"]') as HTMLElement;
    expect(readRow.classList.contains('bg-attention/5')).toBe(false);
    expect(q(readRow, '[data-testid="notification-unread-dot"]')).toBeNull();
    expect(q(unreadRow, '[data-testid="notification-unread-dot"]')).toBeNull();
    expect(readTarget.classList.contains('opacity-60')).toBe(true);
    expect(unreadTarget.classList.contains('opacity-60')).toBe(false);
    expect(readContent.querySelectorAll(':scope > *')).toHaveLength(2);
    expect(unreadContent.querySelectorAll(':scope > *')).toHaveLength(2);
  });

  it('renders a retained occurrence projection without refetching it', async () => {
    const rendered = render(NotificationsPage);
    expect(rendered.container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(
      1
    );
    expect(rendered.container.textContent).not.toContain('Loading');
    await new Promise((resolve) => setTimeout(resolve, 80));
    expect(mocks.store.notifications.fetch).not.toHaveBeenCalled();
    expect(mocks.store.notifications.fetchPage).not.toHaveBeenCalled();
    rendered.unmount();
  });

  it('renders a retained empty projection without flashing or refetching', async () => {
    retainProjection([]);

    const rendered = render(NotificationsPage);
    expect(rendered.container.textContent).toContain('No notifications');
    expect(rendered.container.textContent).not.toContain('Loading');

    await new Promise((resolve) => setTimeout(resolve, 80));
    expect(mocks.store.notifications.fetch).not.toHaveBeenCalled();
    expect(mocks.store.notifications.fetchPage).not.toHaveBeenCalled();
    expect(rendered.container.textContent).toContain('No notifications');
    rendered.unmount();
  });

  it('keeps initial hydration visually quiet', async () => {
    const originalStore = mocks.store.notifications;
    let resolveRefresh: ((value: ReturnType<typeof page>) => void) | undefined;
    const api = {
      listNotificationOccurrences: vi.fn(
        () =>
          new Promise<ReturnType<typeof page>>((resolve) => {
            resolveRefresh = resolve;
          })
      ),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    (mocks.store as { notifications: unknown }).notifications = notificationStore;

    const rendered = render(NotificationsPage);
    expect(rendered.container.textContent).not.toContain('Loading');
    expect(rendered.container.textContent).not.toContain('No notifications');

    await vi.waitFor(() => expect(resolveRefresh).toEqual(expect.any(Function)));
    resolveRefresh!(page());
    await vi.waitFor(() => {
      expect(
        rendered.container.querySelectorAll('[data-testid="notification-group"]')
      ).toHaveLength(1);
    });
    rendered.unmount();
    (mocks.store as { notifications: unknown }).notifications = originalStore;
  });

  it('renders a full-sentence summary and omits the single-occurrence counter', async () => {
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
      signalKind: NotificationSignalKind.FOLLOWED_THREAD
    };
    retainProjection([occurrence]);

    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"]');
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });

    expect(row.textContent).toContain('Alice replied in a thread you follow.');
    expect(row.textContent).not.toContain('Followed threads');
    expect(row.textContent).not.toMatch(/·\s*1\s*·/);
  });

  it('consolidates reactions to one target while showing their emoji and actors', async () => {
    const alice = {
      id: 'alice',
      login: 'alice',
      displayName: 'Alice',
      deleted: false,
      avatarUrl: null,
      presenceStatus: 1,
      customStatus: null
    };
    const bob = { ...alice, id: 'bob', login: 'bob', displayName: 'Bob' };
    const reaction = (id: string, actor: typeof alice, emoji: string) => ({
      ...mocks.occurrence,
      id,
      actor,
      eventId: 'reacted-to-message',
      threadRootId: null,
      signalKind: NotificationSignalKind.REACTION,
      attentionLevel: NotificationAttentionLevel.AMBIENT,
      reactionEmoji: emoji
    });
    retainProjection([
      reaction('reaction-alice', alice, 'thumbsup'),
      reaction('reaction-bob', bob, 'heart')
    ]);

    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const rows = container.querySelectorAll('[data-testid="notification-group"]');
      expect(rows).toHaveLength(1);
      return rows[0] as HTMLElement;
    });

    expect(row.textContent).toContain(
      "Alice and others reacted with '👍 ❤️' to your message in #general."
    );
    expect(row.textContent?.match(/#general/g)).toHaveLength(1);
    expect(row.dataset.notificationAttention).toBe('ambient');
    expect(row.classList.contains('bg-attention/5')).toBe(false);
    expect(q(row, '[data-testid="notification-unread-dot"]')).toBeNull();
    expect(q(row, '[data-testid="notification-actor-stack"]')?.children).toHaveLength(2);
  });

  it('renders a single reaction as a sentence', async () => {
    const bob = {
      id: 'bob',
      login: 'bob',
      displayName: 'Bob',
      deleted: false,
      avatarUrl: null,
      presenceStatus: 1,
      customStatus: null
    };
    retainProjection([
      {
        ...mocks.occurrence,
        id: 'reaction-bob',
        actor: bob,
        signalKind: NotificationSignalKind.REACTION,
        attentionLevel: NotificationAttentionLevel.AMBIENT,
        reactionEmoji: 'heart'
      }
    ]);

    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"]');
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });

    expect(row.textContent).toContain("Bob reacted with '❤️' to your message in #general.");
    expect(row.textContent?.match(/#general/g)).toHaveLength(1);
  });

  it('keeps a known reaction actor as the lead when another actor was deleted', async () => {
    const alice = {
      id: 'alice',
      login: 'alice',
      displayName: 'Alice',
      deleted: false,
      avatarUrl: null,
      presenceStatus: 1,
      customStatus: null
    };
    retainProjection([
      {
        ...mocks.occurrence,
        id: 'reaction-alice',
        createdAt: '2026-08-19T12:00:00.000Z',
        actor: alice,
        eventId: 'reacted-to-message',
        signalKind: NotificationSignalKind.REACTION,
        attentionLevel: NotificationAttentionLevel.AMBIENT,
        reactionEmoji: 'thumbsup'
      },
      {
        ...mocks.occurrence,
        id: 'reaction-deleted',
        createdAt: '2026-08-19T12:01:00.000Z',
        actor: null,
        eventId: 'reacted-to-message',
        signalKind: NotificationSignalKind.REACTION,
        attentionLevel: NotificationAttentionLevel.AMBIENT,
        reactionEmoji: 'heart'
      }
    ] as NotificationOccurrenceItem[]);

    const { container } = render(NotificationsPage);
    const row = await vi.waitFor(() => {
      const element = q(container, '[data-testid="notification-group"]');
      expect(element).not.toBeNull();
      return element as HTMLElement;
    });

    expect(row.textContent).toContain(
      "Alice and others reacted with '❤️ 👍' to your message in #general."
    );
    expect(row.textContent).not.toContain('[deleted user] and others');
  });

  it('preserves a healthy server result when another server fails', async () => {
    const remoteStore = {
      ...mocks.store,
      notifications: {
        ...mocks.store.notifications,
        occurrences: [],
        consumedCount: 0,
        totalCount: 0,
        hasMore: false,
        hasLoaded: false,
        error: 'remote offline',
        fetch: vi.fn().mockResolvedValue(undefined)
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

  it('does not render a stale notification page for a room behind a privacy boundary', async () => {
    mocks.store.notifications.revokedRoomIds.add('room-1');

    const { container } = render(NotificationsPage);

    await vi.waitFor(() => {
      expect(mocks.store.notifications.fetchPage).not.toHaveBeenCalled();
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(0);
    });
  });

  it('clears mounted notification rows immediately at a projection reset boundary', async () => {
    const originalStore = mocks.store.notifications;
    const api = {
      listNotificationOccurrences: vi.fn().mockResolvedValue(page()),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    (mocks.store as { notifications: unknown }).notifications = notificationStore;
    const rendered = render(NotificationsPage);
    try {
      await vi.waitFor(() => {
        expect(
          rendered.container.querySelectorAll('[data-testid="notification-group"]')
        ).toHaveLength(1);
      });

      notificationStore.resetProjectionState();

      await vi.waitFor(() => {
        expect(
          rendered.container.querySelectorAll('[data-testid="notification-group"]')
        ).toHaveLength(0);
      });
    } finally {
      rendered.unmount();
      (mocks.store as { notifications: unknown }).notifications = originalStore;
    }
  });

  it('scrubs a deleted actor from mounted notification rows immediately', async () => {
    const originalStore = mocks.store.notifications;
    const actorOccurrence = {
      ...mocks.occurrence,
      signalKind: NotificationSignalKind.REACTION,
      attentionLevel: NotificationAttentionLevel.AMBIENT,
      reactionEmoji: 'heart',
      actor: {
        id: 'alice',
        login: 'alice',
        displayName: 'Alice Example',
        avatarUrl: null,
        presenceStatus: 'OFFLINE',
        deleted: false
      }
    } as unknown as NotificationOccurrenceItem;
    const api = {
      listNotificationOccurrences: vi.fn().mockResolvedValue(page([actorOccurrence])),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    (mocks.store as { notifications: unknown }).notifications = notificationStore;
    const rendered = render(NotificationsPage);
    try {
      await vi.waitFor(() => {
        expect(rendered.container.textContent).toContain('Alice Example');
      });

      notificationStore.scrubUser('alice');

      await vi.waitFor(() => {
        expect(rendered.container.textContent).not.toContain('Alice Example');
        expect(rendered.container.textContent).toContain(
          "[deleted user] reacted with '❤️' to your message in #general."
        );
      });
    } finally {
      rendered.unmount();
      (mocks.store as { notifications: unknown }).notifications = originalStore;
    }
  });

  it('applies an authoritative realtime replacement without a list request', async () => {
    const originalStore = mocks.store.notifications;
    const api = {
      listNotificationOccurrences: vi.fn(),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    notificationStore.replaceOccurrenceProjection(page());
    (mocks.store as { notifications: unknown }).notifications = notificationStore;
    const rendered = render(NotificationsPage);
    try {
      await vi.waitFor(() => {
        expect(
          rendered.container.querySelectorAll('[data-testid="notification-group"]')
        ).toHaveLength(1);
      });

      notificationStore.replaceOccurrenceProjection(page([]));
      await vi.waitFor(() => {
        expect(
          rendered.container.querySelectorAll('[data-testid="notification-group"]')
        ).toHaveLength(0);
      });
      expect(api.listNotificationOccurrences).not.toHaveBeenCalled();
    } finally {
      rendered.unmount();
      (mocks.store as { notifications: unknown }).notifications = originalStore;
    }
  });

  it('updates mounted rows after quiet reconciliation', async () => {
    const originalStore = mocks.store.notifications;
    const api = {
      listNotificationOccurrences: vi.fn().mockResolvedValue(page([])),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    notificationStore.replaceOccurrenceProjection(page());
    (mocks.store as { notifications: unknown }).notifications = notificationStore;
    const rendered = render(NotificationsPage);
    try {
      await vi.waitFor(() => {
        expect(
          rendered.container.querySelectorAll('[data-testid="notification-group"]')
        ).toHaveLength(1);
      });

      await notificationStore.reconcile();
      await vi.waitFor(() => {
        expect(
          rendered.container.querySelectorAll('[data-testid="notification-group"]')
        ).toHaveLength(0);
      });
      expect(api.listNotificationOccurrences).toHaveBeenCalledTimes(1);
    } finally {
      rendered.unmount();
      (mocks.store as { notifications: unknown }).notifications = originalStore;
    }
  });

  it('keeps a dismissed row absent when the request outcome is ambiguous', async () => {
    let rejectMutation: ((reason?: unknown) => void) | undefined;
    mocks.store.notifications.deleteOccurrences.mockImplementation(
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
    deleteButton.click();
    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="notification-group"]')).toBeNull();
    });

    rejectMutation?.(new Error('offline'));
    await vi.waitFor(() => {
      expect(getToasts().at(-1)?.message).toBe('Network error. Please try again.');
      expect(container.querySelector('[data-testid="notification-group"]')).toBeNull();
    });
  });

  it('does not start Dismiss read while an exact dismissal is in flight', async () => {
    let resolveMutation: (() => void) | undefined;
    mocks.store.notifications.deleteOccurrences.mockImplementation(
      () => new Promise<void>((resolve) => (resolveMutation = resolve))
    );
    retainProjection([
      mocks.occurrence as NotificationOccurrenceItem,
      {
        ...mocks.occurrence,
        id: 'mention-2',
        eventId: 'event-2',
        createdAt: new Date(Date.now() - 1_000).toISOString(),
        unread: false
      }
    ]);

    const { container } = render(NotificationsPage);
    const deleteButton = await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
      return q(
        container,
        '[data-notification-state="unread"] button[aria-label="Delete"]'
      ) as HTMLButtonElement;
    });
    deleteButton.click();

    const dismissRead = await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
      const button = q(container, 'button[aria-label="Dismiss read"]') as HTMLButtonElement;
      expect(button.disabled).toBe(true);
      return button;
    });
    dismissRead.click();
    expect(mocks.store.notifications.deleteOccurrences).toHaveBeenCalledTimes(1);

    resolveMutation?.();
    await vi.waitFor(() => expect(dismissRead.disabled).toBe(false));
  });

  it('groups rows by date in the viewer timezone', async () => {
    const now = new Date();
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1_000);
    const older = new Date(now);
    older.setUTCMonth(older.getUTCMonth() - 2);
    const occurrences = [
      { ...mocks.occurrence, id: 'today', createdAt: now.toISOString() },
      { ...mocks.occurrence, id: 'yesterday', createdAt: yesterday.toISOString() },
      { ...mocks.occurrence, id: 'older', createdAt: older.toISOString() }
    ];
    mocks.store.currentUser.user.settings = {
      timezone: 'Pacific/Auckland',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
    };
    retainProjection(occurrences);

    const { container } = render(NotificationsPage);
    const headings = await vi.waitFor(() => {
      const elements = container.querySelectorAll('[data-testid="notification-date-heading"]');
      expect(elements.length).toBeGreaterThanOrEqual(3);
      return [...elements].map((heading) => heading.textContent?.trim());
    });

    expect(headings).toContain('Today');
    expect(headings).toContain('Yesterday');
    expect(headings.some((heading) => heading?.match(/\w+ \d{4}/))).toBe(true);
    const firstHeading = container.querySelector(
      '[data-testid="notification-date-heading"]'
    ) as HTMLElement;
    expect(firstHeading.classList.contains('sticky')).toBe(false);
    expect(firstHeading.classList.contains('w-full')).toBe(true);
    expect(firstHeading.classList.contains('px-4')).toBe(false);
    expect(firstHeading.querySelectorAll('.h-px.bg-border')).toHaveLength(2);
  });

  it('dismisses only the read snapshot with one exact request per server', async () => {
    let resolveOrigin: (() => void) | undefined;
    let resolveRemote: (() => void) | undefined;
    mocks.store.notifications.deleteOccurrences.mockImplementation(
      () => new Promise<void>((resolve) => (resolveOrigin = resolve))
    );
    retainProjection([
      { ...mocks.occurrence, id: 'origin-read', unread: false },
      { ...mocks.occurrence, id: 'shared-id', eventId: 'event-new', unread: true }
    ]);
    const remoteStore = {
      ...mocks.store,
      notifications: {
        ...mocks.store.notifications,
        occurrences: [{ ...mocks.occurrence, id: 'shared-id', unread: false }],
        consumedCount: 1,
        totalCount: 1,
        hasMore: false,
        hasLoaded: true,
        error: null,
        deleteOccurrences: vi.fn(() => new Promise<void>((resolve) => (resolveRemote = resolve)))
      }
    };
    mocks.servers.push({ id: 'remote', url: 'https://remote.example.test' });
    mocks.stores.set('remote', remoteStore);

    const { container } = render(NotificationsPage);
    const dismissRead = await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(3);
      const button = q(container, 'button[aria-label="Dismiss read"]');
      expect(button).not.toBeNull();
      return button as HTMLButtonElement;
    });

    dismissRead.click();
    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
    });
    expect(container.textContent).toContain('chat.example.test');
    expect(mocks.store.notifications.deleteOccurrences).toHaveBeenCalledWith(['origin-read'], {
      unread: 0,
      importantUnread: 0
    });
    expect(remoteStore.notifications.deleteOccurrences).toHaveBeenCalledWith(['shared-id'], {
      unread: 0,
      importantUnread: 0
    });

    resolveOrigin?.();
    resolveRemote?.();
  });

  it('keeps the exact read snapshot absent after an ambiguous Dismiss read', async () => {
    retainProjection([{ ...mocks.occurrence, unread: false }]);
    const remoteStore = {
      ...mocks.store,
      notifications: {
        ...mocks.store.notifications,
        occurrences: [{ ...mocks.occurrence, id: 'remote-notification', unread: false }],
        consumedCount: 1,
        totalCount: 1,
        hasMore: false,
        hasLoaded: true,
        error: null,
        deleteOccurrences: vi.fn().mockRejectedValue(new Error('remote offline'))
      }
    };
    mocks.servers.push({ id: 'remote', url: 'https://remote.example.test' });
    mocks.stores.set('remote', remoteStore);

    const { container } = render(NotificationsPage);
    const dismissRead = await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
      return q(container, 'button[aria-label="Dismiss read"]') as HTMLButtonElement;
    });
    dismissRead.click();

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(0);
      expect(getToasts().at(-1)?.message).toBe('Network error. Please try again.');
    });
    expect(container.textContent).not.toContain('remote.example.test');
    expect(container.textContent).not.toContain('chat.example.test');
    expect(mocks.store.notifications.deleteOccurrences).toHaveBeenCalledTimes(1);
    expect(remoteStore.notifications.deleteOccurrences).toHaveBeenCalledTimes(1);
  });

  it('does not restore the origin after an ambiguous optimistic Dismiss read', async () => {
    retainProjection([{ ...mocks.occurrence, unread: false }]);
    mocks.store.notifications.deleteOccurrences.mockRejectedValue(new Error('origin offline'));
    const remoteStore = {
      ...mocks.store,
      notifications: {
        ...mocks.store.notifications,
        occurrences: [{ ...mocks.occurrence, id: 'remote-notification', unread: false }],
        consumedCount: 1,
        totalCount: 1,
        hasMore: false,
        hasLoaded: true,
        error: null,
        deleteOccurrences: vi.fn().mockResolvedValue(undefined)
      }
    };
    mocks.servers.push({ id: 'remote', url: 'https://remote.example.test' });
    mocks.stores.set('remote', remoteStore);

    const { container } = render(NotificationsPage);
    const dismissRead = await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
      return q(container, 'button[aria-label="Dismiss read"]') as HTMLButtonElement;
    });
    dismissRead.click();

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(0);
      expect(getToasts().at(-1)?.message).toBe('Network error. Please try again.');
    });
    expect(container.textContent).not.toContain('chat.example.test');
    expect(container.textContent).not.toContain('remote.example.test');
  });

  it('loads the next page from each server independently', async () => {
    const originalStore = mocks.store.notifications;
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
    const api = {
      listNotificationOccurrences: vi.fn((_limit: number, offset = 0) => {
        const occurrence = {
          ...mocks.occurrence,
          id: `notification-${offset}`,
          createdAt: `2026-08-11T12:0${offset}:00Z`
        };
        return Promise.resolve(page([occurrence], false));
      }),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    const firstOccurrence = {
      ...mocks.occurrence,
      id: 'notification-0',
      createdAt: '2026-08-11T12:00:00Z'
    } as NotificationOccurrenceItem;
    notificationStore.replaceOccurrenceProjection(page([firstOccurrence], true));
    const fetchPage = vi.spyOn(notificationStore, 'fetchPage');
    (mocks.store as { notifications: unknown }).notifications = notificationStore;

    const { container, unmount } = render(NotificationsPage);
    try {
      await vi.waitFor(() => {
        expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
      });
      intersectionCallback?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver
      );
      await vi.waitFor(() => {
        expect(fetchPage).toHaveBeenCalledWith(1);
        expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(2);
      });
    } finally {
      unmount();
      (mocks.store as { notifications: unknown }).notifications = originalStore;
    }
  });

  it('continues past an empty privacy-filtered page using its raw consumed offset', async () => {
    const originalStore = mocks.store.notifications;
    const api = {
      listNotificationOccurrences: vi.fn((_limit: number, offset = 0) => {
        return Promise.resolve(
          offset === 50
            ? { ...page([mocks.occurrence as NotificationOccurrenceItem]), consumedCount: 1 }
            : page([])
        );
      }),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn(),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    notificationStore.replaceOccurrenceProjection({ ...page([], true), consumedCount: 50 });
    const fetchPage = vi.spyOn(notificationStore, 'fetchPage');
    (mocks.store as { notifications: unknown }).notifications = notificationStore;

    const { container, unmount } = render(NotificationsPage);
    try {
      await vi.waitFor(() => {
        expect(fetchPage).toHaveBeenCalledWith(50);
        expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
      });
    } finally {
      unmount();
      (mocks.store as { notifications: unknown }).notifications = originalStore;
    }
  });

  it('consolidates one direct-message group across loaded pages and dismisses every member', async () => {
    const originalStore = mocks.store.notifications;
    let intersectionCallback: IntersectionObserverCallback | undefined;
    let resolveSecondPage: (() => void) | undefined;
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
    const occurrenceAt = (offset: number) =>
      ({
        ...mocks.occurrence,
        id: `dm-${offset}`,
        eventId: `dm-event-${offset}`,
        threadRootId: null,
        signalKind: NotificationSignalKind.DIRECT_MESSAGE,
        createdAt: new Date(Date.UTC(2026, 7, 11, 12, 0, offset)).toISOString()
      }) as NotificationOccurrenceItem;
    const api = {
      listNotificationOccurrences: vi.fn(async (_limit: number, offset = 0) => {
        if (offset === 1) {
          await new Promise<void>((resolve) => {
            resolveSecondPage = resolve;
          });
        }
        return page([occurrenceAt(offset)], false);
      }),
      markNotificationRead: vi.fn(),
      batchDeleteNotificationOccurrences: vi.fn().mockResolvedValue(0),
      deleteAllNotificationOccurrences: vi.fn(),
      getNotificationPolicy: vi.fn(),
      updateNotificationPolicy: vi.fn()
    };
    const notificationStore = new NotificationStore(api as never);
    notificationStore.replaceOccurrenceProjection(page([occurrenceAt(0)], true));
    const fetchPage = vi.spyOn(notificationStore, 'fetchPage');
    const deleteOccurrences = vi.spyOn(notificationStore, 'deleteOccurrences');
    (mocks.store as { notifications: unknown }).notifications = notificationStore;

    const { container, unmount } = render(NotificationsPage);
    try {
      await vi.waitFor(() => {
        expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
      });
      intersectionCallback?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver
      );
      await vi.waitFor(() => {
        expect(fetchPage).toHaveBeenCalledWith(1);
        expect(resolveSecondPage).toEqual(expect.any(Function));
        expect(q(container, '.selectable-list')?.getAttribute('aria-busy')).toBe('true');
        expect(container.textContent).not.toContain('Loading');
      });
      resolveSecondPage!();
      await vi.waitFor(() => {
        expect(container.textContent).not.toContain('Loading');
        expect(q(container, '.selectable-list')?.getAttribute('aria-busy')).toBe('false');
        expect(container.querySelectorAll('[data-testid="notification-group"]')).toHaveLength(1);
      });

      (q(container, 'button[aria-label="Delete"]') as HTMLButtonElement).click();
      await vi.waitFor(() => {
        expect(deleteOccurrences).toHaveBeenCalledWith(expect.arrayContaining(['dm-0', 'dm-1']), {
          unread: 2,
          importantUnread: 2,
          roomId: 'room-1'
        });
      });
      expect(deleteOccurrences.mock.calls[0]?.[0]).toHaveLength(2);
    } finally {
      unmount();
      (mocks.store as { notifications: unknown }).notifications = originalStore;
    }
  });
});
