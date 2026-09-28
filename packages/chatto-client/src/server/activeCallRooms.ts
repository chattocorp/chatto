/**
 * Tracks which rooms have active voice calls and who's in each call.
 *
 * The server's active calls come from the realtime projection. The local
 * VoiceCallState is overlaid for instant feedback about the viewer's own call.
 */

import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import type { VoiceCallState } from '$lib/state/server/voiceCall.svelte';

/** Participant info for display in the room list sidebar. */
export type CallRoomParticipant = {
  userId: string;
  displayName: string;
  login: string;
  avatarUrl: string | null;
  isBot: boolean;
};

export type CallPresenceKind = 'voice' | 'video';

type ActiveCallRoomSnapshot = {
  callId: string | null;
  participants: CallRoomParticipant[];
};

export class ActiveCallRoomsState {
  #voiceCall: VoiceCallState;
  #getCalls: () => readonly ActiveCall[];

  /** Room ID → server-observed active call, derived from the projection. */
  readonly #serverRooms: Readonly<Record<string, ActiveCallRoomSnapshot>> = $derived.by(() => {
    const rooms: Record<string, ActiveCallRoomSnapshot> = Object.create(null);
    for (const call of this.#getCalls()) {
      const roomId = call.room?.id;
      if (roomId) rooms[roomId] = snapshotFromCall(call);
    }
    return rooms;
  });

  constructor(voiceCall: VoiceCallState, getCalls: () => readonly ActiveCall[]) {
    this.#voiceCall = voiceCall;
    this.#getCalls = getCalls;
  }

  /**
   * Whether a room has an active call.
   * Checks both server state and local user's call state.
   */
  has(roomId: string): boolean {
    if (this.#voiceCall.connected && this.#voiceCall.roomId === roomId) {
      return true;
    }
    return roomId in this.#serverRooms;
  }

  /**
   * Get participants for a room's active call.
   */
  getParticipants(roomId: string): CallRoomParticipant[] {
    return this.#serverRooms[roomId]?.participants ?? [];
  }

  /** Return the projected call ID for transition reconciliation. */
  getCallId(roomId: string): string | null {
    return this.#serverRooms[roomId]?.callId ?? null;
  }

  /**
   * Return a user's call presence for a room.
   *
   * Backend-observed participants only tell us that someone is in the call,
   * so those render as voice. Once the local user has joined LiveKit, track
   * state lets us upgrade participants with an active camera track to video.
   */
  getParticipantCallPresence(roomId: string, userId: string): CallPresenceKind | null {
    if (this.#voiceCall.connected && this.#voiceCall.roomId === roomId) {
      const livePresence = this.liveParticipantCallPresence(userId);
      if (livePresence) return livePresence;
    }

    const serverParticipant = this.#serverRooms[roomId]?.participants.some(
      (p) => p.userId === userId
    );
    return serverParticipant ? 'voice' : null;
  }

  /**
   * Return a user's call presence in any active room on this server.
   *
   * Server snapshots only expose membership, so they render as voice. The
   * current LiveKit room can upgrade visible participants to video when a
   * camera track is active.
   */
  getParticipantCallPresenceInAnyRoom(userId: string): CallPresenceKind | null {
    const livePresence = this.liveParticipantCallPresence(userId);
    if (livePresence) return livePresence;

    for (const snapshot of Object.values(this.#serverRooms)) {
      if (snapshot.participants.some((p) => p.userId === userId)) return 'voice';
    }

    return null;
  }

  private liveParticipantCallPresence(userId: string): CallPresenceKind | null {
    if (!this.#voiceCall.connected) return null;

    const liveParticipant = this.#voiceCall.participants.find((p) => p.identity === userId);
    if (!liveParticipant) return null;

    return liveParticipant.isCameraEnabled && liveParticipant.videoTrack ? 'video' : 'voice';
  }
}

function snapshotFromCall(call: ActiveCall): ActiveCallRoomSnapshot {
  return {
    callId: call.callId || null,
    participants: call.participants.flatMap((participant) => {
      const user = participant.user;
      if (!user?.id) return [];
      return [
        {
          userId: user.id,
          displayName: user.displayName,
          login: user.login,
          avatarUrl: user.avatarUrl ?? null,
          isBot: !!user.bot
        }
      ];
    })
  };
}
