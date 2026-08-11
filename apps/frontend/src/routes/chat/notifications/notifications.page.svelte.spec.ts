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

import NotificationsPage from './+page.svelte';

describe('notifications page', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
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
    mocks.store.notifications.fetchView.mockResolvedValue({
      groups: [group],
      unreadGroupCount: 1,
      totalCount: 1,
      hasMore: false
    });
    mocks.servers.splice(0, mocks.servers.length, {
      id: 'origin',
      url: 'https://chat.example.test'
    });
    mocks.stores.clear();
    mocks.stores.set('origin', mocks.store);
  });

  it('reveals the target room before navigating from a notification row', async () => {
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
        'event-1'
      );
      expect(mocks.store.notifications.markOccurrenceRead).toHaveBeenCalledWith('mention-1');
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-1/thread-1');
      expect(mocks.goto.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.store.notifications.markOccurrenceRead.mock.invocationCallOrder[0]
      );
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
