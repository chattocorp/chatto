import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  page: { params: { serverId: '-', userId: 'recipient' } as Record<string, string> },
  startDM: vi.fn(),
  ensureRoomAvailable: vi.fn(),
  loadRoomMembers: vi.fn(),
  goto: vi.fn(),
  record: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/state', () => ({
  page: {
    route: { id: '/chat/[serverId]/user/[userId]' },
    get params() {
      return mocks.page.params;
    }
  }
}));
vi.mock('$app/navigation', () => ({
  pushState: vi.fn(),
  goto: mocks.goto
}));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    path
      .replace('[serverId]', params.serverId)
      .replace('[userId]', params.userId)
      .replace('[roomId]', params.roomId)
}));
vi.mock('$lib/navigation', () => ({
  segmentToServerId: vi.fn(),
  serverIdToSegment: () => '-'
}));
vi.mock('$lib/state/recentQuickSwitcher.svelte', () => ({
  recentQuickSwitcher: { record: mocks.record }
}));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

vi.mock('../../[roomId]/Room.svelte', async () => ({
  default: (await import('$lib/test-utils/ConversationStub.svelte')).default
}));

import Destination from './+layout.svelte';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

let mounted: ReturnType<typeof render> | undefined;
let server: TestServerScope;
// SvelteKit replaces this object on navigation, including thread-only changes.
class TestPage {
  params = $state.raw<Record<string, string>>({ serverId: '-', userId: 'recipient' });
}

beforeEach(() => {
  vi.resetAllMocks();
  mocks.page = new TestPage();
  server = createTestServerScope({
    serverId: 'origin',
    viewer: { id: 'self' },
    api: { startDM: mocks.startDM },
    store: {
      ensureRoomAvailable: mocks.ensureRoomAvailable,
      loadRoomMembers: mocks.loadRoomMembers
    }
  });
  mocks.startDM.mockResolvedValue({ id: 'dm-room' });
  mocks.ensureRoomAvailable.mockResolvedValue(undefined);
});
afterEach(async () => {
  await mounted?.unmount();
  mounted = undefined;
});

describe('user page', () => {
  it('keeps the conversation mounted across thread and message navigation', async () => {
    mounted = render(Destination);
    const conversation = mounted.getByText('Conversation: dm-room');
    await expect.element(conversation).toBeVisible();
    const original = conversation.element();

    const selections: Record<string, string>[] = [
      { threadId: 'thread-1' },
      { threadId: 'thread-2' },
      { threadId: 'thread-2', messageId: 'message-1' },
      {}
    ];
    for (const selection of selections) {
      mocks.page.params = { serverId: '-', userId: 'recipient', ...selection };
      flushSync();
      expect(original.isConnected).toBe(true);
      await expect.element(conversation).toBeVisible();
      expect(conversation.element()).toBe(original);
      expect(mocks.startDM).toHaveBeenCalledOnce();
      expect(mocks.ensureRoomAvailable).toHaveBeenCalledOnce();
      expect(mocks.loadRoomMembers).toHaveBeenCalledExactlyOnceWith('dm-room');
    }
  });

  it.each(['command', 'refresh'])('keeps a pending %s during thread navigation', async (stage) => {
    const pending = deferred<{ id: string }>();
    if (stage === 'command') mocks.startDM.mockReturnValue(pending.promise);
    else mocks.ensureRoomAvailable.mockReturnValue(pending.promise);
    mounted = render(Destination);
    await vi.waitFor(() =>
      expect(stage === 'command' ? mocks.startDM : mocks.ensureRoomAvailable).toHaveBeenCalledOnce()
    );

    mocks.page.params = { serverId: '-', userId: 'recipient', threadId: 'thread-1' };
    flushSync();
    expect(mocks.startDM).toHaveBeenCalledOnce();
    pending.resolve({ id: 'dm-room' });
    await expect.element(mounted.getByText('Conversation: dm-room')).toBeVisible();
    expect(mocks.ensureRoomAvailable).toHaveBeenCalledOnce();
  });

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
    await expect.element(mounted.getByText('Conversation: dm-room')).toBeVisible();
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.record).toHaveBeenCalledWith('/chat/-/user/recipient');
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
    await expect.element(mounted.getByText('Conversation: dm-room')).toBeVisible();
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
    mocks.page.params = { serverId: '-', userId: 'self' };
    flushSync();
    await expect.element(mounted.getByText('Conversation: dm-room')).toBeVisible();
    expect(mocks.startDM).toHaveBeenLastCalledWith([]);
    old.resolve({ id: 'old-room' });
    await new Promise<void>((done) => queueMicrotask(done));
    expect(mocks.ensureRoomAvailable).not.toHaveBeenCalledWith('old-room');
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.record).toHaveBeenCalledWith('/chat/-/user/self');
  });

  it('waits for the account before opening a conversation', async () => {
    const viewer = server.currentUser.user;
    server.currentUser.user = undefined;
    mocks.page.params = { serverId: '-', userId: 'self' };
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
