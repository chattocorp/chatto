import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createThreadAPI } from './threads';

const mocks = vi.hoisted(() => ({
	createClient: vi.fn(),
	createConnectTransport: vi.fn(),
	handleAuthenticationRequired: vi.fn(),
	followThread: vi.fn(),
	unfollowThread: vi.fn()
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

describe('createThreadAPI', () => {
	beforeEach(() => {
		mocks.createClient.mockReset();
		mocks.createConnectTransport.mockReset();
		mocks.handleAuthenticationRequired.mockReset();
		mocks.followThread.mockReset();
		mocks.unfollowThread.mockReset();
		mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
		mocks.createClient.mockReturnValue({
			followThread: mocks.followThread,
			unfollowThread: mocks.unfollowThread
		});
	});

	it('follows a thread with bearer auth', async () => {
		mocks.followThread.mockResolvedValue({ following: true });

		const api = createThreadAPI({
			serverId: 'remote',
			baseUrl: 'https://remote.example.test/api/connect',
			bearerToken: 'remote-token'
		});
		const following = await api.followThread({
			roomId: 'room-1',
			threadRootEventId: 'root-1'
		});

		expect(mocks.createConnectTransport).toHaveBeenCalledWith({
			baseUrl: 'https://remote.example.test/api/connect',
			useBinaryFormat: true
		});
		expect(mocks.followThread).toHaveBeenCalledWith(
			{
				roomId: 'room-1',
				threadRootEventId: 'root-1'
			},
			{
				headers: { Authorization: 'Bearer remote-token' }
			}
		);
		expect(following).toBe(true);
	});

	it('unfollows a thread without auth headers when no token is available', async () => {
		mocks.unfollowThread.mockResolvedValue({ following: false });

		const api = createThreadAPI({
			baseUrl: 'https://remote.example.test/api/connect',
			bearerToken: null
		});
		const following = await api.unfollowThread({
			roomId: 'room-1',
			threadRootEventId: 'root-1'
		});

		expect(mocks.unfollowThread).toHaveBeenCalledWith(
			{
				roomId: 'room-1',
				threadRootEventId: 'root-1'
			},
			{
				headers: undefined
			}
		);
		expect(following).toBe(false);
	});

	it('marks the server authentication stale on unauthenticated Connect errors', async () => {
		const err = new ConnectError('authentication required', Code.Unauthenticated);
		mocks.followThread.mockRejectedValue(err);

		const api = createThreadAPI({
			serverId: 'remote',
			baseUrl: 'https://remote.example.test/api/connect',
			bearerToken: 'expired-token'
		});

		await expect(api.followThread({ roomId: 'room-1', threadRootEventId: 'root-1' })).rejects.toBe(
			err
		);

		expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
	});
});
