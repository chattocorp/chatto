import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { MessageSearchOrder } from '$lib/api-client/messageSearch';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { q } from '$lib/test-utils';
import { queryClient } from '$lib/query/client';
import { clearUserStores, getUserStore, resetUserStoresForTests, type UserStore } from '$lib/state/server/users.svelte';

import { quickSwitcher } from '$lib/state/globals.svelte';

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  query: vi.fn(),
  mutation: vi.fn(),
  startDM: vi.fn(),
  listRooms: vi.fn(),
  listRoomMembers: vi.fn(),
  listUsers: vi.fn(),
  searchMessages: vi.fn(),
  privacyListeners: [] as Array<
    (matches: (result: { id: string }) => boolean, force: boolean) => void
  >,
  toastError: vi.fn(),
  recents: {
    urls: [] as string[],
    record: vi.fn((url: string) => {
      mocks.recents.urls = [url, ...mocks.recents.urls.filter((entry) => entry !== url)];
    })
  },
  servers: [
    {
      id: 'origin',
      url: 'https://chat.example.test',
      name: 'Fallback Server'
    }
  ],
  store: {
    isAuthenticated: true,
    realtimeSync: { hasUsableProjection: true },
    projection: {
      users: undefined as unknown as UserStore,
      rooms: new Map<string, RoomWithViewerState>()
    },
    serverInfo: {
      name: 'Workspace Server',
      iconUrl: null,
      supportsFeature: vi.fn(() => true)
    },
    permissions: {
      canStartDMs: true
    },
    currentUser: {
      user: {
        id: 'user-current'
      }
    },
    navigation: {
      rooms: [] as Array<{
        id: string;
        name: string;
        type: RoomKind;
        viewerIsMember: boolean;
        hasMessageHistory?: boolean | null;
        members: User[];
      }>,
      isInitialLoading: false
    },
    messageSearch: {
      available: true,
      ensureStatus: vi.fn(),
      privacyRevision: 0,
      subscribePrivacyInvalidation: vi.fn(
        (listener: (matches: (result: { id: string }) => boolean, force: boolean) => void) => {
          mocks.privacyListeners.push(listener);
          return vi.fn();
        }
      )
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    Object.entries(params ?? {}).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => (serverId === 'origin' ? '-' : serverId),
  segmentToServerId: (segment: string) => (segment === '-' ? 'origin' : null)
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    tryGetStore: vi.fn((id: string) => stores.get(id) ?? mocks.store)
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: () => ({
      connectBaseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token-1',
      queryScope: 'test-session',
      getAPI: (factory: (config: never) => unknown) => factory({} as never),
      client: {
        query: mocks.query,
        mutation: mocks.mutation
      }
    })
  }
}));

vi.mock('$lib/state/recentQuickSwitcher.svelte', () => ({
  recentQuickSwitcher: mocks.recents
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: string) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
    getLiveBio: () => null,
    getLiveTimezone: () => null,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/ui/toast', () => ({
  toast: {
    error: mocks.toastError
  }
}));

vi.mock('$lib/api-client/rooms', () => ({
  createRoomCommandAPI: vi.fn(() => ({
    startDM: mocks.startDM
  }))
}));

vi.mock('$lib/api-client/memberDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/memberDirectory')>();
  return {
    ...actual,
    createMemberDirectoryAPI: vi.fn(() => ({
      listRoomMembers: mocks.listRoomMembers,
      listUsers: mocks.listUsers
    }))
  };
});

vi.mock('$lib/api-client/messageSearch', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/messageSearch')>();
  return {
    ...actual,
    createMessageSearchAPI: vi.fn(() => ({
      searchMessages: mocks.searchMessages
    }))
  };
});

vi.mock('$lib/api-client/roomDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/roomDirectory')>();
  return {
    ...actual,
    createRoomDirectoryAPI: vi.fn(() => ({
      listRooms: mocks.listRooms
    }))
  };
});

import QuickSwitcher from './QuickSwitcher.svelte';

const stores = new SvelteMap<string, typeof mocks.store>();

