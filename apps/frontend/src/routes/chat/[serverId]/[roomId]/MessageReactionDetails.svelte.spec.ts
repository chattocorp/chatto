import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import type { ReactionSummaryView } from '$lib/render/reactions';
import { queryClient } from '$lib/query/client';
import { q } from '$lib/test-utils';
import MessageReactionDetails from './MessageReactionDetails.svelte';

const mocks = vi.hoisted(() => ({
  listReactionUsers: vi.fn(),
  batchGetUsers: vi.fn(),
  deletedIds: new Set<string>()
}));

vi.mock('$lib/api-client/reactions', () => ({
  createReactionAPI: () => ({ listReactionUsers: mocks.listReactionUsers })
}));

vi.mock('$lib/api-client/users', () => ({
  createUserAPI: () => ({ batchGetUsers: mocks.batchGetUsers })
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'server-1',
    connection: {
      queryScope: 'session-1',
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    },
    store: { projection: { users: { isDeleted: (id: string) => mocks.deletedIds.has(id) } } },
    isCurrent: () => true
  })
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({ get: (_key: unknown, fallback: unknown) => fallback })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_id: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_id: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_id: string, fallback: unknown) => fallback
}));

function reaction(emoji: string, count: number): ReactionSummaryView {
  return { emoji, count, hasReacted: false, users: [] };
}

function user(id: string, displayName = id, isBot = false) {
  return { id, login: id, displayName, deleted: false, isBot, avatarUrl: null };
}

function renderDetails(reactions: ReactionSummaryView[]) {
  return render(MessageReactionDetails, {
    props: {
      roomId: 'room-1',
      messageEventId: 'message-1',
      reactions,
      onClose: vi.fn()
    }
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((finish) => {
    resolve = finish;
  });
  return { promise, resolve };
}

beforeEach(() => {
  mocks.deletedIds.clear();
  mocks.listReactionUsers.mockReset();
  mocks.batchGetUsers.mockReset();
  mocks.listReactionUsers.mockImplementation(async (_input, offset: number) => ({
    userIds: offset === 0 ? ['alice'] : [],
    totalCount: 1,
    hasMore: false
  }));
  mocks.batchGetUsers.mockImplementation(async () => [user('alice', 'Alice')]);
});

afterEach(() => {
  queryClient.clear();
  vi.unstubAllGlobals();
});

describe('message reaction details', () => {
  it('shows a complete list for one emoji and preserves bot identity', async () => {
    mocks.listReactionUsers.mockResolvedValue({
      userIds: ['alice', 'bot'],
      totalCount: 2,
      hasMore: false
    });
    mocks.batchGetUsers.mockResolvedValue([user('alice', 'Alice'), user('bot', 'Helper', true)]);
    const { container } = renderDetails([reaction('heart', 2)]);

    await expect.element(q(container, '[role="tabpanel"]')).toHaveTextContent('Alice');
    expect(container.querySelectorAll('[data-testid="reaction-details-user"]')).toHaveLength(2);
    expect(q(container, '[role="tabpanel"]')?.textContent).toContain('Helper');
    expect(q(container, '[role="tabpanel"]')?.textContent).toContain('BOT');
    expect(mocks.listReactionUsers).toHaveBeenCalledWith(
      { roomId: 'room-1', messageEventId: 'message-1', emoji: 'heart' },
      0,
      50,
      expect.any(AbortSignal)
    );
  });

  it('shows only the selected emoji when an earlier read finishes late', async () => {
    const first = deferred<{ userIds: string[]; totalCount: number; hasMore: boolean }>();
    mocks.listReactionUsers.mockImplementation(({ emoji }: { emoji: string }) =>
      emoji === 'heart'
        ? first.promise
        : Promise.resolve({ userIds: ['bob'], totalCount: 1, hasMore: false })
    );
    mocks.batchGetUsers.mockImplementation(async (ids: string[]) =>
      ids.map((id) => user(id, id === 'bob' ? 'Bob' : 'Alice'))
    );
    const { container } = renderDetails([reaction('heart', 1), reaction('thumbsup', 1)]);
    await vi.waitFor(() => expect(mocks.listReactionUsers).toHaveBeenCalledOnce());

    (container.querySelectorAll('[role="tab"]')[1] as HTMLButtonElement).click();
    await expect.element(q(container, '[role="tabpanel"]')).toHaveTextContent('Bob');
    first.resolve({ userIds: ['alice'], totalCount: 1, hasMore: false });
    await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
    expect(q(container, '[role="tabpanel"]')?.textContent).not.toContain('Alice');
    expect(container.querySelectorAll('[aria-selected="true"]')).toHaveLength(1);
  });

  it('loads the next bounded page when its sentinel becomes visible', async () => {
    let notify: IntersectionObserverCallback | undefined;
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(callback: IntersectionObserverCallback) {
          notify = callback;
        }
        observe() {}
        unobserve() {}
        disconnect() {}
      }
    );
    mocks.listReactionUsers.mockImplementation(async (_input, offset: number) => ({
      userIds:
        offset === 0 ? Array.from({ length: 50 }, (_, index) => `user-${index}`) : ['user-50'],
      totalCount: 51,
      hasMore: offset === 0
    }));
    mocks.batchGetUsers.mockImplementation(async (ids: string[]) => ids.map((id) => user(id)));
    const { container } = renderDetails([reaction('heart', 51)]);
    await vi.waitFor(() =>
      expect(container.querySelectorAll('[data-testid="reaction-details-user"]')).toHaveLength(50)
    );

    notify?.([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver);
    await vi.waitFor(() =>
      expect(container.querySelectorAll('[data-testid="reaction-details-user"]')).toHaveLength(51)
    );
    expect(mocks.listReactionUsers).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ emoji: 'heart' }),
      50,
      50,
      expect.any(AbortSignal)
    );
  });

  it('uses a fallback when an account profile is unavailable', async () => {
    mocks.batchGetUsers.mockResolvedValue([]);
    const { container } = renderDetails([reaction('heart', 1)]);
    await expect.element(q(container, '[role="tabpanel"]')).toHaveTextContent('Unknown user');
  });

  it('offers retry after a failed read and hides names after access is lost', async () => {
    mocks.listReactionUsers.mockRejectedValue(new Error('offline'));
    const { container } = renderDetails([reaction('heart', 1)]);
    await vi.waitFor(() => expect(q(container, '[role="alert"]')).not.toBeNull(), {
      timeout: 5000
    });
    mocks.listReactionUsers.mockResolvedValue({
      userIds: ['alice'],
      totalCount: 1,
      hasMore: false
    });
    (q(container, '[role="alert"] button') as HTMLButtonElement).click();
    await expect.element(q(container, '[role="tabpanel"]')).toHaveTextContent('Alice');

    mocks.listReactionUsers.mockRejectedValue(new ConnectError('denied', Code.PermissionDenied));
    await queryClient.invalidateQueries({ queryKey: ['server', 'server-1'] });
    await vi.waitFor(() => expect(q(container, '[role="alert"]')).not.toBeNull());
    expect(container.querySelectorAll('[data-testid="reaction-details-user"]')).toHaveLength(0);
    expect(q(container, '[role="alert"] button')).toBeNull();
  });
});
