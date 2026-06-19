/**
 * Tracks which rooms have active voice calls and who's in each call.
 *
 * Uses the wire `ListActiveCalls` request as the source of truth. Real-time
 * updates come from room events:
 * - CallParticipantJoinedEvent → add participant to the room
 * - CallParticipantLeftEvent → remove participant; delete room if empty
 * - CallEndedEvent → delete the room regardless of participant snapshot
 *
 * Also includes the local user's active call from VoiceCallState for instant feedback.
 */

import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { ListActiveCallsRequest, type CallParticipantView } from '$lib/pb/chatto/api/v1/chat_pb';
import type { User } from '$lib/pb/chatto/core/v1/models_pb';
import type { WireClient } from '$lib/wire/client';
import type { VoiceCallState } from '$lib/state/server/voiceCall.svelte';
import { wireEventBusManager } from './wireEventBus.svelte';

/** Participant info for display in the room list sidebar. */
export type CallRoomParticipant = {
	userId: string;
	displayName: string;
	login: string;
	avatarUrl: string | null;
};

type ActiveCallRoomSnapshot = {
	callId: string | null;
	participants: CallRoomParticipant[];
};

type ParticipantActor = Pick<User, 'id' | 'displayName' | 'login'> & {
	avatarUrl?: string | null;
};

export class ActiveCallRoomsState {
	#voiceCall: VoiceCallState;
	#getWireClient: () => WireClient | null | undefined;

	/** Map of room ID → server-observed active call snapshot. */
	private serverRooms = new SvelteMap<string, ActiveCallRoomSnapshot>();

	constructor(
		serverId: string,
		voiceCall: VoiceCallState,
		getWireClient: () => WireClient | null | undefined = () =>
			wireEventBusManager.getClient(serverId)
	) {
		this.#voiceCall = voiceCall;
		this.#getWireClient = getWireClient;
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
	 * Load active call room IDs and their participants from the server.
	 * Should be called when entering the chat (alongside room list loading).
	 */
	async load(): Promise<void> {
		const client = this.#getWireClient();
		if (!client) return;

		const result = await client.listActiveCalls(new ListActiveCallsRequest());
		const roomIds = new SvelteSet(result.calls.map((call) => call.roomId).filter(Boolean));
		for (const id of this.serverRooms.keys()) {
			if (!roomIds.has(id)) {
				this.serverRooms.delete(id);
			}
		}

		for (const call of result.calls) {
			if (!call.roomId) continue;
			this.serverRooms.set(call.roomId, {
				callId: call.callId || call.participants[0]?.callId || null,
				participants: call.participants.map(toCallRoomParticipant).filter((p) => p.userId)
			});
		}
	}

	/**
	 * Handle a CallParticipantJoinedEvent — add participant to the room.
	 */
	handleJoin(roomId: string, callId: string, actor: ParticipantActor | null): void {
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
		if (this.serverRooms.get(roomId)?.callId !== callId) return;
		this.serverRooms.delete(roomId);
	}

	/**
	 * Clear state.
	 */
	clear(): void {
		this.serverRooms.clear();
	}
}

function toCallRoomParticipant(p: CallParticipantView): CallRoomParticipant {
	const user = p.user;
	return {
		userId: user?.id ?? '',
		displayName: user?.displayName ?? '',
		login: user?.login ?? '',
		avatarUrl: null
	};
}
