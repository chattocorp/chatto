import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { queryClient } from '$lib/query/client';
import UserCombobox from './UserCombobox.svelte';

const mocks = vi.hoisted(() => ({
  listUsers: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {},
    connection: {
      queryScope: 'origin-session',
      connectBaseUrl: 'http://localhost/api/connect',
      bearerToken: null,
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    }
  })
}));

vi.mock('$lib/api-client/memberDirectory', () => ({
  createMemberDirectoryAPI: () => ({
    listUsers: mocks.listUsers
  })
}));

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function enterSearch(container: Element, search: string) {
  const input = container.querySelector('input') as HTMLInputElement;
  input.value = search;
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe('UserCombobox', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    queryClient.clear();
    mocks.listUsers.mockReset();
    mocks.listUsers.mockResolvedValue({
      members: [
        {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice Admin',
          deleted: false,
          avatarUrl: null,
          presenceStatus: 'ONLINE',
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      totalCount: 1,
      hasMore: false
    });
  });

  afterEach(() => {
    queryClient.clear();
    vi.useRealTimers();
  });

  it('searches server members as the actor text changes', async () => {
    const { container, unmount } = render(UserCombobox, {
      props: {
        id: 'actor',
        label: 'Actor'
      }
    });

    enterSearch(container, 'alice');
    await vi.advanceTimersByTimeAsync(220);
    await settle();

    expect(mocks.listUsers).toHaveBeenCalledWith('alice', 10, 0, {
      signal: expect.any(AbortSignal)
    });

    enterSearch(container, 'bob');
    unmount();
    await vi.advanceTimersByTimeAsync(220);
    expect(mocks.listUsers).toHaveBeenCalledTimes(1);
  });

  it('reuses a fresh cached search after remounting', async () => {
    const first = render(UserCombobox, {
      props: { id: 'actor', label: 'Actor' }
    });
    enterSearch(first.container, 'alice');
    await vi.advanceTimersByTimeAsync(220);
    await settle();
    first.unmount();

    const second = render(UserCombobox, {
      props: { id: 'actor', label: 'Actor' }
    });
    enterSearch(second.container, 'alice');
    await vi.advanceTimersByTimeAsync(220);
    await settle();

    expect(mocks.listUsers).toHaveBeenCalledOnce();
    expect(second.container.textContent).toContain('Alice Admin');
  });

  it('cancels an in-flight search when unmounted', async () => {
    const pending = deferred<never>();
    mocks.listUsers.mockReturnValue(pending.promise);
    const view = render(UserCombobox, {
      props: { id: 'actor', label: 'Actor' }
    });
    enterSearch(view.container, 'alice');
    await vi.advanceTimersByTimeAsync(220);
    await settle();
    const options = mocks.listUsers.mock.calls[0]?.[3] as { signal: AbortSignal };

    view.unmount();

    expect(options.signal.aborted).toBe(true);
  });

  it('hides a superseded search result while the next term is debouncing', async () => {
    const alice = deferred<Awaited<ReturnType<typeof mocks.listUsers>>>();
    mocks.listUsers.mockReturnValueOnce(alice.promise).mockResolvedValue({
      members: [
        {
          id: 'user-2',
          login: 'bob',
          displayName: 'Bob Builder',
          deleted: false,
          avatarUrl: null,
          presenceStatus: 'ONLINE',
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      totalCount: 1,
      hasMore: false
    });
    const view = render(UserCombobox, {
      props: { id: 'actor', label: 'Actor' }
    });
    enterSearch(view.container, 'alice');
    await vi.advanceTimersByTimeAsync(220);
    await settle();

    enterSearch(view.container, 'bob');
    alice.resolve({
      members: [
        {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice Admin',
          deleted: false,
          avatarUrl: null,
          presenceStatus: 'ONLINE',
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      totalCount: 1,
      hasMore: false
    });
    await settle();

    expect(view.container.textContent).not.toContain('Alice Admin');

    await vi.advanceTimersByTimeAsync(220);
    await settle();
    expect(view.container.textContent).toContain('Bob Builder');
  });
});
