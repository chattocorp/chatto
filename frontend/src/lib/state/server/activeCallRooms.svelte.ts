/**
 * Tracks which rooms have active voice calls and who's in each call.
 *
 * Uses the `activeCallRoomIds` GraphQL query (backed by LiveKit's ListRooms API)
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
		await Promise.all(
			roomIds.map(async (roomId: string) => {
				const participantResult = await this.#client
					.query(CallParticipantsQuery, { roomId })
					.toPromise();

				const participants = participantResult.data?.room?.callParticipants;
				if (participants) {
					this.serverRooms.set(
						roomId,
						{
							callId: participants[0]?.callId ?? null,
							participants: participants.map((p) => {
								const user = useFragment(UserAvatarUserFragmentDoc, p.user);
								return {
									userId: user.id,
									displayName: user.displayName,
									login: user.login,
									avatarUrl: user.avatarUrl ?? null
								};
							})
						}
					);
				} else if (!this.serverRooms.has(roomId)) {
					// Room is active but we couldn't fetch participants
					this.serverRooms.set(roomId, { callId: null, participants: [] });
				}
			})
		);
	}

	/**
	 * Handle a CallParticipantJoinedEvent — add participant to the room.
	 */
	handleJoin(roomId: string, callId: string, actor: UserAvatarUserFragment | null): void {
		const existing = this.serverRooms.get(roomId);
		if (existing?.callId && existing.callId !== callId) return;

		const snapshot = existing ?? { callId, participants: [] };
		const participants = snapshot.participants;

		if (actor) {
			// Avoid duplicates
			if (participants.some((p) => p.userId === actor.id)) return;

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
		} else if (!this.serverRooms.has(roomId)) {
			// No actor data but room is now active
			this.serverRooms.set(roomId, { callId, participants: [] });
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

		const updated = snapshot.participants.filter((p) => p.userId !== actorId);
		if (updated.length > 0) {
			this.serverRooms.set(roomId, { callId: snapshot.callId, participants: updated });
		} else {
			this.serverRooms.delete(roomId);
		}
	}

	/**
	 * Handle a CallEndedEvent — clear the room's server-side call snapshot.
	 */
	handleEnd(roomId: string, callId: string): void {
		const snapshot = this.serverRooms.get(roomId);
		if (!snapshot) return;
		if (snapshot.callId !== null && snapshot.callId !== callId) return;
		this.serverRooms.delete(roomId);
	}

	/**
	 * Clear state.
	 */
	clear(): void {
		this.serverRooms.clear();
	}
}
