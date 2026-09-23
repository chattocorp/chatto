import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { UserStore } from '$lib/state/server/users.svelte';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { TYPING_TIMEOUT_MS, createTypingIndicator } from './useTypingIndicator.svelte';

type TypingSignal = { roomId: string; threadRootEventId: string | null; userId: string };

const { mocks } = vi.hoisted(() => ({
  mocks: {
    useServerScope: vi.fn(),
    batchGetUsers: vi.fn<(ids: string[]) => Promise<unknown[]>>(),
    typingHandler: null as ((signal: TypingSignal) => void) | null
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: mocks.useServerScope
}));

vi.mock('./useEvent.svelte', () => ({
  useTypingEvent: (handler: (signal: TypingSignal) => void) => { mocks.typingHandler = handler; }
}));

const profiles = new UserStore();
const signal: TypingSignal = { roomId: 'room', threadRootEventId: null, userId: 'late' };

beforeEach(() => {
  vi.useFakeTimers();
  profiles.clear();
  mocks.batchGetUsers.mockReset().mockResolvedValue([]);
  mocks.useServerScope.mockReturnValue({
    store: { projection: { users: profiles } },
    connection: { getAPI: () => ({ batchGetUsers: mocks.batchGetUsers }) }
  });
  mocks.typingHandler = null;
});

afterEach(() => {
  vi.useRealTimers();
});

describe('createTypingIndicator profile hydration', () => {
  it('reads an unknown typer once per burst and retries on a later burst', async () => {
    const dispose = $effect.root(() => {
      createTypingIndicator(() => ({
        roomId: 'room', threadRootEventId: null, currentUserId: 'viewer'
      }));
    });
    flushSync();

    mocks.typingHandler?.(signal);
    mocks.typingHandler?.(signal);
    expect(mocks.batchGetUsers).toHaveBeenCalledExactlyOnceWith(['late']);

    await vi.advanceTimersByTimeAsync(TYPING_TIMEOUT_MS);
    mocks.typingHandler?.(signal);
    expect(mocks.batchGetUsers).toHaveBeenCalledTimes(2);

    dispose();
  });

  it('does not read a known or deleted typer', () => {
    profiles.delete('removed');
    profiles.set('known', new DirectoryMember({ user: { id: 'known' } }));
    const dispose = $effect.root(() => {
      createTypingIndicator(() => ({
        roomId: 'room', threadRootEventId: null, currentUserId: 'viewer'
      }));
    });
    flushSync();

    mocks.typingHandler?.({ ...signal, userId: 'removed' });
    mocks.typingHandler?.({ ...signal, userId: 'known' });
    mocks.typingHandler?.({ ...signal, userId: 'viewer' });
    expect(mocks.batchGetUsers).not.toHaveBeenCalled();

    dispose();
  });

  it('permits a new profile read after a message ends the typing burst', () => {
    let indicator!: ReturnType<typeof createTypingIndicator>;
    const dispose = $effect.root(() => {
      indicator = createTypingIndicator(() => ({
        roomId: 'room', threadRootEventId: null, currentUserId: 'viewer'
      }));
    });
    flushSync();

    mocks.typingHandler?.(signal);
    indicator.removeTypingUser('late');
    mocks.typingHandler?.(signal);
    expect(mocks.batchGetUsers).toHaveBeenCalledTimes(2);

    dispose();
  });
});