function publishNavigation(serverId = 'origin', navigation = mocks.store.navigation) {
  stores.set(serverId, { ...mocks.store, navigation: { ...navigation } });
  flushSync();
}

type User = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl: string | null;
  presenceStatus: string;
  isBot?: boolean;
};

function user(
  id: string,
  login: string,
  displayName: string,
  isBot = false
): User {
  return {
    id,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: 'ONLINE',
    isBot
  };
}

const currentUser = user('user-current', 'alice', 'Alice Current');
const teammate = user('user-teammate', 'river', 'River Teammate');
let currentRender: { unmount: () => void } | undefined;
let originalShowModal: typeof HTMLDialogElement.prototype.showModal;
let originalClose: typeof HTMLDialogElement.prototype.close;

function installQueryMocks() {
  mocks.startDM.mockResolvedValue({ id: 'dm-new' });
  mocks.store.navigation.rooms = [
    {
      id: 'room-general',
      name: 'general',
      type: RoomKind.CHANNEL,
      viewerIsMember: true,
      members: []
    },
    {
      id: 'room-xylophone',
      name: 'xylophone-chat',
      type: RoomKind.CHANNEL,
      viewerIsMember: true,
      members: []
    },
    {
      id: 'dm-existing',
      name: '',
      type: RoomKind.DM,
      viewerIsMember: true,
      members: [currentUser, teammate]
    },
    {
      id: 'dm-empty',
      name: '',
      type: RoomKind.DM,
      viewerIsMember: true,
      hasMessageHistory: false,
      members: [currentUser, user('user-empty', 'empty', 'Empty Conversation')]
    }
  ];
}

async function renderOpenSwitcher() {
  const rendered = render(QuickSwitcher);
  currentRender = rendered;

  quickSwitcher.open();
  flushSync();

  await vi.waitFor(() => {
    expect(dialog(rendered.container).hasAttribute('open')).toBe(true);
  });
  await vi.waitFor(() => {
    expect(rendered.container.textContent).toContain('xylophone-chat');
  });

  return rendered;
}

function input(container: HTMLElement): HTMLInputElement {
  return q(
    container,
    'input[placeholder="Go somewhere, or type ? to search messages..."]'
  ) as HTMLInputElement;
}

function dialog(container: HTMLElement): HTMLDialogElement {
  const el = q(container, 'dialog.quick-switcher') as HTMLDialogElement | null;
  if (!el) throw new Error('QuickSwitcher dialog not found');
  return el;
}

function setSearch(container: HTMLElement, value: string) {
  const search = input(container);
  search.value = value;
  search.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

function resultButtons(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('button[data-index]'));
}

beforeAll(() => {
  originalShowModal = HTMLDialogElement.prototype.showModal;
  originalClose = HTMLDialogElement.prototype.close;
  HTMLDialogElement.prototype.showModal = function showModal() {
    this.setAttribute('open', '');
  };
  HTMLDialogElement.prototype.close = function close() {
    this.removeAttribute('open');
  };
});

beforeEach(() => {
  quickSwitcher.close();
  flushSync();
  queryClient.clear();
  resetUserStoresForTests();
  stores.clear();
  mocks.store.navigation.isInitialLoading = false;
  mocks.store.permissions.canStartDMs = true;
  mocks.store.isAuthenticated = true;
  mocks.store.realtimeSync.hasUsableProjection = true;
  mocks.store.projection.users = getUserStore('origin', 'test-session');
  mocks.store.projection.rooms = new SvelteMap();
  installQueryMocks();
  mocks.goto.mockReset();
  mocks.toastError.mockReset();
  mocks.recents.urls = [];
  mocks.recents.record.mockClear();
  mocks.mutation.mockClear();
  mocks.startDM.mockClear();
  mocks.listRooms.mockClear();
  mocks.listRoomMembers.mockClear();
  mocks.listUsers.mockClear();
  mocks.searchMessages.mockReset();
  mocks.store.messageSearch.ensureStatus.mockReset();
  mocks.store.messageSearch.available = true;
  mocks.store.messageSearch.privacyRevision = 0;
  mocks.store.messageSearch.subscribePrivacyInvalidation.mockClear();
  mocks.privacyListeners = [];
  mocks.store.serverInfo.supportsFeature.mockReset();
  mocks.store.serverInfo.supportsFeature.mockReturnValue(true);
  mocks.servers.splice(1);
  mocks.query.mockClear();
});

