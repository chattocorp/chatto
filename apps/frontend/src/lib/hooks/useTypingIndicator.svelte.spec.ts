import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { UserStore } from '@chatto/client/server/users';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { TYPING_TIMEOUT_MS, createTypingIndicator } from './useTypingIndicator.svelte';
import UseTypingIndicatorHarness from './UseTypingIndicatorHarness.svelte';

type TypingSignal = { roomId: string; threadRootEventId: string | null; userId: string };

const { mocks } = vi.hoisted(() => ({
  mocks: {
    batchGetUsers: vi.fn<(ids: string[]) => Promise<unknown[]>>(),
    refreshTypingIndicator: vi.fn(),
    typingHandler: null as ((signal: TypingSignal) => void) | null
  }
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('./useEvent.svelte', () => ({
  useTypingEvent: (handler: (signal: TypingSignal) => void) => {
    mocks.typingHandler = handler;
  }
}));

let profiles: UserStore;
const signal: TypingSignal = { roomId: 'room', threadRootEventId: null, userId: 'late' };
const defaultConfig = { roomId: 'room', threadRootEventId: null, currentUserId: 'viewer' };

/** Mount a real owner so timer destruction and derived rendering run in Svelte. */
function mount(getConfig: Parameters<typeof createTypingIndicator>[0] = () => defaultConfig) {
  const screen = render(UseTypingIndicatorHarness, { getConfig });
  return { indicator: screen.component.indicator, screen };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-01-01T12:00:00Z'));
  mocks.batchGetUsers.mockReset().mockResolvedValue([]);
  mocks.refreshTypingIndicator.mockReset().mockResolvedValue(undefined);
  profiles = createTestServerScope({ api: mocks }).scope.store.projection.users;
  mocks.typingHandler = null;
});

afterEach(() => vi.useRealTimers());

describe('createTypingIndicator', () => {
  it('reads an unknown typer once per burst and retries after expiry or a message', async () => {
    const { indicator, screen } = mount();
    mocks.batchGetUsers.mockRejectedValueOnce(new Error('offline'));
    mocks.typingHandler?.(signal);
    mocks.typingHandler?.(signal);
    expect(mocks.batchGetUsers).toHaveBeenCalledExactlyOnceWith(['late']);
    await vi.advanceTimersByTimeAsync(TYPING_TIMEOUT_MS);
    expect(indicator.userIds).toEqual([]);
    mocks.typingHandler?.(signal);
    expect(mocks.batchGetUsers).toHaveBeenCalledTimes(2);
    indicator.removeTypingUser('late');
    mocks.typingHandler?.(signal);
    expect(mocks.batchGetUsers).toHaveBeenCalledTimes(3);
    await screen.unmount();
  });

  it('ignores other conversations and the viewer, and does not hydrate known or deleted users', async () => {
    profiles.delete('removed');
    profiles.set('known', new DirectoryMember({ user: { id: 'known' } }));
    const { indicator, screen } = mount();
    mocks.typingHandler?.({ ...signal, roomId: 'other' });
    mocks.typingHandler?.({ ...signal, threadRootEventId: 'thread' });
    mocks.typingHandler?.({ ...signal, userId: 'viewer' });
    expect(indicator.userIds).toEqual([]);
    mocks.typingHandler?.({ ...signal, userId: 'removed' });
    mocks.typingHandler?.({ ...signal, userId: 'known' });
    expect(mocks.batchGetUsers).not.toHaveBeenCalled();
    await screen.unmount();
  });

  it('updates rendered typers without discarding bursts when the viewer resolves', async () => {
    let currentUserId = $state<string | null>('viewer');
    const { indicator, screen } = mount(() => ({ ...defaultConfig, currentUserId }));
    mocks.typingHandler?.(signal);
    flushSync();
    expect(screen.container.textContent).toBe('late');
    currentUserId = 'late';
    flushSync();
    expect(screen.container.textContent).toBe('');
    currentUserId = null;
    flushSync();
    expect(screen.container.textContent).toBe('');
    currentUserId = 'viewer';
    flushSync();
    expect(screen.container.textContent).toBe('late');
    indicator.removeTypingUser('late');
    flushSync();
    expect(screen.container.textContent).toBe('');
    await screen.unmount();
  });

  it.each([
    { roomId: 'other', threadRootEventId: null },
    { roomId: 'room', threadRootEventId: 'thread' }
  ])('switches immediately to $roomId/$threadRootEventId with a fresh throttle', async (target) => {
    let config = $state<{
      roomId: string;
      threadRootEventId: string | null;
      currentUserId: string;
    }>(defaultConfig);
    const { indicator, screen } = mount(() => config);
    mocks.typingHandler?.(signal);
    await indicator.sendTypingIndicator();
    config = { ...target, currentUserId: 'viewer' };
    // Deliberately do not flush effects between the navigation and input/event.
    expect(indicator.userIds).toEqual([]);
    await indicator.sendTypingIndicator();
    expect(mocks.refreshTypingIndicator).toHaveBeenLastCalledWith(
      target.roomId,
      target.threadRootEventId
    );
    expect(mocks.refreshTypingIndicator).toHaveBeenCalledTimes(2);
    mocks.typingHandler?.(signal);
    expect(indicator.userIds).toEqual([]);
    mocks.typingHandler?.({ ...signal, ...target });
    expect(indicator.userIds).toEqual(['late']);
    expect(mocks.batchGetUsers).toHaveBeenCalledTimes(2);
    config = { ...defaultConfig };
    expect(indicator.userIds).toEqual([]);
    await screen.unmount();
  });

  it('throttles sends within a conversation and resets after a message', async () => {
    const { indicator, screen } = mount();
    await indicator.sendTypingIndicator();
    await indicator.sendTypingIndicator();
    expect(mocks.refreshTypingIndicator).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2000);
    await indicator.sendTypingIndicator();
    expect(mocks.refreshTypingIndicator).toHaveBeenCalledTimes(2);
    indicator.resetDebounce();
    await indicator.sendTypingIndicator();
    expect(mocks.refreshTypingIndicator).toHaveBeenCalledTimes(3);
    await screen.unmount();
  });

  it('releases the expiry timer when its component is destroyed', async () => {
    const timersBefore = vi.getTimerCount();
    const { screen } = mount();
    expect(vi.getTimerCount()).toBe(timersBefore + 1);
    await screen.unmount();
    expect(vi.getTimerCount()).toBe(timersBefore);
  });
});
