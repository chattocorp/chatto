/**
 * Tracks participants in an active voice call for a specific room.
 *
 * Used by the observer mode of VoiceCallPanel to show who's in a call
 * to room members who haven't joined yet.
 *
 * Data sources:
 * - Initial load: Connect voice-call API (from the call-state projection)
 * - Real-time updates: Optimistic adds/removes from CallParticipantJoined/Left events
 */

import type { VoiceCallAPI } from '$lib/api/voiceCalls';

/** Participant info stored in observer mode. */
export type ObserverParticipant = {
  userId: string;
  displayName: string;
  login: string;
  avatarUrl: string | null;
};

type QueryCallParticipant = NonNullable<
  Awaited<ReturnType<VoiceCallAPI['listCallParticipants']>>
>[number];

type CallActor = {
  id: string;
  displayName: string;
  login: string;
  avatarUrl?: string | null;
};

export class CallParticipantsState {
  #api: VoiceCallAPI;

  /** Current participants visible to observers. */
  participants = $state<ObserverParticipant[]>([]);

  /** The room these participants are for. */
  private currentRoomId: string | null = null;
  private currentCallId: string | null = null;
  private version = 0;

  constructor(api: VoiceCallAPI) {
    this.#api = api;
  }

  /**
   * Load participants from the server for a specific room.
   * Called when entering a room that has an active call.
   */
  private bumpVersion(): number {
    this.version += 1;
    return this.version;
  }

  private async fetchParticipants(roomId: string): Promise<QueryCallParticipant[] | null> {
    return await this.#api.listCallParticipants(roomId);
  }

  async load(roomId: string): Promise<void> {
    this.currentRoomId = roomId;
    const version = this.bumpVersion();

    const participants = await this.fetchParticipants(roomId);
    if (version !== this.version || this.currentRoomId !== roomId) return;

    if (participants) {
      this.currentCallId = participants[0]?.callId ?? null;
      this.participants = participants.map(toObserverParticipant);
    }
  }

  /**
   * Optimistically add a participant from a CallParticipantJoinedEvent.
   * Uses the actor data from the Event envelope.
   */
  async handleJoin(roomId: string, callId: string, actor: CallActor | null): Promise<void> {
    if (roomId !== this.currentRoomId) return;
    if (this.currentCallId && this.currentCallId !== callId) return;
    if (!actor) {
      const version = this.bumpVersion();
      const participants = await this.fetchParticipants(roomId);
      if (version !== this.version || this.currentRoomId !== roomId) return;
      if (participants) {
        const loadedCallId = participants[0]?.callId ?? callId;
        if (loadedCallId !== callId) return;
        this.currentCallId = loadedCallId;
        this.participants = participants.map(toObserverParticipant);
      }
      return;
    }

    this.bumpVersion();
    this.currentCallId = callId;

    // Avoid duplicates
    if (this.participants.some((p) => p.userId === actor.id)) return;

    this.participants = [
      ...this.participants,
      {
        userId: actor.id,
        displayName: actor.displayName,
        login: actor.login,
        avatarUrl: actor.avatarUrl ?? null
      }
    ];
  }

  /**
   * Optimistically remove a participant from a CallParticipantLeftEvent.
   */
  handleLeave(roomId: string, callId: string | null, actorId: string | null): void {
    if (roomId !== this.currentRoomId) return;
    if (callId !== null && this.currentCallId !== callId) return;
    if (!actorId) return;

    this.bumpVersion();
    this.participants = this.participants.filter((p) => p.userId !== actorId);
  }

  /** Clear observer participants when the room's call ends. */
  handleEnd(roomId: string, callId: string): void {
    if (roomId !== this.currentRoomId) return;
    if (this.currentCallId !== null && this.currentCallId !== callId) return;
    this.clear();
  }

  /** Clear state (e.g., when leaving a room or call ends). */
  clear(): void {
    this.bumpVersion();
    this.participants = [];
    this.currentRoomId = null;
    this.currentCallId = null;
  }
}

function toObserverParticipant(p: QueryCallParticipant): ObserverParticipant {
  return {
    userId: p.user.id,
    displayName: p.user.displayName,
    login: p.user.login,
    avatarUrl: p.user.avatarUrl ?? null
  };
}
