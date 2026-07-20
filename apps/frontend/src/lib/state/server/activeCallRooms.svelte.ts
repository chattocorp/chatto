/**
 * Tracks which rooms have active voice calls and who's in each call.
 *
 * Canonical server state arrives through server-scoped active-call projection
 * replacements. The local VoiceCallState is overlaid for instant feedback.
 *
 * Also includes the local user's active call from VoiceCallState for instant feedback.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import type { VoiceCallState } from '$lib/state/server/voiceCall.svelte';

/** Participant info for display in the room list sidebar. */
export type CallRoomParticipant = {
  userId: string;
  displayName: string;
  login: string;
  avatarUrl: string | null;
};

export type CallPresenceKind = 'voice' | 'video';

type ActiveCallRoomSnapshot = {
  callId: string | null;
  participants: CallRoomParticipant[];
};

export class ActiveCallRoomsState {
  #voiceCall: VoiceCallState;

  /** Map of room ID → server-observed active call snapshot. */
  private serverRooms = new SvelteMap<string, ActiveCallRoomSnapshot>();
  constructor(voiceCall: VoiceCallState) {
    this.#voiceCall = voiceCall;
  }

  /**
   * Whether a room has an active call.
   * Checks both server state and local user's call state.
   */
  has(roomId: string): boolean {
    if (this.#voiceCall.connected && this.#voiceCall.roomId === roomId) {
      return true;
    }
    return this.serverRooms.has(roomId);
  }

  /**
   * Get participants for a room's active call.
   */
  getParticipants(roomId: string): CallRoomParticipant[] {
    return this.serverRooms.get(roomId)?.participants ?? [];
  }

  /** Return the projected call ID for transition reconciliation. */
  getCallId(roomId: string): string | null {
    return this.serverRooms.get(roomId)?.callId ?? null;
  }

  /** Locate a projected participant before applying the next replacement. */
  findParticipantCall(userId: string): { roomId: string; callId: string | null } | null {
    for (const [roomId, snapshot] of this.serverRooms) {
      if (snapshot.participants.some((participant) => participant.userId === userId)) {
        return { roomId, callId: snapshot.callId };
      }
    }
    return null;
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

    const serverParticipant = this.serverRooms
      .get(roomId)
      ?.participants.some((p) => p.userId === userId);
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

    for (const snapshot of this.serverRooms.values()) {
      if (snapshot.participants.some((p) => p.userId === userId)) return 'voice';
    }

    return null;
  }

  /** Replace server-observed calls from the canonical realtime projection. */
  replaceProjection(calls: readonly ActiveCall[]): void {
    const activeRoomIds = new SvelteSet(
      calls.flatMap((call) => (call.room?.id ? [call.room.id] : []))
    );
    for (const roomId of this.serverRooms.keys()) {
      if (!activeRoomIds.has(roomId)) this.serverRooms.delete(roomId);
    }
    for (const call of calls) {
      const roomId = call.room?.id;
      if (!roomId) continue;
      this.serverRooms.set(roomId, {
        callId: call.callId || null,
        participants: call.participants.flatMap((participant) => {
          const user = participant.user;
          if (!user?.id) return [];
          return [
            {
              userId: user.id,
              displayName: user.displayName,
              login: user.login,
              avatarUrl: user.avatarUrl ?? null
            }
          ];
        })
      });
    }
  }

  /** Immediately discard one room at a local authorization boundary. */
  clearRoom(roomId: string): void {
    this.serverRooms.delete(roomId);
  }

  /** Remove copied participant profile data for a deleted account. */
  scrubUser(userId: string): void {
    for (const [roomId, snapshot] of this.serverRooms) {
      const participants = snapshot.participants.filter(
        (participant) => participant.userId !== userId
      );
      if (participants.length === snapshot.participants.length) continue;
      // Account removal scrubs copied profile data; it does not imply that the
      // call resource ended. A later active-calls replacement owns that state.
      this.serverRooms.set(roomId, { callId: snapshot.callId, participants });
    }
  }

  private liveParticipantCallPresence(userId: string): CallPresenceKind | null {
    if (!this.#voiceCall.connected) return null;

    const liveParticipant = this.#voiceCall.participants.find((p) => p.identity === userId);
    if (!liveParticipant) return null;

    return liveParticipant.isCameraEnabled && liveParticipant.videoTrack ? 'video' : 'voice';
  }

  /** Optimistically remove the local user after a failed/aborted join. */
  handleLeave(roomId: string, callId: string | null, actorId: string | null): void {
    if (!actorId) return;

    const snapshot = this.serverRooms.get(roomId);
    if (!snapshot || (callId !== null && snapshot.callId !== callId)) return;

    if (!snapshot.participants.some((p) => p.userId === actorId)) return;

    const updated = snapshot.participants.filter((p) => p.userId !== actorId);
    if (updated.length > 0) {
      this.serverRooms.set(roomId, { callId: snapshot.callId, participants: updated });
    } else {
      this.serverRooms.delete(roomId);
    }
  }

  /**
   * Clear state.
   */
  clear(): void {
    this.serverRooms.clear();
  }
}
