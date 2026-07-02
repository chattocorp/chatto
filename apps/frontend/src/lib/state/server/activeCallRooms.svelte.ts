/**
 * Tracks which rooms have active voice calls and who's in each call.
 *
 * Uses the Connect voice-call API (backed by the call-state projection)
 * as the source of truth. Real-time updates come from room events:
 * - CallParticipantJoinedEvent → add participant to the room
 * - CallParticipantLeftEvent → remove participant; delete room if empty
 * - CallEndedEvent → delete the room regardless of participant snapshot
 *
 * Also includes the local user's active call from VoiceCallState for instant feedback.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { VoiceCallState } from '$lib/state/server/voiceCall.svelte';
import type {
  ActiveVoiceCall,
  VoiceCallAPI,
  VoiceCallParticipant
} from '$lib/api-client/voiceCalls';

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

type CallActor = {
  id: string;
  displayName: string;
  login: string;
  avatarUrl?: string | null;
};

export class ActiveCallRoomsState {
  #api: VoiceCallAPI;
  #voiceCall: VoiceCallState;

  /** Map of room ID → server-observed active call snapshot. */
  private serverRooms = new SvelteMap<string, ActiveCallRoomSnapshot>();
  private roomVersions = new SvelteMap<string, number>();
  private pendingCallIds = new SvelteMap<string, string>();

  constructor(api: VoiceCallAPI, voiceCall: VoiceCallState) {
    this.#api = api;
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

  /**
   * Load active call snapshots from the server.
   * Should be called when entering the chat (alongside room list loading).
   */
  async load(): Promise<void> {
    const calls = await this.#api.listActiveCalls();
    const activeRoomIds = new SvelteSet(calls.map((call) => call.roomId));

    // Remove rooms that are no longer active
    for (const id of this.serverRooms.keys()) {
      if (!activeRoomIds.has(id)) {
        this.serverRooms.delete(id);
      }
    }

    for (const call of calls) {
      this.applyActiveCall(call);
    }
  }

  private applyActiveCall(call: ActiveVoiceCall): void {
    this.serverRooms.set(call.roomId, {
      callId: call.callId,
      participants: call.participants.map(toCallRoomParticipant)
    });
    if (this.pendingCallIds.get(call.roomId) === call.callId) {
      this.pendingCallIds.delete(call.roomId);
    }
  }

  private liveParticipantCallPresence(userId: string): CallPresenceKind | null {
    if (!this.#voiceCall.connected) return null;

    const liveParticipant = this.#voiceCall.participants.find((p) => p.identity === userId);
    if (!liveParticipant) return null;

    return liveParticipant.isCameraEnabled && liveParticipant.videoTrack ? 'video' : 'voice';
  }

  private bumpRoomVersion(roomId: string): number {
    const next = (this.roomVersions.get(roomId) ?? 0) + 1;
    this.roomVersions.set(roomId, next);
    return next;
  }

  private async loadRoomParticipants(
    roomId: string,
    fallbackCallId: string | null = null,
    expectedVersion?: number
  ) {
    const participants = await this.#api.listCallParticipants(roomId);

    if (expectedVersion !== undefined && this.roomVersions.get(roomId) !== expectedVersion) {
      return;
    }

    if (participants) {
      const callId = participants[0]?.callId ?? fallbackCallId;
      if (fallbackCallId !== null && callId !== null && callId !== fallbackCallId) return;

      this.serverRooms.set(roomId, {
        callId,
        participants: participants.map(toCallRoomParticipant)
      });
      if (this.pendingCallIds.get(roomId) === fallbackCallId) {
        this.pendingCallIds.delete(roomId);
      }
    } else if (!this.serverRooms.has(roomId)) {
      // Room is active but we couldn't fetch participants
      this.serverRooms.set(roomId, { callId: fallbackCallId, participants: [] });
    }
  }

  /**
   * Handle a CallParticipantJoinedEvent — add participant to the room.
   */
  async handleJoin(roomId: string, callId: string, actor: CallActor | null): Promise<void> {
    const existing = this.serverRooms.get(roomId);
    if (existing?.callId && existing.callId !== callId) return;

    const snapshot = existing ?? { callId, participants: [] };
    const participants = snapshot.participants;

    if (actor) {
      // Avoid duplicates
      if (participants.some((p) => p.userId === actor.id)) return;

      this.bumpRoomVersion(roomId);
      this.pendingCallIds.delete(roomId);
      this.serverRooms.set(roomId, {
        callId,
        participants: [
          ...participants,
          {
            userId: actor.id,
            displayName: actor.displayName,
            login: actor.login,
            avatarUrl: actor.avatarUrl ?? null
          }
        ]
      });
    } else {
      this.pendingCallIds.set(roomId, callId);
      const version = this.bumpRoomVersion(roomId);
      await this.loadRoomParticipants(roomId, callId, version);
    }
  }

  /**
   * Handle a CallParticipantLeftEvent — remove participant from the room.
   * Deletes the room entry if no participants remain.
   */
  handleLeave(roomId: string, callId: string | null, actorId: string | null): void {
    if (!actorId) return;

    const snapshot = this.serverRooms.get(roomId);
    if (!snapshot || (callId !== null && snapshot.callId !== callId)) return;

    if (!snapshot.participants.some((p) => p.userId === actorId)) return;

    const updated = snapshot.participants.filter((p) => p.userId !== actorId);
    this.bumpRoomVersion(roomId);
    if (updated.length > 0) {
      this.serverRooms.set(roomId, { callId: snapshot.callId, participants: updated });
    } else {
      this.serverRooms.delete(roomId);
      this.pendingCallIds.delete(roomId);
    }
  }

  /**
   * Handle a CallEndedEvent — clear the room's server-side call snapshot.
   */
  handleEnd(roomId: string, callId: string): void {
    const snapshot = this.serverRooms.get(roomId);
    if (!snapshot) {
      if (this.pendingCallIds.get(roomId) === callId) {
        this.bumpRoomVersion(roomId);
        this.pendingCallIds.delete(roomId);
      }
      return;
    }
    if (snapshot.callId !== null && snapshot.callId !== callId) return;
    this.bumpRoomVersion(roomId);
    this.serverRooms.delete(roomId);
    this.pendingCallIds.delete(roomId);
  }

  /**
   * Clear state.
   */
  clear(): void {
    this.serverRooms.clear();
    this.roomVersions.clear();
    this.pendingCallIds.clear();
  }
}

function toCallRoomParticipant(participant: VoiceCallParticipant): CallRoomParticipant {
  return {
    userId: participant.user.id,
    displayName: participant.user.displayName,
    login: participant.user.login,
    avatarUrl: participant.user.avatarUrl ?? null
  };
}
