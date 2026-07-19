import { describe, expect, it } from 'vitest';
import { ActiveCall, CallParticipant } from '@chatto/api-types/api/v1/voice_calls_pb';
import { RoomSummary } from '@chatto/api-types/api/v1/rooms_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';

function call(roomId: string, callId: string, userIds: string[]): ActiveCall {
  return new ActiveCall({
    room: new RoomSummary({ id: roomId }),
    callId,
    participants: userIds.map(
      (userId) =>
        new CallParticipant({
          user: new User({
            id: userId,
            login: userId.toLowerCase(),
            displayName: userId
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

describe('ActiveCallRoomsState', () => {
  it('authoritatively replaces calls and participants from the projection', () => {
    const state = new ActiveCallRoomsState(voiceCall());

    state.replaceProjection([call('R1', 'call-1', ['U1', 'U2'])]);

    expect(state.has('R1')).toBe(true);
    expect(state.getCallId('R1')).toBe('call-1');
    expect(state.getParticipants('R1').map(({ userId }) => userId)).toEqual(['U1', 'U2']);
    expect(state.findParticipantCall('U2')).toEqual({ roomId: 'R1', callId: 'call-1' });

    state.replaceProjection([call('R2', 'call-2', ['U3'])]);

    expect(state.has('R1')).toBe(false);
    expect(state.getParticipants('R1')).toEqual([]);
    expect(state.findParticipantCall('U2')).toBeNull();
    expect(state.findParticipantCall('U3')).toEqual({ roomId: 'R2', callId: 'call-2' });
  });

  it('optimistically removes only the failed local participant', () => {
    const state = new ActiveCallRoomsState(voiceCall());
    state.replaceProjection([call('R1', 'call-1', ['U1', 'U2'])]);

    state.handleLeave('R1', null, 'U1');

    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1').map(({ userId }) => userId)).toEqual(['U2']);
  });

  it('scrubs deleted participants from projected calls', () => {
    const state = new ActiveCallRoomsState(voiceCall());
    state.replaceProjection([call('R1', 'call-1', ['U1', 'U2'])]);

    state.scrubUser('U1');

    expect(state.getParticipants('R1').map(({ userId }) => userId)).toEqual(['U2']);
    expect(state.findParticipantCall('U1')).toBeNull();
  });

  it('clears only the room whose access was revoked', () => {
    const state = new ActiveCallRoomsState(voiceCall());
    state.replaceProjection([call('R1', 'call-1', ['U1']), call('R2', 'call-2', ['U2'])]);

    state.clearRoom('R1');

    expect(state.has('R1')).toBe(false);
    expect(state.has('R2')).toBe(true);
  });

  it('does not infer that a call ended when its last deleted participant is scrubbed', () => {
    const state = new ActiveCallRoomsState(voiceCall());
    state.replaceProjection([call('R1', 'call-1', ['U1'])]);

    state.scrubUser('U1');

    expect(state.has('R1')).toBe(true);
    expect(state.getParticipants('R1')).toEqual([]);
  });

  it('reports projected participants as voice and LiveKit camera participants as video', () => {
    const state = new ActiveCallRoomsState(
      voiceCall({
        connected: true,
        roomId: 'R1',
        participants: [
          { identity: 'U1', isCameraEnabled: true, videoTrack: {} },
          { identity: 'U2', isCameraEnabled: false, videoTrack: null }
        ]
      })
    );
    state.replaceProjection([call('R1', 'call-1', ['U1', 'U2', 'U3'])]);

    expect(state.getParticipantCallPresence('R1', 'U1')).toBe('video');
    expect(state.getParticipantCallPresence('R1', 'U2')).toBe('voice');
    expect(state.getParticipantCallPresence('R1', 'U3')).toBe('voice');
    expect(state.getParticipantCallPresenceInAnyRoom('U1')).toBe('video');
  });

  it('clears all projected state', () => {
    const state = new ActiveCallRoomsState(voiceCall());
    state.replaceProjection([call('R1', 'call-1', ['U1'])]);

    state.clear();

    expect(state.has('R1')).toBe(false);
    expect(state.findParticipantCall('U1')).toBeNull();
  });
});
