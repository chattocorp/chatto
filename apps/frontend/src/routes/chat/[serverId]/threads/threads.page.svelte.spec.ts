import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';

const mocks = vi.hoisted(() => ({
  listFollowedThreads: vi.fn(),
  refreshUnreadFollowedThreads: vi.fn(),
  pageState: {
    threadFilter: 'unread'
  }
}));

vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    Object.entries(params).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));

vi.mock('$app/state', () => ({
  page: {
    state: mocks.pageState
  }
}));

vi.mock('$lib/api-client/threads', () => ({
  createThreadAPI: () => ({
    listFollowedThreads: mocks.listFollowedThreads
  })
}));

vi.mock('$lib/eventBus.svelte', () => ({
  onThreadFollowChanged: vi.fn(() => () => {})
}));

vi.mock('$lib/hooks', () => ({
  useEvent: vi.fn()
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => serverId,
  segmentToServerId: (segment: string) => segment
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/room', () => ({
  createRoomPermissions: vi.fn(),
  DEFAULT_ROOM_PERMISSIONS: {},
  createRoomMembers: vi.fn(),
  createComposerContext: vi.fn(),
  createMentionRoles: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    serverId: 'origin',
    connectBaseUrl: '/api/connect',
    bearerToken: null
  })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      rooms: {
        refreshUnreadFollowedThreads: mocks.refreshUnreadFollowedThreads
      }
    })
  }
}));

vi.mock('$lib/state/userSettings.svelte', () => ({
  getUserSettings: () => ({})
}));

vi.mock('../[roomId]/RoomEvent.svelte', async () => ({
  default: (await import('./ThreadsRoomEventMock.svelte')).default
}));

import ThreadsPage from './+page.svelte';

describe('My Threads page', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    mocks.pageState.threadFilter = 'unread';
    mocks.listFollowedThreads.mockResolvedValue({
      threads: [],
      hasMore: false,
      totalCount: 0,
      unreadOnly: true
    });
    mocks.refreshUnreadFollowedThreads.mockResolvedValue(undefined);
    await loadLocaleMessages('en');
    setReactiveLocale('en');
  });

  it('reconciles the sidebar indicator after loading the authoritative unread list', async () => {
    render(ThreadsPage);

    await vi.waitFor(() => {
      expect(mocks.listFollowedThreads).toHaveBeenCalledWith({
        limit: 20,
        offset: 0,
        unreadOnly: true
      });
      expect(mocks.refreshUnreadFollowedThreads).toHaveBeenCalledOnce();
    });
  });

  it('does not add a viewer refresh for the unfiltered thread list', async () => {
    mocks.pageState.threadFilter = 'all';
    mocks.listFollowedThreads.mockResolvedValue({
      threads: [],
      hasMore: false,
      totalCount: 0,
      unreadOnly: false
    });

    render(ThreadsPage);

    await vi.waitFor(() => {
      expect(mocks.listFollowedThreads).toHaveBeenCalledWith({
        limit: 20,
        offset: 0,
        unreadOnly: false
      });
    });
    expect(mocks.refreshUnreadFollowedThreads).not.toHaveBeenCalled();
  });
});
