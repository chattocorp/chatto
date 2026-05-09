/**
 * Tracks which rooms have active voice calls and who's in each call.
 *
 * Uses the `activeCallRoomIds` GraphQL query (backed by LiveKit's ListRooms API)
 * as the source of truth. Real-time updates come from room events:
 * - CallParticipantJoinedEvent → add participant to the room
 * - CallParticipantLeftEvent → remove participant; delete room if empty
 *
 * Also includes the local user's active call from VoiceCallState for instant feedback.
 */

import { SvelteMap } from 'svelte/reactivity';
import { graphql } from '$lib/gql';
import type { UserAvatarUserFragment } from '$lib/gql/graphql';
import type { Client } from '@urql/svelte';
import type { VoiceCallState } from '$lib/state/instance/voiceCall.svelte';

const ActiveCallRoomIdsQuery = graphql(`
	query GetActiveCallRoomIds {
		activeCallRoomIds
	}
`);

const CallParticipantsQuery = graphql(`
	query GetSidebarCallParticipants($roomId: ID!) {
		callParticipants(roomId: $roomId) {
			userId
			displayName
			login
			avatarUrl
			joinedAt
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

export class ActiveCallRoomsState {
	#client: Client;
	#voiceCall: VoiceCallState;

	/** Map of room ID → participants for rooms with active calls. */
	private serverRooms = new SvelteMap<string, CallRoomParticipant[]>();

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
		return this.serverRooms.get(roomId) ?? [];
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

				if (participantResult.data?.callParticipants) {
					this.serverRooms.set(
						roomId,
						participantResult.data.callParticipants.map((p) => ({
							userId: p.userId,
							displayName: p.displayName,
							login: p.login,
							avatarUrl: p.avatarUrl ?? null
						}))
					);
				} else if (!this.serverRooms.has(roomId)) {
					// Room is active but we couldn't fetch participants
					this.serverRooms.set(roomId, []);
				}
			})
		);
	}

	/**
	 * Handle a CallParticipantJoinedEvent — add participant to the room.
	 */
	handleJoin(roomId: string, actor: UserAvatarUserFragment | null): void {
		const existing = this.serverRooms.get(roomId) ?? [];

		if (actor) {
			// Avoid duplicates
			if (existing.some((p) => p.userId === actor.id)) return;

			this.serverRooms.set(roomId, [
				...existing,
				{
					userId: actor.id,
					displayName: actor.displayName,
					login: actor.login,
					avatarUrl: actor.avatarUrl ?? null
				}
			]);
		} else if (!this.serverRooms.has(roomId)) {
			// No actor data but room is now active
			this.serverRooms.set(roomId, []);
		}
	}

	/**
	 * Handle a CallParticipantLeftEvent — remove participant from the room.
	 * Deletes the room entry if no participants remain.
	 */
	handleLeave(roomId: string, actorId: string | null): void {
		if (!actorId) return;

		const existing = this.serverRooms.get(roomId);
		if (!existing) return;

		const updated = existing.filter((p) => p.userId !== actorId);
		if (updated.length > 0) {
			this.serverRooms.set(roomId, updated);
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
