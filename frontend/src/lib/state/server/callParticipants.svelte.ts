/**
 * Tracks participants in an active voice call for a specific room.
 *
 * Used by the observer mode of VoiceCallPanel to show who's in a call
 * to room members who haven't joined yet.
 *
 * Data sources:
 * - Initial load: wire `GetCallParticipants` request (from the call-state projection)
 * - Real-time updates: Optimistic adds/removes from CallParticipantJoined/Left events
 */

import { GetCallParticipantsRequest, type CallParticipantView } from '$lib/pb/chatto/api/v1/chat_pb';
import type { User } from '$lib/pb/chatto/core/v1/models_pb';
import type { WireClient } from '$lib/wire/client';
import { wireEventBusManager } from './wireEventBus.svelte';

/** Participant info stored in observer mode. */
export type ObserverParticipant = {
	userId: string;
	displayName: string;
	login: string;
	avatarUrl: string | null;
};

type ParticipantActor = Pick<User, 'id' | 'displayName' | 'login'> & {
	avatarUrl?: string | null;
};

export class CallParticipantsState {
	#getWireClient: () => WireClient | null | undefined;

	/** Current participants visible to observers. */
	participants = $state<ObserverParticipant[]>([]);

	/** The room these participants are for. */
	private currentRoomId: string | null = null;
	private currentCallId: string | null = null;

	constructor(
		serverId: string,
		getWireClient: () => WireClient | null | undefined = () =>
			wireEventBusManager.getClient(serverId)
	) {
		this.#getWireClient = getWireClient;
	}

	/**
	 * Load participants from the server for a specific room.
	 * Called when entering a room that has an active call.
	 */
	async load(roomId: string): Promise<void> {
		this.currentRoomId = roomId;

		const client = this.#getWireClient();
		if (!client) return;

		const result = await client.getCallParticipants(new GetCallParticipantsRequest({ roomId }));
		const participants = result.participants;
		if (participants.length > 0) {
			this.currentCallId = participants[0]?.callId ?? null;
			this.participants = participants.map(toObserverParticipant).filter((p) => p.userId);
		} else {
			this.currentCallId = null;
			this.participants = [];
		}
	}

	/**
	 * Optimistically add a participant from a CallParticipantJoinedEvent.
	 * Uses the actor data from the Event envelope.
	 */
	handleJoin(roomId: string, callId: string, actor: ParticipantActor | null): void {
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
		if (this.currentCallId !== callId) return;
		this.clear();
	}

	/** Clear state (e.g., when leaving a room or call ends). */
	clear(): void {
		this.participants = [];
		this.currentRoomId = null;
		this.currentCallId = null;
	}
}

function toObserverParticipant(p: CallParticipantView): ObserverParticipant {
	const user = p.user;
	return {
		userId: user?.id ?? '',
		displayName: user?.displayName ?? '',
		login: user?.login ?? '',
		avatarUrl: null
	};
}
