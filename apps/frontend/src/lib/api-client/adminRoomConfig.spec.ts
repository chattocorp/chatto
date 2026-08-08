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

	it('maps stored, effective, and source values', async () => {
		getRoomConfig.mockResolvedValue({
			state: {
				layer: { authorEditWindowSeconds: 1800 },
				effective: { authorEditWindowSeconds: 1800 },
				sources: {
					authorEditWindow: { source: { case: 'roomId', value: 'R1' } }
				}
			}
		});
		const api = createAdminRoomConfigAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

		await expect(api.getRoomConfig({ kind: 'room', id: 'R1' })).resolves.toEqual({
			authorEditWindowSeconds: 1800,
			effectiveAuthorEditWindowSeconds: 1800,
			authorEditWindowSource: { kind: 'room', id: 'R1' }
		});
		expect(getRoomConfig).toHaveBeenCalledWith(
			{ scope: { scope: { case: 'roomId', value: 'R1' } } },
			expect.objectContaining({ headers: new Headers({ Authorization: 'Bearer token' }) })
		);
	});

	it('clears a layer value by omitting the optional field', async () => {
		updateRoomConfig.mockResolvedValue({
			state: {
				layer: {},
				effective: { authorEditWindowSeconds: 10800 },
				sources: { authorEditWindow: { source: { case: 'productDefault', value: true } } }
			}
		});
		const api = createAdminRoomConfigAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

		await expect(api.updateRoomConfig({ kind: 'server' }, null)).resolves.toEqual({
			authorEditWindowSeconds: null,
			effectiveAuthorEditWindowSeconds: 10800,
			authorEditWindowSource: { kind: 'product-default', id: null }
		});
		expect(updateRoomConfig).toHaveBeenCalledWith(
			{
				scope: { scope: { case: 'server', value: true } },
				layer: { authorEditWindowSeconds: undefined },
				updateMask: { paths: ['author_edit_window_seconds'] }
			},
			{ headers: new Headers({ Authorization: 'Bearer token' }) }
		);
	});
});
