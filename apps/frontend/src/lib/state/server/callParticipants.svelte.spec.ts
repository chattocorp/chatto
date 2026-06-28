import { describe, expect, it, vi } from 'vitest';
import type { VoiceCallAPI, VoiceCallParticipant } from '$lib/api/voiceCalls';
import { CallParticipantsState } from './callParticipants.svelte';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function participant(
  userId: string,
  displayName = 'Alice',
  login = 'alice',
  callId = 'call-1'
): VoiceCallParticipant {
  return {
    callId,
    joinedAt: '2026-01-01T00:00:00Z',
    user: {
      id: userId,
      displayName,
      login,
      deleted: false,
      avatarUrl: null
    }
  };
}

function makeVoiceCallAPI(overrides: Partial<VoiceCallAPI> = {}): VoiceCallAPI {
  return {
    listActiveCallRoomIds: vi.fn().mockResolvedValue([]),
    listCallParticipants: vi.fn().mockResolvedValue([]),
    joinCall: vi.fn().mockResolvedValue(true),
    getCallToken: vi.fn().mockResolvedValue(null),
    leaveCall: vi.fn().mockResolvedValue(true),
    ...overrides
  };
}

describe('CallParticipantsState', () => {
  it('removes a failed local participant from observer participants', async () => {
    const state = new CallParticipantsState(makeVoiceCallAPI());

    await state.load('R1');
    state.handleJoin('R1', 'call-1', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);
    state.handleJoin('R1', 'call-1', {
      id: 'U2',
      displayName: 'Bob',
      login: 'bob',
      avatarUrl: null
    } as never);

    state.handleLeave('R1', 'call-1', 'U1');

    expect(state.participants).toEqual([
      {
        userId: 'U2',
        displayName: 'Bob',
        login: 'bob',
        avatarUrl: null
      }
    ]);
  });

  it('reloads observer participants when a protobuf call join has no hydrated actor', async () => {
    const listCallParticipants = vi
      .fn()
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([participant('U1')]);
    const state = new CallParticipantsState(makeVoiceCallAPI({ listCallParticipants }));

    await state.load('R1');
    await state.handleJoin('R1', 'call-1', null);

    expect(listCallParticipants).toHaveBeenCalledTimes(2);
    expect(state.participants).toEqual([
      {
        userId: 'U1',
        displayName: 'Alice',
        login: 'alice',
        avatarUrl: null
      }
    ]);
  });

  it('does not resurrect observer participants from a late actor-less join reload', async () => {
    const reload = deferred<VoiceCallParticipant[]>();
    const listCallParticipants = vi
      .fn()
      .mockResolvedValueOnce([])
      .mockReturnValueOnce(reload.promise);
    const state = new CallParticipantsState(makeVoiceCallAPI({ listCallParticipants }));

    await state.load('R1');
    const join = state.handleJoin('R1', 'call-1', null);
    state.handleEnd('R1', 'call-1');
    reload.resolve([participant('U1')]);
    await join;

    expect(state.participants).toEqual([]);
  });

  it('clears observer participants when the current room call ends', async () => {
    const state = new CallParticipantsState(makeVoiceCallAPI());

    await state.load('R1');
    state.handleJoin('R1', 'call-1', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    expect(state.participants).toHaveLength(1);

    state.handleEnd('R1', 'call-1');

    expect(state.participants).toEqual([]);
  });

  it('clears observer state for an end event when the loaded snapshot had no call id', async () => {
    const state = new CallParticipantsState(makeVoiceCallAPI());

    await state.load('R1');
    state.handleEnd('R1', 'call-1');
    state.handleJoin('R1', 'call-1', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    expect(state.participants).toEqual([]);
  });

  it('ignores stale leave and end events from an older call', async () => {
    const state = new CallParticipantsState(makeVoiceCallAPI());

    await state.load('R1');
    state.handleJoin('R1', 'call-2', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    state.handleLeave('R1', 'call-1', 'U1');
    state.handleEnd('R1', 'call-1');

    expect(state.participants).toEqual([
      {
        userId: 'U1',
        displayName: 'Alice',
        login: 'alice',
        avatarUrl: null
      }
    ]);
  });
});
