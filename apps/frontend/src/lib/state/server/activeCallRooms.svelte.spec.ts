import { describe, expect, it, vi } from 'vitest';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

describe('ActiveCallRoomsState', () => {
  it('removes a failed local participant without hiding other active participants', () => {
    const state = new ActiveCallRoomsState(
      { query: vi.fn() } as never,
      { connected: false, roomId: null } as never
    );

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
    const state = new ActiveCallRoomsState(
      { query: vi.fn() } as never,
      { connected: false, roomId: null, participants: [] } as never
    );

    state.handleJoin('R1', 'call-1', {
      id: 'U1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: null
    } as never);

    expect(state.getParticipantCallPresence('R1', 'U1')).toBe('voice');
    expect(state.getParticipantCallPresence('R1', 'U2')).toBeNull();
  });

  it('reloads participants when a protobuf call join has no hydrated actor', async () => {
    const query = vi.fn().mockReturnValueOnce({
      toPromise: vi.fn(async () => ({
        data: {
          room: {
            callParticipants: [
              {
                callId: 'call-1',
                joinedAt: '2026-01-01T00:00:00Z',
                user: {
                  id: 'U1',
                  displayName: 'Alice',
                  login: 'alice',
                  avatarUrl: null
                }
              }
            ]
          }
        }
      }))
    });
    const state = new ActiveCallRoomsState(
      { query } as never,
      { connected: false, roomId: null, participants: [] } as never
    );

    await state.handleJoin('R1', 'call-1', null);

    expect(query).toHaveBeenCalledTimes(1);
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
      {
        query: vi.fn(() => ({
          toPromise: vi.fn(async () => ({
            data: {
              room: {
                callParticipants: [
                  {
                    callId: 'call-1',
                    joinedAt: '2026-01-01T00:00:00Z',
                    user: {
                      id: 'U1',
                      displayName: 'Alice',
                      login: 'alice',
                      avatarUrl: null
                    }
                  }
                ]
              }
            }
          }))
        }))
      } as never,
      { connected: false, roomId: null, participants: [] } as never
    );

    await state.handleJoin('R1', 'call-1', null);
    state.handleLeave('R1', 'call-1', 'U2');

    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toHaveLength(1);
  });

  it('does not resurrect an ended call from a late actor-less join reload', async () => {
    const reload = deferred<{
      data: {
        room: {
          callParticipants: Array<{
            callId: string;
            joinedAt: string;
            user: { id: string; displayName: string; login: string; avatarUrl: string | null };
          }>;
        };
      };
    }>();
    const state = new ActiveCallRoomsState(
      {
        query: vi.fn(() => ({
          toPromise: vi.fn(() => reload.promise)
        }))
      } as never,
      { connected: false, roomId: null, participants: [] } as never
    );

    const join = state.handleJoin('R1', 'call-1', null);
    state.handleEnd('R1', 'call-1');
    reload.resolve({
      data: {
        room: {
          callParticipants: [
            {
              callId: 'call-1',
              joinedAt: '2026-01-01T00:00:00Z',
              user: {
                id: 'U1',
                displayName: 'Alice',
                login: 'alice',
                avatarUrl: null
              }
            }
          ]
        }
      }
    });
    await join;

    expect(state.has('R1')).toBe(false);
    expect(state.getParticipants('R1')).toEqual([]);
  });

  it('reports active LiveKit camera participants as video participants', () => {
    const state = new ActiveCallRoomsState(
      { query: vi.fn() } as never,
      {
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
      } as never
    );

    expect(state.getParticipantCallPresence('R1', 'U1')).toBe('video');
    expect(state.getParticipantCallPresence('R1', 'U2')).toBe('voice');
    expect(state.getParticipantCallPresence('R2', 'U1')).toBeNull();
  });

  it('clears a room when its call ends', () => {
    const state = new ActiveCallRoomsState(
      { query: vi.fn() } as never,
      { connected: false, roomId: null } as never
    );

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
  });

  it('clears a room with an unknown call id when a call end event arrives', async () => {
    const state = new ActiveCallRoomsState(
      {
        query: vi
          .fn()
          .mockReturnValueOnce({
            toPromise: vi.fn(async () => ({ data: { activeCallRoomIds: ['R1'] } }))
          })
          .mockReturnValueOnce({
            toPromise: vi.fn(async () => ({
              data: { room: { callParticipants: [] } }
            }))
          })
      } as never,
      { connected: false, roomId: null } as never
    );

    await state.load();

    expect(state.has('R1')).toBe(true);

    state.handleEnd('R1', 'call-1');

    expect(state.has('R1')).toBe(false);
    expect(state.getParticipants('R1')).toEqual([]);
  });

  it('ignores stale leave and end events from an older call', () => {
    const state = new ActiveCallRoomsState(
      { query: vi.fn() } as never,
      { connected: false, roomId: null } as never
    );

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
