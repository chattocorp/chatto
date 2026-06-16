import { beforeEach, describe, expect, it, vi } from 'vitest';

const { queryMock } = vi.hoisted(() => ({
  queryMock: vi.fn()
}));

vi.mock('$app/environment', () => ({
  browser: true
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$lib/gql', () => ({
  graphql: vi.fn(() => ({}))
}));

vi.mock('$lib/state/server/graphqlClient.svelte', () => ({
  graphqlClientManager: {
    originClient: {
      client: {
        query: queryMock
      }
    }
  }
}));

const user = {
  id: 'U1',
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
  presenceStatus: 'ONLINE',
  hasVerifiedEmail: true,
  settings: { timezone: 'UTC', timeFormat: '24h' }
};

async function loadModule() {
  vi.resetModules();
  return import('./loadAuth');
}

describe('loadCurrentUser', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('refreshes from the server on each call', async () => {
    const { loadCurrentUser } = await loadModule();
    queryMock
      .mockResolvedValueOnce({ data: { viewer: { user } }, error: null })
      .mockResolvedValueOnce({
        data: { viewer: { user: { ...user, displayName: 'Alice Fresh' } } },
        error: null
      });

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual({ ...user, displayName: 'Alice Fresh' });
    expect(queryMock).toHaveBeenCalledTimes(2);
  });

  it('keeps the cached user when a later refresh errors', async () => {
    const { loadCurrentUser } = await loadModule();
    queryMock
      .mockResolvedValueOnce({ data: { viewer: { user } }, error: null })
      .mockResolvedValueOnce({ data: undefined, error: { message: 'not found' } });

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual(user);
  });

  it('clears the cached user on a clean viewer=null response', async () => {
    const { loadCurrentUser } = await loadModule();
    queryMock
      .mockResolvedValueOnce({ data: { viewer: { user } }, error: null })
      .mockResolvedValueOnce({ data: { viewer: null }, error: null });

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toBeNull();
  });

  it('returns null when the first load cannot determine a user', async () => {
    const { loadCurrentUser } = await loadModule();
    queryMock.mockResolvedValue({ data: undefined, error: { message: 'unreachable' } });

    expect(await loadCurrentUser()).toBeNull();
  });
});
