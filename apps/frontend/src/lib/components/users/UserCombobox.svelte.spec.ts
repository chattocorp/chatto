import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import UserCombobox from './UserCombobox.svelte';

const mocks = vi.hoisted(() => ({
  listServerMembers: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    connectBaseUrl: 'http://localhost/api/connect',
    bearerToken: null
  })
}));

vi.mock('$lib/api/memberDirectory', () => ({
  createMemberDirectoryAPI: () => ({
    listServerMembers: mocks.listServerMembers
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
    mocks.listServerMembers.mockReset();
    mocks.listServerMembers.mockResolvedValue({
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

    expect(mocks.listServerMembers).toHaveBeenCalledWith('alice', 10, 0);
  });
});
