import type { EventEnvelope } from '$lib/eventBus.svelte';
import {
	MessagesStore,
	RoomFilesStore,
	RoomMembersStore,
	setRoomMembersStore
} from '$lib/state/room';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

/**
 * Owns the room-scoped stores that must move together when the URL server or
 * room changes without remounting `Room.svelte`.
 */
export class RoomStores {
	readonly files: RoomFilesStore;
	readonly members: RoomMembersStore;
	readonly messages: MessagesStore;

	constructor(connection: ServerConnection, getCurrentUserId: () => string | null) {
		this.files = new RoomFilesStore(connection);
		this.members = setRoomMembersStore(new RoomMembersStore(connection));
		this.messages = new MessagesStore(connection, getCurrentUserId);
	}

	sync(connection: ServerConnection, roomId: string): void {
		this.messages.setConnection(connection);
		this.files.setConnection(connection);
		this.members.setConnection(connection);
		this.messages.setRoom(roomId);
		this.files.setRoom(roomId);
		this.members.setRoom(roomId);
	}

	ingestServerEvent(event: EventEnvelope): void {
		this.files.ingestServerEvent(event);
		this.members.ingestServerEvent(event);
	}

	dispose(): void {
		this.messages.dispose();
	}
}
