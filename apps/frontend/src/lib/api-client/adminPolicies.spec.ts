import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PolicySourceScope } from '@chatto/api-types/api/v1/policies_pb';
import { createAdminPolicyAPI } from './adminPolicies';

const getPolicyConfiguration = vi.hoisted(() => vi.fn());
const updatePolicyConfiguration = vi.hoisted(() => vi.fn());

vi.mock('./connect.js', () => ({
	authHeaders: () => new Headers({ Authorization: 'Bearer token' }),
	createChattoClient: () => ({ getPolicyConfiguration, updatePolicyConfiguration }),
	handleAuthError: (_config: unknown, error: unknown) => {
		throw error;
	}
}));

describe('admin policy API', () => {
	beforeEach(() => {
		getPolicyConfiguration.mockReset();
		updatePolicyConfiguration.mockReset();
	});

	it('maps stored, effective, and source values', async () => {
		getPolicyConfiguration.mockResolvedValue({
			policyConfiguration: {
				overrides: { authorEditWindowSeconds: 1800 },
				effective: { authorEditWindowSeconds: 1800 },
				sources: {
					authorEditWindow: { scope: PolicySourceScope.ROOM, scopeId: 'R1' }
				}
			}
		});
		const api = createAdminPolicyAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

		await expect(api.getPolicyConfiguration({ kind: 'room', id: 'R1' })).resolves.toEqual({
			authorEditWindowSeconds: 1800,
			effectiveAuthorEditWindowSeconds: 1800,
			authorEditWindowSource: { kind: 'room', id: 'R1' }
		});
		expect(getPolicyConfiguration).toHaveBeenCalledWith(
			{ scope: { scope: { case: 'roomId', value: 'R1' } } },
			expect.objectContaining({ headers: new Headers({ Authorization: 'Bearer token' }) })
		);
	});

	it('clears an override by omitting the optional field', async () => {
		updatePolicyConfiguration.mockResolvedValue({
			policyConfiguration: {
				overrides: {},
				effective: { authorEditWindowSeconds: 10800 },
				sources: { authorEditWindow: { scope: PolicySourceScope.PRODUCT_DEFAULT } }
			}
		});
		const api = createAdminPolicyAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

		await expect(api.updatePolicyConfiguration({ kind: 'server' }, null)).resolves.toEqual({
			authorEditWindowSeconds: null,
			effectiveAuthorEditWindowSeconds: 10800,
			authorEditWindowSource: { kind: 'product-default', id: null }
		});
		expect(updatePolicyConfiguration).toHaveBeenCalledWith(
			{
				scope: { scope: { case: 'server', value: true } },
				overrides: { authorEditWindowSeconds: undefined },
				updateMask: { paths: ['author_edit_window_seconds'] }
			},
			{ headers: new Headers({ Authorization: 'Bearer token' }) }
		);
	});
});
