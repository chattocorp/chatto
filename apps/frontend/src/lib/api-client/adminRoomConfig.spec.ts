import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createAdminRoomConfigAPI } from './adminRoomConfig';

const getRoomConfig = vi.hoisted(() => vi.fn());
const updateRoomConfig = vi.hoisted(() => vi.fn());

vi.mock('./connect.js', () => ({
	authHeaders: () => new Headers({ Authorization: 'Bearer token' }),
	createChattoClient: () => ({ getRoomConfig, updateRoomConfig }),
	handleAuthError: (_config: unknown, error: unknown) => {
		throw error;
	}
}));

describe('admin room configuration API', () => {
	beforeEach(() => {
		getRoomConfig.mockReset();
		updateRoomConfig.mockReset();
	});

	it('maps stored and effective values', async () => {
		getRoomConfig.mockResolvedValue({
			state: {
				layer: { authorEditWindow: { seconds: 1800n, nanos: 500_000_000 } },
				effective: { authorEditWindow: { seconds: 1800n, nanos: 500_000_000 } }
			}
		});
		const api = createAdminRoomConfigAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

		await expect(api.getRoomConfig({ kind: 'room', id: 'R1' })).resolves.toEqual({
			authorEditWindowSeconds: 1800.5,
			effectiveAuthorEditWindowSeconds: 1800.5
		});
		expect(getRoomConfig).toHaveBeenCalledWith(
			{ scope: { scope: { case: 'roomId', value: 'R1' } } },
			expect.objectContaining({ headers: new Headers({ Authorization: 'Bearer token' }) })
		);
	});

	it('encodes a layer value as a protobuf duration', async () => {
		updateRoomConfig.mockResolvedValue({
			state: {
				layer: { authorEditWindow: { seconds: 1800n, nanos: 0 } },
				effective: { authorEditWindow: { seconds: 1800n, nanos: 0 } }
			}
		});
		const api = createAdminRoomConfigAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

		await api.updateRoomConfig({ kind: 'server' }, 1800);

		expect(updateRoomConfig).toHaveBeenCalledWith(
			{
				scope: { scope: { case: 'server', value: true } },
				layer: { authorEditWindow: { seconds: 1800n, nanos: 0 } },
				updateMask: { paths: ['author_edit_window'] }
			},
			{ headers: new Headers({ Authorization: 'Bearer token' }) }
		);
	});

	it('clears a layer value by omitting the optional field', async () => {
		updateRoomConfig.mockResolvedValue({
			state: {
				layer: {},
				effective: { authorEditWindow: { seconds: 10800n, nanos: 0 } }
			}
		});
		const api = createAdminRoomConfigAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

		await expect(api.updateRoomConfig({ kind: 'server' }, null)).resolves.toEqual({
			authorEditWindowSeconds: null,
			effectiveAuthorEditWindowSeconds: 10800
		});
		expect(updateRoomConfig).toHaveBeenCalledWith(
			{
				scope: { scope: { case: 'server', value: true } },
				layer: { authorEditWindow: undefined },
				updateMask: { paths: ['author_edit_window'] }
			},
			{ headers: new Headers({ Authorization: 'Bearer token' }) }
		);
	});
});
