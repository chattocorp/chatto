import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    servers: [{ id: 'origin', url: 'https://chat.example.test' }],
    stores: new Map<string, unknown>(),
    appUi: {
      disableRoomCallWideFor: vi.fn()
    },
    notification: {
      id: 'mention-1',
      kind: 'mention',
      createdAt: new Date().toISOString(),
      actor: null,
      summary: 'Mentioned you in a message',
      mentionRoom: { id: 'room-1', name: 'general' },
      mentionEventId: 'event-1',
      mentionInThread: 'thread-1'
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
        notifications: [] as unknown[],
        unreadNotificationCount: 1,
        fetch: vi.fn().mockResolvedValue(undefined),
        dismiss: vi.fn().mockResolvedValue(true),
        dismissAll: vi.fn().mockResolvedValue(0),
        getCleanPath: vi.fn().mockReturnValue('/chat/-/room-1/thread-1'),
        getLocationString: vi.fn().mockReturnValue('#general in Test Server')
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
    getStore: vi.fn((serverId: string) => mocks.stores.get(serverId))
  }
}));

vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => mocks.appUi
}));

import NotificationsPage from './+page.svelte';

describe('notifications page', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.store.notifications.notifications = [mocks.notification];
    mocks.store.notifications.fetch.mockResolvedValue(undefined);
    mocks.store.notifications.dismiss.mockResolvedValue(true);
    mocks.store.notifications.getCleanPath.mockReturnValue('/chat/-/room-1/thread-1');
    mocks.store.notifications.getLocationString.mockReturnValue('#general in Test Server');
    mocks.servers.splice(0, mocks.servers.length, {
      id: 'origin',
      url: 'https://chat.example.test'
    });
    mocks.stores.clear();
    mocks.stores.set('origin', mocks.store);
  });

  it('reveals the target room before navigating from a notification row', async () => {
    const { container } = render(NotificationsPage);

    const item = q(container, '[data-testid="notification-item"]') as HTMLElement;
    await expect.element(item).toBeInTheDocument();
    item.click();

    await vi.waitFor(() => {
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'room-1');
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
      );
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'room-1',
        'thread-1',
        'event-1'
      );
      expect(mocks.store.notifications.dismiss).toHaveBeenCalledWith('mention-1');
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-1/thread-1');
    });
  });

  it('formats old notifications with their source server viewer settings', async () => {
    const createdAt = '2025-04-27T00:30:00Z';
    mocks.store.currentUser.user.settings = {
      timezone: 'UTC',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
    };
    mocks.store.notifications.notifications = [{ ...mocks.notification, createdAt }];

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
        notifications: [
          {
            ...mocks.notification,
            id: 'mention-remote',
            createdAt,
            summary: 'Remote mention'
          }
        ]
      }
    };
    mocks.servers.push({ id: 'remote', url: 'https://remote.example.test' });
    mocks.stores.set('remote', remoteStore);

    const { container } = render(NotificationsPage);

    await expect.element(container).toHaveTextContent('27 Apr 2025');
    await expect.element(container).toHaveTextContent('26 Apr 2025');
  });
});
