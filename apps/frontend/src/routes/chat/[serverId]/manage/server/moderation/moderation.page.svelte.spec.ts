import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { queryClient } from '$lib/query/client';
import { removeRegisteredAdminUserQueries } from '$lib/query/cacheRegistry';
import type { DirectoryMember } from '$lib/api-client/memberDirectory';
import type { RoomBanSummary } from '$lib/api-client/rooms';
import ModerationPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  listBans: vi.fn(),
  unbanMember: vi.fn()
}));

let originalIntersectionObserver: typeof IntersectionObserver;
let observers: MockIntersectionObserver[] = [];

class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null;
  readonly rootMargin: string;
  readonly scrollMargin: string;
  readonly thresholds: ReadonlyArray<number> = [];
  private elements: Element[] = [];

  constructor(
    private readonly callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit
  ) {
    this.root = options?.root ?? null;
    this.rootMargin = options?.rootMargin ?? '0px';
    this.scrollMargin = options?.scrollMargin ?? '0px';
    observers.push(this);
  }

  observe = (target: Element) => {
    this.elements.push(target);
  };

  unobserve = (target: Element) => {
    this.elements = this.elements.filter((element) => element !== target);
  };

  disconnect = () => {
    this.elements = [];
  };

  takeRecords = () => [];

  trigger(isIntersecting: boolean) {
    const target = this.elements[0] ?? document.createElement('tr');
    this.callback(
      [
        {
          boundingClientRect: target.getBoundingClientRect(),
          intersectionRatio: isIntersecting ? 1 : 0,
          intersectionRect: target.getBoundingClientRect(),
          isIntersecting,
          rootBounds: null,
          target,
          time: performance.now()
        }
      ],
      this
    );
  }
}

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: { currentUser: { user: { settings: null } } },
    connection: {
      queryScope: 'moderation-test',
      getAPI: () => ({
        listBans: mocks.listBans,
        unbanMember: mocks.unbanMember
      })
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/components/UserAvatar.svelte', async () => ({
  default: (await import('./ModerationUserAvatarMock.svelte')).default
}));

function directoryMember(id: string, displayName: string): DirectoryMember {
  return {
    id,
    login: id,
    displayName,
    deleted: false,
    avatarUrl: null,
    customStatus: null,
    presenceStatus: 0,
    roles: [],
    createdAt: null
  };
}

function ban(
  id: string,
  user: DirectoryMember | null = null,
  moderator: DirectoryMember | null = null
): RoomBanSummary {
  return {
    id,
    roomId: 'room-1',
    room: {
      id: 'room-1',
      name: 'general',
      description: '',
      archived: false,
      groupId: '',
      universal: false,
      slowModeSeconds: 0
    },
    userId: `user-${id}`,
    user,
    moderatorId: 'moderator-1',
    moderator,
    reason: 'policy',
    createdAt: '2026-07-01T12:00:00Z',
    expiresAt: null
  };
}

function result(bans: ReturnType<typeof ban>[], hasMore = false) {
  return { bans, totalCount: bans.length, hasMore };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('server admin moderation bans', () => {
  beforeEach(() => {
    queryClient.clear();
    originalIntersectionObserver = globalThis.IntersectionObserver;
    observers = [];
    globalThis.IntersectionObserver =
      MockIntersectionObserver as unknown as typeof IntersectionObserver;
    mocks.listBans.mockReset();
    mocks.unbanMember.mockReset();
    mocks.unbanMember.mockResolvedValue(true);
  });

  afterEach(() => {
    queryClient.clear();
    globalThis.IntersectionObserver = originalIntersectionObserver;
  });

  it('loads bans in cancellable offset pages as the table end intersects', async () => {
    mocks.listBans
      .mockResolvedValueOnce(
        result(
          Array.from({ length: 20 }, (_, index) => ban(String(index))),
          true
        )
      )
      .mockResolvedValueOnce(result([ban('20')]));

    const { container } = render(ModerationPage);
    await settle();

    expect(mocks.listBans).toHaveBeenNthCalledWith(
      1,
      { limit: 20, offset: 0 },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(container.textContent).toContain('user-0');
    expect(observers).toHaveLength(1);

    observers[0].trigger(true);
    await settle();

    expect(mocks.listBans).toHaveBeenNthCalledWith(
      2,
      { limit: 20, offset: 20 },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(container.textContent).toContain('user-20');
  });

  it('invalidates and refreshes the scoped bans query after an unban', async () => {
    mocks.listBans.mockResolvedValueOnce(result([ban('1')])).mockResolvedValue(result([]));

    const { container } = render(ModerationPage);
    await settle();

    const rowUnban = [...container.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Unban'
    ) as HTMLButtonElement;
    rowUnban.click();
    await settle();

    const reason = document.querySelector('#unban-room-member-reason') as HTMLTextAreaElement;
    reason.value = 'Appeal approved';
    reason.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = document.querySelector('dialog button[type="submit"]') as HTMLButtonElement;
    submit.click();
    await vi.waitFor(() => expect(mocks.unbanMember).toHaveBeenCalledOnce());
    await vi.waitFor(() => expect(mocks.listBans).toHaveBeenCalledTimes(2));

    expect(mocks.unbanMember).toHaveBeenCalledWith({
      roomId: 'room-1',
      userId: 'user-1',
      reason: 'Appeal approved'
    });
  });

  it('redacts a removed user from mounted ban and moderator summaries without refetching', async () => {
    const removed = directoryMember('removed', 'Removed Person');
    const retained = directoryMember('retained', 'Retained Person');
    mocks.listBans.mockResolvedValue(
      result([ban('subject', removed, retained), ban('moderator', retained, removed)])
    );

    const { container } = render(ModerationPage);
    await settle();
    expect(container.textContent).toContain('Removed Person');

    removeRegisteredAdminUserQueries('origin', 'removed');
    await settle();

    expect(container.textContent).not.toContain('Removed Person');
    expect(container.textContent).toContain('Retained Person');
    expect(mocks.listBans).toHaveBeenCalledOnce();
  });
});
