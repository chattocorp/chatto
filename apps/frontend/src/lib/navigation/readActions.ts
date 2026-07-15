import { createReadStateAPI } from '$lib/api-client/readState';
import { createRoomDirectoryAPI, RoomDirectoryScope } from '$lib/api-client/roomDirectory';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';

function apiConfig(serverId: string) {
	const connection = serverConnectionManager.getClient(serverId);
	return {
		serverId: connection.serverId ?? serverId,
		baseUrl: connection.connectBaseUrl,
		bearerToken: connection.bearerToken
	};
}

/** Mark one room read while keeping the local unread indicator optimistic and race-safe. */
export async function markNavigationRoomAsRead(serverId: string, roomId: string): Promise<boolean> {
	const unread = serverRegistry.getStore(serverId).roomUnread;
	const optimisticRead = unread.beginOptimisticRead(roomId);

	try {
		await createReadStateAPI(apiConfig(serverId)).markRoomAsRead({ roomId });
		optimisticRead.commit();
		return true;
	} catch (error) {
		optimisticRead.rollback();
		console.error('Failed to mark room as read:', error);
		return false;
	}
}

/** Resolve the server's room snapshot, then mark every unread joined room as read. */
export async function markNavigationServerAsRead(serverId: string): Promise<boolean> {
	const unread = serverRegistry.getStore(serverId).roomUnread;
	const snapshotRevision = unread.captureSnapshotRevision();

	try {
		const rooms = await createRoomDirectoryAPI(apiConfig(serverId)).listRooms(
			RoomDirectoryScope.ALL
		);
		unread.updateRooms(rooms, snapshotRevision);
		unread.resolveUnknownUnread();

		const unreadRoomIds = rooms
			.filter((room) => room.hasUnread || unread.roomIsUnread(room.id))
			.map((room) => room.id);
		const results = await Promise.all(
			unreadRoomIds.map((roomId) => markNavigationRoomAsRead(serverId, roomId))
		);
		return results.every(Boolean);
	} catch (error) {
		console.error('Failed to mark server as read:', error);
		return false;
	}
}
