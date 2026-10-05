import { signal } from '@chatto/client/reactivity';
import { describe, expect, it } from 'vitest';
import { ActiveCall, CallParticipant } from '@chatto/api-types/api/v1/voice_calls_pb';
import { RoomSummary } from '@chatto/api-types/api/v1/rooms_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { ActiveCallRoomsState } from './activeCallRooms';

function call(roomId: string, callId: string, userIds: string[], isBot = false): ActiveCall {
  return new ActiveCall({
    room: new RoomSummary({ id: roomId }),
    callId,
    participants: userIds.map(
      (userId) =>
        new CallParticipant({
          user: new User({
            id: userId,
            login: userId.toLowerCase(),
            displayName: userId,
            bot: isBot ? { ownerUserId: 'owner' } : undefined
          })
        })
    )
  });
}

function voiceCall(overrides: Record<string, unknown> = {}) {
  return {
    connected: false,
    roomId: null,
    participants: [],
    ...overrides
  } as never;
}

const calls = signal<ActiveCall[]>([]);

function activeCallRooms(voice = voiceCall()): ActiveCallRoomsState {
  calls.set([]);
  return new ActiveCallRoomsState(
    () => voice,
    () => calls.get()
  );
}

describe('ActiveCallRoomsState', () => {
  it('follows the projected calls and participants', () => {
    const state = activeCallRooms();

    calls.set([call('R1', 'call-1', ['U1', 'U2'])]);

    expect(state.has('R1')).toBe(true);
    expect(state.getCallId('R1')).toBe('call-1');
    expect(state.getParticipants('R1').map(({ userId }) => userId)).toEqual(['U1', 'U2']);

    calls.set([call('R2', 'call-2', ['U3'])]);

    expect(state.has('R1')).toBe(false);
    expect(state.getParticipants('R1')).toEqual([]);
    expect(state.getParticipants('R2').map(({ userId }) => userId)).toEqual(['U3']);

    calls.set([]);

    expect(state.has('R2')).toBe(false);
  });

  it('preserves bot identity from projected call participants', () => {
    const state = activeCallRooms();

    calls.set([call('R1', 'call-1', ['BOT1'], true)]);

    expect(state.getParticipants('R1')[0]?.isBot).toBe(true);
  });

  it('does not match room IDs against object prototype keys', () => {
    const state = activeCallRooms();

    expect(state.has('toString')).toBe(false);
    expect(state.getParticipants('constructor')).toEqual([]);
  });

  it('reports the viewer own call before the projection has it', () => {
    const state = activeCallRooms(voiceCall({ connected: true, roomId: 'R1' }));

    expect(state.has('R1')).toBe(true);
  });

  it('reports projected participants as voice and LiveKit camera participants as video', () => {
    const state = activeCallRooms(
      voiceCall({
        connected: true,
        roomId: 'R1',
        participants: [
          { identity: 'U1', isCameraEnabled: true, videoTrack: {} },
          { identity: 'U2', isCameraEnabled: false, videoTrack: null }
        ]
      })
    );
    calls.set([call('R1', 'call-1', ['U1', 'U2', 'U3'])]);

    expect(state.getParticipantCallPresence('R1', 'U1')).toBe('video');
    expect(state.getParticipantCallPresence('R1', 'U2')).toBe('voice');
    expect(state.getParticipantCallPresence('R1', 'U3')).toBe('voice');
    expect(state.getParticipantCallPresenceInAnyRoom('U1')).toBe('video');
  });
});
