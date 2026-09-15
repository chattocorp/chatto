import { beforeEach, expect, it, vi } from 'vitest';
import { createEffectivePermissionAPI } from '$lib/api-client/effectivePermissions';
import { EffectivePermissionScopeKind } from '@chatto/api-types/api/v1/permissions_pb';
const mocks = vi.hoisted(() => ({ listEffectivePermissions: vi.fn() }));
vi.mock('@connectrpc/connect', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@connectrpc/connect')>()),
  createClient: () => mocks
}));
vi.mock('@connectrpc/connect-web', () => ({ createConnectTransport: () => ({}) }));
beforeEach(() => vi.resetAllMocks());
it('calls the read-only public API once with the target ID and coverage metadata', async () => {
  const api = createEffectivePermissionAPI({ baseUrl: '/api/connect', bearerToken: 'token' });
  const signal = new AbortController().signal;
  mocks.listEffectivePermissions.mockResolvedValue({
    permissions: [
      {
        permission: 'message.read',
        scope: {
          kind: EffectivePermissionScopeKind.ROOM,
          id: 'r',
          name: 'general',
          parentGroupId: 'g'
        },
        coversDescendants: true
      }
    ]
  });
  await expect(api.listEffectivePermissions('bot', signal)).resolves.toEqual([
    {
      permission: 'message.read',
      scope: 'room',
      scopeId: 'r',
      scopeName: 'general',
      parentGroupId: 'g',
      coversDescendants: true
    }
  ]);
  expect(mocks.listEffectivePermissions).toHaveBeenCalledWith(
    { userId: 'bot' },
    { headers: { Authorization: 'Bearer token' }, signal }
  );
  mocks.listEffectivePermissions.mockResolvedValue({ permissions: [{ scope: { kind: 999 } }] });
  await expect(api.listEffectivePermissions('bot')).rejects.toThrow(
    'Unsupported effective permission scope'
  );
});
