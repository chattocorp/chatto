import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { q } from '$lib/test-utils';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { quickSwitcher } from '$lib/state/globals.svelte';

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  query: vi.fn(),
  mutation: vi.fn(),
  startDM: vi.fn(),
  listRooms: vi.fn(),
  listRoomMembers: vi.fn(),
  listUsers: vi.fn(),
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
    serverInfo: {
      name: 'Workspace Server',
      iconUrl: null
    },
    permissions: {
      canStartDMs: true
    },
    currentUser: {
      user: {
        id: 'user-current'
      }
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '').replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: () => '-',
  segmentToServerId: (segment: string) => (segment === '-' ? 'origin' : null)
}));

vi.mock('$lib/render/data', () => ({
  useRenderData: (_document: unknown, value: unknown) => value
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    tryGetStore: vi.fn(() => mocks.store)
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: () => ({
      connectBaseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token-1',
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

vi.mock('$lib/api-client/memberDirectory', () => ({
  createMemberDirectoryAPI: vi.fn(() => ({
    listRoomMembers: mocks.listRoomMembers,
    listUsers: mocks.listUsers
  }))
}));

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

type User = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl: string | null;
  presenceStatus: string;
};

function user(id: string, login: string, displayName: string): User {
  return {
    id,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: 'ONLINE'
  };
}

const currentUser = user('user-current', 'alice', 'Alice Current');
const teammate = user('user-teammate', 'river', 'River Teammate');
let currentRender: { unmount: () => void } | undefined;
let originalShowModal: typeof HTMLDialogElement.prototype.showModal;
let originalClose: typeof HTMLDialogElement.prototype.close;

function installQueryMocks() {
  mocks.startDM.mockResolvedValue({ id: 'dm-new' });
  mocks.listRooms.mockResolvedValue([
    {
      id: 'room-general',
      name: 'general',
      kind: RoomKind.CHANNEL,
      isMember: true,
      hasUnread: false
    },
    {
      id: 'room-xylophone',
      name: 'xylophone-chat',
      kind: RoomKind.CHANNEL,
      isMember: true,
      hasUnread: false
    },
    {
      id: 'dm-existing',
      name: '',
      kind: RoomKind.DM,
      isMember: true,
      hasUnread: false
    }
  ]);
  mocks.listRoomMembers.mockImplementation(async (roomId: string) => ({
    members: roomId === 'dm-existing' ? [currentUser, teammate] : [],
    totalCount: roomId === 'dm-existing' ? 2 : 0,
    hasMore: false
  }));
  mocks.listUsers.mockImplementation(async (search: string) => ({
    members:
      search === 'river-login' ? [user('user-river-login', 'river-login', 'River Login')] : [],
    totalCount: search === 'river-login' ? 1 : 0,
    hasMore: false
  }));
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
    'input[placeholder="Go to server, room, or conversation..."]'
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
  return Array.from(container.querySelectorAll<HTMLButtonElement>('button.sidebar-item'));
}

async function waitForDebouncedUserSearch() {
  await new Promise((resolve) => setTimeout(resolve, 250));
  await vi.waitFor(() => {
    expect(mocks.listUsers).toHaveBeenCalledWith('river-login', 20, 0);
  });
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
    expect(input(container)).toBe(document.activeElement);
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

  it('loads searchable server members and starts a DM for user results', async () => {
    const { container } = await renderOpenSwitcher();

    setSearch(container, 'river-login');
    await waitForDebouncedUserSearch();
    await vi.waitFor(() => {
      expect(container.textContent).toContain('River Login');
    });

    resultButtons(container)
      .find((button) => button.textContent?.includes('River Login'))!
      .click();

    await vi.waitFor(() => {
      expect(mocks.startDM).toHaveBeenCalledWith(['user-river-login']);
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-new');
    });
    expect(mocks.recents.record).toHaveBeenCalledWith('/chat/-/dm-new');
  });
});
