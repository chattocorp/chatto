/**
 * Tracks participants in an active voice call for a specific room.
 *
 * Used by the observer mode of VoiceCallPanel to show who's in a call
 * to room members who haven't joined yet.
 *
 * Data sources:
 * - Initial load: `callParticipants` GraphQL query (from the call-state projection)
 * - Real-time updates: Optimistic adds/removes from CallParticipantJoined/Left events
 */

import { graphql, useFragment } from '$lib/gql';
import type {
	GetCallParticipantsQuery,
	UserAvatarUserFragment
} from '$lib/gql/graphql';
import { UserAvatarUserFragmentDoc } from '$lib/gql/graphql';
import type { Client } from '@urql/svelte';

const CallParticipantsQuery = graphql(`
	query GetCallParticipants($roomId: ID!) {
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

/** Participant info stored in observer mode. */
export type ObserverParticipant = {
	userId: string;
	displayName: string;
	login: string;
	avatarUrl: string | null;
};

type QueryCallParticipant = NonNullable<GetCallParticipantsQuery['room']>['callParticipants'][number];

export class CallParticipantsState {
	#client: Client;

	/** Current participants visible to observers. */
	participants = $state<ObserverParticipant[]>([]);

	/** The room these participants are for. */
	private currentRoomId: string | null = null;
	private currentCallId: string | null = null;

	constructor(client: Client) {
		this.#client = client;
	}

	/**
	 * Load participants from the server for a specific room.
	 * Called when entering a room that has an active call.
	 */
	async load(roomId: string): Promise<void> {
		this.currentRoomId = roomId;

		const result = await this.#client
			.query(CallParticipantsQuery, { roomId })
			.toPromise();

		const participants = result.data?.room?.callParticipants;
		if (participants) {
			this.currentCallId = participants[0]?.callId ?? null;
			this.participants = participants.map(toObserverParticipant);
		}
	}

	/**
	 * Optimistically add a participant from a CallParticipantJoinedEvent.
	 * Uses the actor data from the Event envelope.
	 */
	handleJoin(roomId: string, callId: string, actor: UserAvatarUserFragment | null): void {
		if (roomId !== this.currentRoomId) return;
		if (this.currentCallId && this.currentCallId !== callId) return;
		if (!actor) return;

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
		this.participants = [];
		this.currentRoomId = null;
		this.currentCallId = null;
	}
}

function toObserverParticipant(p: QueryCallParticipant): ObserverParticipant {
	const user = useFragment(UserAvatarUserFragmentDoc, p.user);
	return {
		userId: user.id,
		displayName: user.displayName,
		login: user.login,
		avatarUrl: user.avatarUrl ?? null
	};
}
