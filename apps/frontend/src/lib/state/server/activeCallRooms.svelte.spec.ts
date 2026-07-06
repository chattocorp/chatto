import { describe, expect, it, vi } from 'vitest';
import type { VoiceCallAPI, VoiceCallParticipant } from '$lib/api-client/voiceCalls';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';

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
    listActiveCalls: vi.fn().mockResolvedValue([]),
    getActiveCall: vi.fn().mockResolvedValue(null),
    batchGetActiveCalls: vi.fn().mockResolvedValue([]),
    listCallParticipants: vi.fn().mockResolvedValue([]),
    joinCall: vi.fn().mockResolvedValue(true),
    getCallToken: vi.fn().mockResolvedValue(null),
    leaveCall: vi.fn().mockResolvedValue(true),
    ...overrides
  };
}

describe('ActiveCallRoomsState', () => {
  it('removes a failed local participant without hiding other active participants', () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: false,
      roomId: null
    } as never);

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

    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toEqual([
      {
        userId: 'U2',
        displayName: 'Bob',
        login: 'bob',
        avatarUrl: null
      }
    ]);
  });

  it('reports backend-observed participants as voice call participants', () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: false,
      roomId: null,
      participants: []
    } as never);

    state.handleJoin('R1', 'call-1', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    expect(state.getParticipantCallPresence('R1', 'U1')).toBe('voice');
    expect(state.getParticipantCallPresence('R1', 'U2')).toBeNull();
  });

  it('reports backend-observed participants in any active room on the server', async () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: false,
      roomId: null,
      participants: []
    } as never);

    await state.handleJoin('R2', 'call-2', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    expect(state.getParticipantCallPresence('R1', 'U1')).toBeNull();
    expect(state.getParticipantCallPresenceInAnyRoom('U1')).toBe('voice');
    expect(state.getParticipantCallPresenceInAnyRoom('U2')).toBeNull();
  });

  it('reloads participants when a protobuf call join has no hydrated actor', async () => {
    const listCallParticipants = vi.fn().mockResolvedValueOnce([participant('U1')]);
    const state = new ActiveCallRoomsState(makeVoiceCallAPI({ listCallParticipants }), {
      connected: false,
      roomId: null,
      participants: []
    } as never);

    await state.handleJoin('R1', 'call-1', null);

    expect(listCallParticipants).toHaveBeenCalledTimes(1);
    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toEqual([
      {
        userId: 'U1',
        displayName: 'Alice',
        login: 'alice',
        avatarUrl: null
      }
    ]);
  });

  it('keeps actor-less active rooms on unrelated leave events', async () => {
    const state = new ActiveCallRoomsState(
      makeVoiceCallAPI({ listCallParticipants: vi.fn().mockResolvedValue([participant('U1')]) }),
      { connected: false, roomId: null, participants: [] } as never
    );

    await state.handleJoin('R1', 'call-1', null);
    state.handleLeave('R1', 'call-1', 'U2');

    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toHaveLength(1);
  });

  it('does not resurrect an ended call from a late actor-less join reload', async () => {
    const reload = deferred<VoiceCallParticipant[]>();
    const state = new ActiveCallRoomsState(
      makeVoiceCallAPI({ listCallParticipants: vi.fn(() => reload.promise) }),
      { connected: false, roomId: null, participants: [] } as never
    );

    const join = state.handleJoin('R1', 'call-1', null);
    state.handleEnd('R1', 'call-1');
    reload.resolve([participant('U1')]);
    await join;

    expect(state.has('R1')).toBe(false);
    expect(state.getParticipants('R1')).toEqual([]);
  });

  it('reports active LiveKit camera participants as video participants', () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: true,
      roomId: 'R1',
      participants: [
        {
          identity: 'U1',
          isCameraEnabled: true,
          videoTrack: {}
        },
        {
          identity: 'U2',
          isCameraEnabled: false,
          videoTrack: null
        }
      ]
    } as never);

    expect(state.getParticipantCallPresence('R1', 'U1')).toBe('video');
    expect(state.getParticipantCallPresence('R1', 'U2')).toBe('voice');
    expect(state.getParticipantCallPresence('R2', 'U1')).toBeNull();
  });

  it('upgrades server-wide local LiveKit camera participants to video', () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: true,
      roomId: 'R1',
      participants: [
        {
          identity: 'U1',
          isCameraEnabled: true,
          videoTrack: {}
        }
      ]
    } as never);

    expect(state.getParticipantCallPresenceInAnyRoom('U1')).toBe('video');
  });

  it('clears a room when its call ends', () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: false,
      roomId: null
    } as never);

    state.handleJoin('R1', 'call-1', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toHaveLength(1);

    state.handleEnd('R1', 'call-1');

    expect(state.has('R1')).toBe(false);
    expect(state.getParticipants('R1')).toEqual([]);
    expect(state.getParticipantCallPresenceInAnyRoom('U1')).toBeNull();
  });

  it('clears server-wide participant presence after the last participant leaves', async () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: false,
      roomId: null,
      participants: []
    } as never);

    await state.handleJoin('R1', 'call-1', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    expect(state.getParticipantCallPresenceInAnyRoom('U1')).toBe('voice');

    state.handleLeave('R1', 'call-1', 'U1');

    expect(state.getParticipantCallPresenceInAnyRoom('U1')).toBeNull();
  });

  it('clears a list-loaded room when its call end event arrives', async () => {
    const state = new ActiveCallRoomsState(
      makeVoiceCallAPI({
        listActiveCalls: vi.fn().mockResolvedValue([{ roomId: 'R1', callId: 'call-unknown', participants: [] }])
      }),
      { connected: false, roomId: null } as never
    );

    await state.load();

    expect(state.has('R1')).toBe(true);

    state.handleEnd('R1', 'call-unknown');

    expect(state.has('R1')).toBe(false);
    expect(state.getParticipants('R1')).toEqual([]);
  });

  it('loads initial active calls through list snapshots', async () => {
    const listActiveCalls = vi.fn().mockResolvedValue([
      { roomId: 'R1', callId: 'call-1', participants: [participant('U1')] }
    ]);
    const listCallParticipants = vi.fn().mockResolvedValue([participant('U2')]);
    const state = new ActiveCallRoomsState(
      makeVoiceCallAPI({ listActiveCalls, listCallParticipants }),
      { connected: false, roomId: null, participants: [] } as never
    );

    await state.load();

    expect(listActiveCalls).toHaveBeenCalledTimes(1);
    expect(listCallParticipants).not.toHaveBeenCalled();
    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toEqual([
      {
        userId: 'U1',
        displayName: 'Alice',
        login: 'alice',
        avatarUrl: null
      }
    ]);
  });

  it('ignores stale leave and end events from an older call', () => {
    const state = new ActiveCallRoomsState(makeVoiceCallAPI(), {
      connected: false,
      roomId: null
    } as never);

    state.handleJoin('R1', 'call-2', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    state.handleLeave('R1', 'call-1', 'U1');
    state.handleEnd('R1', 'call-1');

    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toEqual([
      {
        userId: 'U1',
        displayName: 'Alice',
        login: 'alice',
        avatarUrl: null
      }
    ]);
  });
});
