import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import UserCombobox from './UserCombobox.svelte';

const mocks = vi.hoisted(() => ({
  listUsers: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    connectBaseUrl: 'http://localhost/api/connect',
    bearerToken: null
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

describe('UserCombobox', () => {
  beforeEach(() => {
    vi.useFakeTimers();
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
    vi.useRealTimers();
  });

  it('searches server members as the actor text changes', async () => {
    const { container } = render(UserCombobox, {
      props: {
        id: 'actor',
        label: 'Actor'
      }
    });

    const input = container.querySelector('input') as HTMLInputElement;
    input.value = 'alice';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await vi.advanceTimersByTimeAsync(220);
    await settle();

    expect(mocks.listUsers).toHaveBeenCalledWith('alice', 10, 0);
  });
});
