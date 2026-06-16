import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { q } from '$lib/test-utils';
import { RoomType } from '$lib/gql/graphql';
import { quickSwitcher } from '$lib/state/globals.svelte';

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  query: vi.fn(),
  mutation: vi.fn(),
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
      name: 'Workspace Server'
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: () => '-'
}));

vi.mock('$lib/gql', () => ({
  graphql: (source: string) => source,
  useFragment: (_document: unknown, value: unknown) => value
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get servers() {
      return mocks.servers;
    },
    tryGetStore: vi.fn(() => mocks.store)
  }
}));

vi.mock('$lib/state/server/graphqlClient.svelte', () => ({
  graphqlClientManager: {
    getClient: () => ({
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
    get: (_userId: string, fallback: string) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback
}));

vi.mock('$lib/ui/toast', () => ({
  toast: {
    error: mocks.toastError
  }
}));

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

function queryResult(data: unknown) {
  return {
    toPromise: vi.fn().mockResolvedValue({ data })
  };
}

function mutationResult(data: unknown) {
  return {
    toPromise: vi.fn().mockResolvedValue({ data })
  };
}

function operationName(document: unknown): string {
  if (typeof document === 'string') {
    return document.match(/\b(?:query|mutation)\s+(\w+)/)?.[1] ?? document;
  }

  const definitions = (document as { definitions?: Array<{ name?: { value?: string } }> })
    .definitions;
  return definitions?.[0]?.name?.value ?? '';
}

function installQueryMocks() {
  mocks.query.mockImplementation((document: unknown, variables?: Record<string, unknown>) => {
    const name = operationName(document);

    if (name === 'QuickSwitcherServer') {
      return queryResult({
        server: {
          profile: {
            name: 'E2E Test Server',
            logoUrl: null
          }
        }
      });
    }

    if (name === 'QuickSwitcherRooms') {
      return queryResult({
        viewer: {
          user: {
            id: currentUser.id,
            rooms: [
              {
                id: 'room-general',
                name: 'general',
                type: RoomType.Channel,
                members: {
                  users: [currentUser]
                }
              },
              {
                id: 'room-xylophone',
                name: 'xylophone-chat',
                type: RoomType.Channel,
                members: {
                  users: [currentUser]
                }
              },
              {
                id: 'dm-existing',
                name: '',
                type: RoomType.Dm,
                members: {
                  users: [currentUser, teammate]
                }
              }
            ]
          }
        }
      });
    }

    if (name === 'QuickSwitcherMembers') {
      return queryResult({
        viewer: {
          canStartDMs: true,
          user: {
            id: currentUser.id
          }
        },
        server: {
          members: {
            users:
              variables?.search === 'river-login'
                ? [user('user-river-login', 'river-login', 'River Login')]
                : []
          }
        }
      });
    }

    throw new Error(`Unexpected query document: ${name}`);
  });

  mocks.mutation.mockReturnValue(mutationResult({ startDM: { id: 'dm-new' } }));
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
    expect(
      mocks.query.mock.calls.some(([document]) => operationName(document) === 'QuickSwitcherMembers')
    ).toBe(true);
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
    expect(container.textContent).toContain('E2E Test Server');
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

    setSearch(container, 'e2e test');
    const serverResult = resultButtons(container).find((button) =>
      button.textContent?.includes('E2E Test Server')
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
      const [document, variables] = mocks.mutation.mock.calls[0] ?? [];
      expect(operationName(document)).toBe('QuickSwitcherStartDM');
      expect(variables).toEqual({
        input: {
          participantIds: ['user-river-login']
        }
      });
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-new');
    });
    expect(mocks.recents.record).toHaveBeenCalledWith('/chat/-/dm-new');
  });
});
