import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
	mocks: {
		markRoomAsRead: vi.fn(),
		listRooms: vi.fn(),
		commit: vi.fn(),
		rollback: vi.fn(),
		beginOptimisticRead: vi.fn(),
		captureSnapshotRevision: vi.fn().mockReturnValue(7),
		updateRooms: vi.fn(),
		resolveUnknownUnread: vi.fn(),
		roomIsUnread: vi.fn()
	}
}));

vi.mock('$lib/api-client/readState', () => ({
	createReadStateAPI: () => ({ markRoomAsRead: mocks.markRoomAsRead })
}));

vi.mock('$lib/api-client/roomDirectory', () => ({
	RoomDirectoryScope: { ALL: 1 },
	createRoomDirectoryAPI: () => ({ listRooms: mocks.listRooms })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
	serverRegistry: {
		getStore: () => ({
			roomUnread: {
				beginOptimisticRead: mocks.beginOptimisticRead,
				captureSnapshotRevision: mocks.captureSnapshotRevision,
				updateRooms: mocks.updateRooms,
				resolveUnknownUnread: mocks.resolveUnknownUnread,
				roomIsUnread: mocks.roomIsUnread
			}
		})
	}
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
	serverConnectionManager: {
		getClient: () => ({
			serverId: 'remote',
			connectBaseUrl: 'https://remote.example.test/api/connect',
			bearerToken: 'token'
		})
	}
}));

import { markNavigationRoomAsRead, markNavigationServerAsRead } from './readActions';

beforeEach(() => {
	vi.clearAllMocks();
	mocks.beginOptimisticRead.mockReturnValue({
		commit: mocks.commit,
		rollback: mocks.rollback
	});
	mocks.markRoomAsRead.mockResolvedValue({ lastReadAt: null, previousLastReadAt: null });
	mocks.listRooms.mockResolvedValue([]);
	mocks.roomIsUnread.mockReturnValue(false);
});

describe('navigation read actions', () => {
	it('commits an optimistic room read after the API succeeds', async () => {
		await expect(markNavigationRoomAsRead('remote', 'room-1')).resolves.toBe(true);

		expect(mocks.beginOptimisticRead).toHaveBeenCalledWith('room-1');
		expect(mocks.markRoomAsRead).toHaveBeenCalledWith({ roomId: 'room-1' });
		expect(mocks.commit).toHaveBeenCalledOnce();
		expect(mocks.rollback).not.toHaveBeenCalled();
	});

	it('rolls an optimistic room read back after the API fails', async () => {
		const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
		mocks.markRoomAsRead.mockRejectedValue(new Error('offline'));

		await expect(markNavigationRoomAsRead('remote', 'room-1')).resolves.toBe(false);

		expect(mocks.rollback).toHaveBeenCalledOnce();
		expect(mocks.commit).not.toHaveBeenCalled();
		consoleError.mockRestore();
	});

	it('resolves the room snapshot and marks snapshot or locally unread rooms', async () => {
		const rooms = [
			{ id: 'room-1', hasUnread: true },
			{ id: 'room-2', hasUnread: false },
			{ id: 'room-3', hasUnread: false }
		];
		mocks.listRooms.mockResolvedValue(rooms);
		mocks.roomIsUnread.mockImplementation((roomId: string) => roomId === 'room-2');

		await expect(markNavigationServerAsRead('remote')).resolves.toBe(true);

		expect(mocks.updateRooms).toHaveBeenCalledWith(rooms, 7);
		expect(mocks.resolveUnknownUnread).toHaveBeenCalledOnce();
		expect(mocks.markRoomAsRead).toHaveBeenCalledTimes(2);
		expect(mocks.markRoomAsRead).toHaveBeenCalledWith({ roomId: 'room-1' });
		expect(mocks.markRoomAsRead).toHaveBeenCalledWith({ roomId: 'room-2' });
	});
});
