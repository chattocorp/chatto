import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import UserCombobox from './UserCombobox.svelte';

const mocks = vi.hoisted(() => ({
  query: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    client: {
      query: mocks.query
    }
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
    mocks.query.mockReset();
    mocks.query.mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({
        data: {
          server: {
            members: {
              users: [
                {
                  id: 'user-1',
                  login: 'alice',
                  displayName: 'Alice Admin',
                  avatarUrl: null
                }
              ]
            }
          }
        },
        error: null
      })
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

    expect(mocks.query).toHaveBeenCalledWith(
      expect.anything(),
      { search: 'alice' },
      { requestPolicy: 'network-only' }
    );
  });
});