afterEach(() => {
  quickSwitcher.close();
  flushSync();
  currentRender?.unmount();
  currentRender = undefined;
});

afterAll(() => {
  HTMLDialogElement.prototype.showModal = originalShowModal;
  HTMLDialogElement.prototype.close = originalClose;
});

describe('QuickSwitcher', () => {
  it('opens with server, destination, room, and DM results from mocked data', async () => {
    const { container } = await renderOpenSwitcher();

    expect(container.textContent).toContain('Notifications');
    expect(container.textContent).toContain('Workspace Server');
    expect(container.textContent).toContain('general');
    expect(container.textContent).toContain('River Teammate');
    expect(container.textContent).not.toContain('Empty Conversation');
    expect(input(container)).toBe(document.activeElement);
    expect(mocks.listRooms).not.toHaveBeenCalled();
    expect(mocks.listRoomMembers).not.toHaveBeenCalled();
  });

  it('fuzzy-filters rooms and shows no results for misses', async () => {
    const { container } = await renderOpenSwitcher();
    const initialCount = resultButtons(container).length;

    setSearch(container, 'xylophone');
    await vi.waitFor(() => {
      expect(container.textContent).toContain('xylophone-chat');
      expect(resultButtons(container).length).toBeLessThan(initialCount);
    });

    setSearch(container, 'zzzznothing');
    await vi.waitFor(() => {
      expect(container.textContent).toContain('No results');
    });
  });

  it('limits # searches to channel rooms', async () => {
    const { container } = await renderOpenSwitcher();

    setSearch(container, '#');

    await vi.waitFor(() => {
      expect(container.textContent).toContain('general');
      expect(container.textContent).toContain('xylophone-chat');
      expect(container.textContent).not.toContain('Notifications');
      expect(container.textContent).not.toContain('River Teammate');
    });
  });

  it('records and navigates when selecting a room with Enter', async () => {
    const { container } = await renderOpenSwitcher();

    setSearch(container, '#xylophone');
    input(container).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    );

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-xylophone');
    });
    expect(mocks.recents.record).toHaveBeenCalledWith('/chat/-/room-xylophone');
    expect(dialog(container).hasAttribute('open')).toBe(false);
  });

  it('navigates to the server overview from the server result', async () => {
    const { container } = await renderOpenSwitcher();

    setSearch(container, 'workspace');
    const serverResult = resultButtons(container).find((button) =>
      button.textContent?.includes('Workspace Server')
    );
    expect(serverResult).toBeTruthy();
    serverResult!.click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/overview');
    });
    expect(mocks.recents.record).toHaveBeenCalledWith('/chat/-/overview');
  });

  it('finds existing conversations locally and opens the room without starting a DM', async () => {
    mocks.store.permissions.canStartDMs = false;
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'river');
    expect(resultButtons(container)).toHaveLength(1);
    expect(container.textContent).toContain('River Teammate');
    expect(container.querySelector('[class~="icon-[uil--spinner-alt]"]')).toBeNull();
    input(container).dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-existing');
    });
    expect(mocks.startDM).not.toHaveBeenCalled();
    expect(mocks.listUsers).not.toHaveBeenCalled();
    expect(mocks.recents.record).toHaveBeenCalledWith('/chat/-/dm-existing');
  });

  it('shows local conversations while another server loads and preserves selection on arrival', async () => {
    mocks.servers.push({ id: 'second', url: 'https://second.example.test', name: 'Second' });
    publishNavigation('second', { rooms: [], isInitialLoading: true });
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'river');
    expect(resultButtons(container)).toHaveLength(1);
    expect(container.querySelector('[class~="icon-[uil--spinner-alt]"]')).not.toBeNull();
    publishNavigation('second', {
      isInitialLoading: false,
      rooms: [{ id: 'dm-second', name: '', type: RoomKind.DM, viewerIsMember: true,
        hasMessageHistory: true, members: [currentUser, user('other', 'river', 'River')] }]
    });
    expect(resultButtons(container)).toHaveLength(2);
    expect(container.querySelector('[class~="icon-[uil--spinner-alt]"]')).toBeNull();
    input(container).dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-existing'));
    expect(mocks.listUsers).not.toHaveBeenCalled();
  });

  it('distinguishes incomplete catalogues from an empty local search', async () => {
    mocks.store.navigation.isInitialLoading = true;
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'missing');
    expect(container.textContent).not.toContain('No results');
    publishNavigation('origin', { ...mocks.store.navigation, isInitialLoading: false });
    expect(container.textContent).toContain('No results');
    expect(mocks.listUsers).not.toHaveBeenCalled();
  });

  it('limits group DM avatars while marking bots after the first two participants', async () => {
    mocks.store.navigation.rooms.push({
      id: 'dm-group',
      name: '',
      type: RoomKind.DM,
      viewerIsMember: true,
      members: [
        user('first', 'first', 'First'),
        user('second', 'second', 'Second'),
        user('helper', 'helper_bot', 'Group Helper', true)
      ]
    });
    const { container } = await renderOpenSwitcher();
    const row = resultButtons(container).find((button) => button.textContent?.includes('Group Helper'))!;
    expect(row.querySelectorAll('.command-palette-leading [role="img"]')).toHaveLength(2);
    expect(row.querySelector('[data-testid="bot-badge"]')?.previousElementSibling?.textContent).toBe('Group Helper');
  });

  it('matches group participants and self-DMs by login and display name', async () => {
    mocks.store.navigation.rooms.push(
      { id: 'dm-group', name: '', type: RoomKind.DM, viewerIsMember: true, hasMessageHistory: true,
        members: [currentUser, user('one', 'uniquehandle', 'Cedar'), user('two', 'anotherhandle', 'Maple')] },
      { id: 'dm-self', name: '', type: RoomKind.DM, viewerIsMember: true, hasMessageHistory: true,
        members: [currentUser] }
    );
    const { container } = await renderOpenSwitcher();
    for (const query of ['uniquehandle', 'Cedar', 'anotherhandle', 'Maple', 'uniquehandle Maple']) {
      setSearch(container, query);
      expect(resultButtons(container)).toHaveLength(1);
      expect(resultButtons(container)[0].textContent).toContain('Cedar');
    }
    for (const query of ['alice', 'Alice Current', 'You']) {
      setSearch(container, query);
      expect(resultButtons(container)).toHaveLength(1);
      expect(resultButtons(container)[0].textContent).toContain('Alice Current');
      expect(resultButtons(container)[0].querySelector('[data-testid="you-badge"]')).not.toBeNull();
    }
    expect(mocks.listUsers).not.toHaveBeenCalled();
  });

  it('updates searchable conversations after first messages, profile changes, and removal', async () => {
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'empty');
    expect(resultButtons(container)).toHaveLength(0);
    const rooms = mocks.store.navigation.rooms.map((room) => room.id === 'dm-empty'
      ? { ...room, hasMessageHistory: true } : room);
    publishNavigation('origin', { rooms, isInitialLoading: false });
    expect(resultButtons(container)).toHaveLength(1);

    const renamed = rooms.map((room) => room.id === 'dm-empty'
      ? { ...room, members: [currentUser, user('user-empty', 'newhandle', 'Renamed Conversation')] } : room);
    publishNavigation('origin', { rooms: renamed, isInitialLoading: false });
    expect(resultButtons(container)).toHaveLength(0);
    setSearch(container, 'newhandle');
    expect(resultButtons(container)[0].textContent).toContain('Renamed Conversation');
    publishNavigation('origin', { rooms: renamed.filter((room) => room.id !== 'dm-empty'), isInitialLoading: false });
    expect(resultButtons(container)).toHaveLength(0);
    expect(mocks.listUsers).not.toHaveBeenCalled();
  });

  it('ignores unauthenticated server loading and keeps message mode independent of catalogue loading', async () => {
    const { container } = await renderOpenSwitcher();
    stores.set('origin', { ...mocks.store, isAuthenticated: false,
      navigation: { ...mocks.store.navigation, isInitialLoading: true } });
    setSearch(container, 'missing');
    expect(container.textContent).toContain('No results');
    publishNavigation('origin', { ...mocks.store.navigation, isInitialLoading: true });
    setSearch(container, '?');
    expect(container.querySelector('[class~="icon-[uil--spinner-alt]"]')).toBeNull();
  });

  it('searches known users locally only while typing and starts their DM destination', async () => {
    mocks.store.projection.users.set('known', new DirectoryMember({
      user: { id: 'known', login: 'cedar_handle', displayName: 'Cedar Person' }
    }));
    const { container } = await renderOpenSwitcher();
    expect(container.textContent).not.toContain('Cedar Person');
    setSearch(container, 'cedar_handle');
    expect(resultButtons(container)).toHaveLength(1);
    expect(container.textContent).toContain('Cedar Person');
    expect(mocks.listUsers).not.toHaveBeenCalled();
    resultButtons(container)[0].click();
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm/known'));
  });

  it('finds a bot loaded only by the room directory and opens its DM', async () => {
    const bot = new DirectoryMember({
      user: { id: 'test-bot', login: 'test_bot', displayName: 'TestBot', bot: { ownerUserId: 'owner' } }
    });
    getUserStore('origin', 'old-session').set('test-bot', bot);
    getUserStore('other-server', 'test-session').set('test-bot', bot);
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'test');
    expect(resultButtons(container)).toHaveLength(0);
    // Directory reads populate the same owner as realtime updates.
    getUserStore('origin', 'test-session').set('test-bot', bot);
    flushSync();
    expect(resultButtons(container)).toHaveLength(1);
    expect(container.textContent).toContain('TestBot');
    expect(container.textContent).toContain('BOT');
    expect(mocks.listUsers).not.toHaveBeenCalled();
    resultButtons(container)[0].click();
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm/test-bot'));
  });

  it('updates cached profiles and respects removal markers and session cleanup', async () => {
    const profile = new DirectoryMember({ user: { id: 'known', login: 'cedar', displayName: 'Cedar' } });
    mocks.store.projection.users.set('known', profile);
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'cedar');
    expect(resultButtons(container)).toHaveLength(1);
    getUserStore('origin', 'test-session').set('known', new DirectoryMember({
      user: { id: 'known', login: 'maple', displayName: 'Maple' }
    }));
    flushSync();
    expect(resultButtons(container)).toHaveLength(0);
    setSearch(container, 'maple');
    expect(resultButtons(container)).toHaveLength(1);
    getUserStore('origin', 'test-session').delete('known');
    flushSync();
    expect(resultButtons(container)).toHaveLength(0);
    setSearch(container, 'cedar');
    expect(resultButtons(container)).toHaveLength(0);
    mocks.store.projection.users.clear();
    getUserStore('origin', 'test-session').set('known', profile);
    flushSync();
    expect(resultButtons(container)).toHaveLength(1);
    clearUserStores('origin');
    flushSync();
    expect(resultButtons(container)).toHaveLength(0);
  });

  it('rejects a cached user selection after access is lost before the row updates', async () => {
    getUserStore('origin', 'test-session').set('known', new DirectoryMember({
      user: { id: 'known', login: 'cedar', displayName: 'Cedar' }
    }));
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'cedar');
    const row = resultButtons(container)[0];
    stores.set('origin', { ...mocks.store, isAuthenticated: false });
    row.click();
    flushSync();
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.startDM).not.toHaveBeenCalled();
  });

  it('deduplicates one-to-one and self-DMs but retains group participants as known users', async () => {
    for (const member of [currentUser, teammate, user('group-peer', 'rivergroup', 'River Group')]) {
      mocks.store.projection.users.set(member.id, new DirectoryMember({
        user: { id: member.id, login: member.login, displayName: member.displayName }
      }));
    }
    mocks.store.navigation.rooms.push(
      { id: 'self', name: '', type: RoomKind.DM, viewerIsMember: true, hasMessageHistory: true, members: [currentUser] },
      { id: 'group', name: '', type: RoomKind.DM, viewerIsMember: true, hasMessageHistory: true,
        members: [currentUser, user('group-peer', 'rivergroup', 'River Group')] }
    );
    // One group participant has not loaded; membership must still identify a group.
    mocks.store.projection.rooms.set('group', new RoomWithViewerState({ memberUserIds: ['user-current', 'group-peer', 'unloaded'] }));
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'river');
    expect(resultButtons(container)).toHaveLength(3);
    expect(resultButtons(container).at(-1)?.textContent).toContain('@rivergroup');
    expect(resultButtons(container).filter((button) => button.textContent?.includes('@river ·'))).toHaveLength(0);
    setSearch(container, 'alice');
    expect(resultButtons(container)).toHaveLength(1);
    expect(resultButtons(container)[0].querySelector('[data-testid="you-badge"]')).not.toBeNull();
  });

  it('refreshes known profiles and removes them after deletion, reset, or loss of access', async () => {
    const member = new DirectoryMember({ user: { id: 'known', login: 'cedar', displayName: 'Cedar' } });
    mocks.store.projection.users.set('known', member);
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'cedar');
    expect(resultButtons(container)).toHaveLength(1);
    mocks.store.projection.users.set('known', new DirectoryMember({ user: { id: 'known', login: 'maple', displayName: 'Maple' } }));
    flushSync();
    expect(resultButtons(container)).toHaveLength(0);
    setSearch(container, 'maple');
    expect(resultButtons(container)).toHaveLength(1);
    mocks.store.projection.users.clear();
    flushSync();
    expect(resultButtons(container)).toHaveLength(0);
    mocks.store.projection.users.set('known', member);
    setSearch(container, 'cedar');
    expect(resultButtons(container)).toHaveLength(1);
    mocks.store.projection.users.set('known', new DirectoryMember({ user: { ...member.user, deleted: true } }));
    flushSync();
    expect(resultButtons(container)).toHaveLength(0);
    mocks.store.projection.users.set('known', member);
    for (const override of [
      { permissions: { canStartDMs: false } },
      { isAuthenticated: false },
      { realtimeSync: { hasUsableProjection: false } }
    ]) {
      stores.set('origin', { ...mocks.store, ...override });
      flushSync();
      expect(resultButtons(container)).toHaveLength(0);
    }
  });

  it('keeps the same known user ID separate across servers and opens the selected server', async () => {
    mocks.servers.push({ id: 'second', url: 'https://second.example.test', name: 'Second' });
    mocks.store.projection.users.set('known', new DirectoryMember({ user: { id: 'known', login: 'cedar', displayName: 'Cedar' } }));
    const { container } = await renderOpenSwitcher();
    setSearch(container, 'cedar');
    expect(resultButtons(container)).toHaveLength(2);
    resultButtons(container)[1].click();
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalledWith('/chat/second/dm/known'));
  });

  it('matches bot conversations by login and marks their names', async () => {
    mocks.store.navigation.rooms.push({ id: 'dm-bot', name: '', type: RoomKind.DM,
      viewerIsMember: true, hasMessageHistory: true,
      members: [currentUser, user('user-helper', 'helper_bot', 'Helper', true)] });
    const { container } = await renderOpenSwitcher();

    setSearch(container, 'helper_bot');
    await vi.waitFor(() => {
      expect(container.querySelector('[aria-label="helper_bot"]')).not.toBeNull();
      expect(container.querySelector('[data-testid="bot-badge"]')?.textContent).toBe('BOT');
    });
  });

  it('merges message search results across servers by provider relevance score', async () => {
    mocks.servers.push({
      id: 'second',
      url: 'https://second.example.test',
      name: 'Second Server'
    });
    mocks.searchMessages
      .mockResolvedValueOnce({
        results: [
          {
            id: 'message-low',
            roomId: 'room-general',
            roomName: 'general',
            roomKind: RoomKind.CHANNEL,
            actorId: 'user-current',
            actor: currentUser,
            body: 'A lower ranked result',
            createdAt: '2026-07-30T12:00:00.000Z',
            threadRootEventId: null,
            attachmentCount: 0,
            relevanceScore: 2.5
          }
        ],
        nextCursor: null
      })
      .mockResolvedValueOnce({
        results: [
          {
            id: 'message-high',
            roomId: 'room-search',
            roomName: 'search',
            roomKind: RoomKind.CHANNEL,
            actorId: 'user-teammate',
            actor: { ...teammate, isBot: true },
            body: 'The highest ranked result has enough text to wrap naturally across multiple lines in the palette',
            createdAt: '2026-07-29T12:00:00.000Z',
            threadRootEventId: 'thread-root',
            attachmentCount: 0,
            relevanceScore: 9.75
          }
        ],
        nextCursor: null
      });

    const { container } = await renderOpenSwitcher();
    setSearch(container, '? ranking');

    await vi.waitFor(() => expect(mocks.searchMessages).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(resultButtons(container)).toHaveLength(2));
    const buttons = resultButtons(container);
    expect(buttons[0]?.textContent).toContain('The highest ranked result');
    expect(buttons[1]?.textContent).toContain('A lower ranked result');
    const messageExcerpt = buttons[0]!.querySelector('.line-clamp-2');
    expect(messageExcerpt?.textContent).toContain('wrap naturally across multiple lines');
    expect(messageExcerpt?.getAttribute('dir')).toBe('auto');
    expect(
      buttons[0]!.querySelector('[data-testid="message-search-provenance"]')?.textContent
    ).toBe('River TeammateBOT · #search · Workspace Server');
    expect(buttons[0]!.querySelector('[data-testid="message-search-provenance"] [data-testid="bot-badge"]')).not.toBeNull();
    expect(
      buttons[0]!.querySelector('[data-testid="message-search-provenance"]')?.getAttribute('dir')
    ).toBe('auto');

    buttons[0]!.click();
    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith(
        '/chat/second/room-search/thread-root/m/message-high'
      );
    });
    expect(mocks.recents.record).not.toHaveBeenCalled();
    expect(mocks.searchMessages).toHaveBeenCalledWith({
      query: 'ranking',
      order: MessageSearchOrder.RELEVANCE,
      pageSize: 10
    });
  });

  it('shows healthy message results without waiting for a stalled server', async () => {
    mocks.servers.push({
      id: 'second',
      url: 'https://second.example.test',
      name: 'Second Server'
    });
    mocks.searchMessages.mockReturnValueOnce(new Promise(() => {})).mockResolvedValueOnce({
      results: [
        {
          id: 'message-healthy',
          roomId: 'room-general',
          roomName: 'general',
          roomKind: RoomKind.CHANNEL,
          actorId: 'user-current',
          actor: currentUser,
          body: 'A result from the healthy server',
          createdAt: '2026-07-30T12:00:00.000Z',
          threadRootEventId: null,
          attachmentCount: 0,
          relevanceScore: 5
        }
      ],
      nextCursor: null
    });

    const { container } = await renderOpenSwitcher();
    setSearch(container, '? available');

    await vi.waitFor(() => expect(mocks.searchMessages).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => {
      expect(container.textContent).toContain('A result from the healthy server');
    });
  });

  it('selects the result under a moving pointer', async () => {
    const { container } = await renderOpenSwitcher();
    const target = resultButtons(container).find((button) => button.textContent?.includes('xylophone-chat'))!;
    target.dispatchEvent(new PointerEvent('pointermove', { bubbles: true }));
    input(container).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    );
    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-xylophone');
    });
  });

  it('preserves keyboard selection when a slower server changes the ranking', async () => {
    mocks.servers.push({
      id: 'second',
      url: 'https://second.example.test',
      name: 'Second Server'
    });
    let resolveSlow!: (page: { results: unknown[]; nextCursor: null }) => void;
    const slow = new Promise<{ results: unknown[]; nextCursor: null }>(
      (resolve) => (resolveSlow = resolve)
    );
    const message = (id: string, body: string, relevanceScore: number) => ({
      id,
      roomId: 'room-general',
      roomName: 'general',
      roomKind: RoomKind.CHANNEL,
      actorId: 'user-current',
      actor: currentUser,
      body,
      createdAt: '2026-07-30T12:00:00.000Z',
      threadRootEventId: null,
      attachmentCount: 0,
      relevanceScore
    });
    mocks.searchMessages
      .mockResolvedValueOnce({
        results: [
          message('fast-first', 'Fast first result', 5),
          message('fast-selected', 'Fast selected result', 4)
        ],
        nextCursor: null
      })
      .mockReturnValueOnce(slow);

    const { container } = await renderOpenSwitcher();
    setSearch(container, '? ranking');
    await vi.waitFor(() => expect(resultButtons(container)).toHaveLength(2));
    input(container).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true })
    );

    resolveSlow({ results: [message('slow-high', 'Slow highest result', 10)], nextCursor: null });
    await vi.waitFor(() => expect(resultButtons(container)).toHaveLength(3));
    // Reordering can move a different row under a stationary pointer. A boundary
    // event must not replace the selection made with the keyboard.
    resultButtons(container)[1].dispatchEvent(new PointerEvent('pointerenter'));
    input(container).dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    );

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-general/m/fast-selected');
    });
  });

  it('finishes with no results when one server stalls', async () => {
    mocks.servers.push({
      id: 'second',
      url: 'https://second.example.test',
      name: 'Second Server'
    });
    mocks.searchMessages
      .mockReturnValueOnce(new Promise(() => {}))
      .mockResolvedValueOnce({ results: [], nextCursor: null });

    const { container } = await renderOpenSwitcher();
    setSearch(container, '? missing');

    await vi.waitFor(() => expect(mocks.searchMessages).toHaveBeenCalledTimes(2));
    await vi.waitFor(
      () => {
        expect(container.textContent).toContain('No messages found');
        expect(container.querySelector('[class~="icon-[uil--spinner-alt]"]')).toBeNull();
      },
      { timeout: 4_000 }
    );
  });

  it('purges message plaintext when the server raises a privacy fence', async () => {
    mocks.searchMessages.mockResolvedValue({
      results: [
        {
          id: 'message-private',
          roomId: 'room-private',
          roomName: 'private',
          roomKind: RoomKind.CHANNEL,
          actorId: 'user-current',
          actor: currentUser,
          body: 'Private search result plaintext',
          createdAt: '2026-07-30T12:00:00.000Z',
          threadRootEventId: null,
          attachmentCount: 0,
          relevanceScore: 5
        }
      ],
      nextCursor: null
    });

    const { container } = await renderOpenSwitcher();
    setSearch(container, '? private');
    await vi.waitFor(() => {
      expect(container.textContent).toContain('Private search result plaintext');
    });

    mocks.privacyListeners.at(-1)!(() => true, false);
    flushSync();

    expect(container.textContent).not.toContain('Private search result plaintext');
  });

  it('fences an in-flight response when privacy changes before it resolves', async () => {
    let resolveSearch!: (page: { results: unknown[]; nextCursor: null }) => void;
    const pending = new Promise<{ results: unknown[]; nextCursor: null }>(
      (resolve) => (resolveSearch = resolve)
    );
    mocks.searchMessages
      .mockReturnValueOnce(pending)
      .mockResolvedValue({ results: [], nextCursor: null });

    const { container } = await renderOpenSwitcher();
    setSearch(container, '? private');
    await vi.waitFor(() => expect(mocks.searchMessages).toHaveBeenCalledOnce());

    mocks.privacyListeners.at(-1)!(() => false, false);
    resolveSearch({
      results: [
        {
          id: 'message-stale',
          roomId: 'room-private',
          roomName: 'private',
          roomKind: RoomKind.CHANNEL,
          actorId: 'user-current',
          actor: currentUser,
          body: 'Stale in-flight plaintext',
          createdAt: '2026-07-30T12:00:00.000Z',
          threadRootEventId: null,
          attachmentCount: 0,
          relevanceScore: 5
        }
      ],
      nextCursor: null
    });
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(container.textContent).not.toContain('Stale in-flight plaintext');
  });
});
