import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  page: { params: { serverId: '-', userId: 'recipient' } },
  startDM: vi.fn(),
  ensureRoomAvailable: vi.fn(),
  goto: vi.fn(),
  record: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return mocks.page.params;
    }
  }
}));
vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    path.replace('[serverId]', params.serverId).replace('[roomId]', params.roomId)
}));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: () => '-' }));
vi.mock('$lib/state/recentQuickSwitcher.svelte', () => ({
  recentQuickSwitcher: { record: mocks.record }
}));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

import Destination from './+page.svelte';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

let mounted: ReturnType<typeof render> | undefined;
let server: TestServerScope;
beforeEach(() => {
  vi.resetAllMocks();
  const page = $state({ params: { serverId: '-', userId: 'recipient' } });
  mocks.page = page;
  server = createTestServerScope({
    serverId: 'origin',
    viewer: { id: 'self' },
    api: { startDM: mocks.startDM },
    store: { ensureRoomAvailable: mocks.ensureRoomAvailable }
  });
  mocks.startDM.mockResolvedValue({ id: 'dm-room' });
  mocks.ensureRoomAvailable.mockResolvedValue(undefined);
});
afterEach(async () => {
  await mounted?.unmount();
  mounted = undefined;
});

describe('DM destination', () => {
  it('shows loading until both the command and authoritative room refresh finish', async () => {
    const command = deferred<{ id: string }>();
    const refresh = deferred<void>();
    mocks.startDM.mockReturnValue(command.promise);
    mocks.ensureRoomAvailable.mockReturnValue(refresh.promise);
    mounted = render(Destination);
    await expect.element(mounted.getByRole('status', { name: 'Loading...' })).toBeInTheDocument();
    expect(mocks.startDM).toHaveBeenCalledWith(['recipient']);
    expect(mocks.goto).not.toHaveBeenCalled();
    command.resolve({ id: 'dm-room' });
    await vi.waitFor(() => expect(mocks.ensureRoomAvailable).toHaveBeenCalledWith('dm-room'));
    expect(mocks.goto).not.toHaveBeenCalled();
    refresh.resolve();
    await vi.waitFor(() =>
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-room', { replaceState: true })
    );
    expect(mocks.record).toHaveBeenCalledWith('/chat/-/dm-room');
  });

  it.each(['command', 'refresh', 'missing'])('offers retry after %s failure', async (failure) => {
    if (failure === 'command') mocks.startDM.mockRejectedValueOnce(new Error('Unavailable'));
    if (failure === 'refresh')
      mocks.ensureRoomAvailable.mockRejectedValueOnce(new Error('Unavailable'));
    if (failure === 'missing') mocks.startDM.mockResolvedValueOnce(null);
    mounted = render(Destination);
    const retry = mounted.getByRole('button', { name: 'Try Again' });
    await expect.element(retry).toBeVisible();
    expect(mocks.goto).not.toHaveBeenCalled();
    await retry.click();
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalledOnce());
    expect(mocks.startDM).toHaveBeenCalledTimes(2);
  });

  it.each(['command', 'refresh'])('ignores a late %s result after leaving', async (stage) => {
    const pending = deferred<{ id: string }>();
    if (stage === 'command') mocks.startDM.mockReturnValue(pending.promise);
    else mocks.ensureRoomAvailable.mockReturnValue(pending.promise);
    mounted = render(Destination);
    await vi.waitFor(() =>
      expect(stage === 'command' ? mocks.startDM : mocks.ensureRoomAvailable).toHaveBeenCalled()
    );
    await mounted.unmount();
    mounted = undefined;
    pending.resolve({ id: 'dm-room' });
    await new Promise<void>((done) => queueMicrotask(done));
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('ignores a previous recipient and handles self-DMs', async () => {
    const old = deferred<{ id: string }>();
    mocks.startDM.mockReturnValueOnce(old.promise);
    mounted = render(Destination);
    await vi.waitFor(() => expect(mocks.startDM).toHaveBeenCalledOnce());
    mocks.page.params.userId = 'self';
    flushSync();
    await vi.waitFor(() => expect(mocks.goto).toHaveBeenCalledOnce());
    expect(mocks.startDM).toHaveBeenLastCalledWith([]);
    old.resolve({ id: 'old-room' });
    await new Promise<void>((done) => queueMicrotask(done));
    expect(mocks.ensureRoomAvailable).not.toHaveBeenCalledWith('old-room');
    expect(mocks.goto).toHaveBeenCalledOnce();
  });

  it('waits for the account before opening a conversation', async () => {
    const viewer = server.currentUser.user;
    server.currentUser.user = undefined;
    mocks.page.params.userId = 'self';
    mounted = render(Destination);
    await expect.element(mounted.getByRole('status', { name: 'Loading...' })).toBeInTheDocument();
    expect(mocks.startDM).not.toHaveBeenCalled();
    server.currentUser.user = viewer;
    flushSync();
    await vi.waitFor(() => expect(mocks.startDM).toHaveBeenCalledWith([]));
    expect(mocks.startDM).toHaveBeenCalledOnce();
  });

  it('does not navigate after its server scope is replaced', async () => {
    const pending = deferred<{ id: string }>();
    mocks.startDM.mockReturnValue(pending.promise);
    mounted = render(Destination);
    await vi.waitFor(() => expect(mocks.startDM).toHaveBeenCalledOnce());
    server.current = false;
    pending.resolve({ id: 'dm-room' });
    await new Promise<void>((done) => queueMicrotask(done));
    expect(mocks.ensureRoomAvailable).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
