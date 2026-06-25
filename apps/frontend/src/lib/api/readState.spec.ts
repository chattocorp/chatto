import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createReadStateAPI } from './readState';

const mocks = vi.hoisted(() => ({
	createClient: vi.fn(),
	createConnectTransport: vi.fn(),
	handleAuthenticationRequired: vi.fn(),
	markRoomAsRead: vi.fn(),
	markThreadAsRead: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
	const actual = await importOriginal<typeof import('@connectrpc/connect')>();
	return {
		...actual,
		createClient: mocks.createClient
	};
});

vi.mock('@connectrpc/connect-web', () => ({
	createConnectTransport: mocks.createConnectTransport
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
	serverRegistry: {
		handleAuthenticationRequired: mocks.handleAuthenticationRequired
	}
}));

describe('createReadStateAPI', () => {
	beforeEach(() => {
		mocks.createClient.mockReset();
		mocks.createConnectTransport.mockReset();
		mocks.handleAuthenticationRequired.mockReset();
		mocks.markRoomAsRead.mockReset();
		mocks.markThreadAsRead.mockReset();
		mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
		mocks.createClient.mockReturnValue({
			markRoomAsRead: mocks.markRoomAsRead,
			markThreadAsRead: mocks.markThreadAsRead
		});
	});

	it('marks a room read with bearer auth and converts timestamp fields', async () => {
		mocks.markRoomAsRead.mockResolvedValue({
			lastReadAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
			previousLastReadAt: Timestamp.fromDate(new Date('2026-06-01T11:00:00Z'))
		});

		const api = createReadStateAPI({
			serverId: 'remote',
			baseUrl: 'https://remote.example.test/api/connect',
			bearerToken: 'remote-token'
		});
		const result = await api.markRoomAsRead({
			roomId: 'room-1',
			upToEventId: 'event-2'
		});

		expect(mocks.createConnectTransport).toHaveBeenCalledWith({
			baseUrl: 'https://remote.example.test/api/connect',
			useBinaryFormat: true
		});
		expect(mocks.markRoomAsRead).toHaveBeenCalledWith(
			{
				roomId: 'room-1',
				upToEventId: 'event-2'
			},
			{
				headers: { Authorization: 'Bearer remote-token' }
			}
		);
		expect(result).toEqual({
			lastReadAt: '2026-06-01T12:00:00.000Z',
			previousLastReadAt: '2026-06-01T11:00:00.000Z'
		});
	});

	it('marks a thread read without auth headers when no token is available', async () => {
		mocks.markThreadAsRead.mockResolvedValue({
			previousReadAt: Timestamp.fromDate(new Date('2026-06-01T10:00:00Z'))
		});

		const api = createReadStateAPI({
			baseUrl: 'https://remote.example.test/api/connect',
			bearerToken: null
		});
		const result = await api.markThreadAsRead({
			roomId: 'room-1',
			threadRootEventId: 'root-1'
		});

		expect(mocks.markThreadAsRead).toHaveBeenCalledWith(
			{
				roomId: 'room-1',
				threadRootEventId: 'root-1',
				upToEventId: ''
			},
			{
				headers: undefined
			}
		);
		expect(result).toEqual({
			previousReadAt: '2026-06-01T10:00:00.000Z'
		});
	});

	it('marks the server authentication stale on unauthenticated Connect errors', async () => {
		const err = new ConnectError('authentication required', Code.Unauthenticated);
		mocks.markRoomAsRead.mockRejectedValue(err);

		const api = createReadStateAPI({
			serverId: 'remote',
			baseUrl: 'https://remote.example.test/api/connect',
			bearerToken: 'expired-token'
		});

		await expect(api.markRoomAsRead({ roomId: 'room-1' })).rejects.toBe(err);

		expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
	});
});
