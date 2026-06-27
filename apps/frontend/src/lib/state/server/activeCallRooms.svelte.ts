/**
 * Tracks which rooms have active voice calls and who's in each call.
 *
 * Uses the `activeCallRoomIds` GraphQL query (backed by the call-state projection)
 * as the source of truth. Real-time updates come from room events:
 * - CallParticipantJoinedEvent → add participant to the room
 * - CallParticipantLeftEvent → remove participant; delete room if empty
 * - CallEndedEvent → delete the room regardless of participant snapshot
 *
 * Also includes the local user's active call from VoiceCallState for instant feedback.
 */

import { SvelteMap } from 'svelte/reactivity';
import { graphql, useFragment } from '$lib/gql';
import { UserAvatarUserFragmentDoc, type UserAvatarUserFragment } from '$lib/gql/graphql';
import type { Client } from '@urql/svelte';
import type { VoiceCallState } from '$lib/state/server/voiceCall.svelte';

const ActiveCallRoomIdsQuery = graphql(`
	query GetActiveCallRoomIds {
		activeCallRoomIds
	}
`);

const CallParticipantsQuery = graphql(`
	query GetSidebarCallParticipants($roomId: ID!) {
		room(roomId: $roomId) {
			callParticipants {
				user {
					...UserAvatarUser
				}
				joinedAt
				callId
			}
		}
	}
`);

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
  #client: Client;
  #voiceCall: VoiceCallState;

  /** Map of room ID → server-observed active call snapshot. */
  private serverRooms = new SvelteMap<string, ActiveCallRoomSnapshot>();
  private roomVersions = new SvelteMap<string, number>();
  private pendingCallIds = new SvelteMap<string, string>();

  constructor(client: Client, voiceCall: VoiceCallState) {
    this.#client = client;
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
      const liveParticipant = this.#voiceCall.participants.find((p) => p.identity === userId);
      if (liveParticipant) {
        return liveParticipant.isCameraEnabled && liveParticipant.videoTrack ? 'video' : 'voice';
      }
    }

    const serverParticipant = this.serverRooms
      .get(roomId)
      ?.participants.some((p) => p.userId === userId);
    return serverParticipant ? 'voice' : null;
  }

  /**
   * Load active call room IDs and their participants from the server.
   * Should be called when entering the chat (alongside room list loading).
   */
  async load(): Promise<void> {
    const result = await this.#client.query(ActiveCallRoomIdsQuery, {}).toPromise();
    const roomIds = result.data?.activeCallRoomIds ?? [];

    // Remove rooms that are no longer active
    for (const id of this.serverRooms.keys()) {
      if (!roomIds.includes(id)) {
        this.serverRooms.delete(id);
      }
    }

    // Fetch participants for each active room in parallel
    await Promise.all(roomIds.map((roomId: string) => this.loadRoomParticipants(roomId)));
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
    const participantResult = await this.#client
      .query(CallParticipantsQuery, { roomId })
      .toPromise();

    if (expectedVersion !== undefined && this.roomVersions.get(roomId) !== expectedVersion) {
      return;
    }

    const participants = participantResult.data?.room?.callParticipants;
    if (participants) {
      const callId = participants[0]?.callId ?? fallbackCallId;
      if (fallbackCallId !== null && callId !== null && callId !== fallbackCallId) return;

      this.serverRooms.set(roomId, {
        callId,
        participants: participants.map((p) => {
          const user = useFragment(UserAvatarUserFragmentDoc, p.user);
          return {
            userId: user.id,
            displayName: user.displayName,
            login: user.login,
            avatarUrl: user.avatarUrl ?? null
          };
        })
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
  async handleJoin(
    roomId: string,
    callId: string,
    actor: UserAvatarUserFragment | null
  ): Promise<void> {
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
