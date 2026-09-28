import { beforeEach, expect, it, vi } from 'vitest';
import { createEffectivePermissionAPI } from '../effectivePermissions.js';
import { PermissionService } from '@chatto/api-types/api/v1/permissions_connect';
import { EffectivePermissionScopeKind } from '@chatto/api-types/api/v1/permissions_pb';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(PermissionService);
beforeEach(() => {
  vi.resetAllMocks();
});

it('calls the read-only public API once with the target ID and coverage metadata', async () => {
  const api = createEffectivePermissionAPI(
    fakeServer((router) => router.service(PermissionService, mocks))
  );
  mocks.listEffectivePermissions.mockReturnValue({
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
  await expect(api.listEffectivePermissions('bot')).resolves.toEqual([
    {
      permission: 'message.read',
      scope: 'room',
      scopeId: 'r',
      scopeName: 'general',
      parentGroupId: 'g',
      coversDescendants: true
    }
  ]);
  expect(mocks.listEffectivePermissions).toHaveBeenCalledOnce();
  expect(receivedRequest(mocks.listEffectivePermissions)).toMatchObject({ userId: 'bot' });
  mocks.listEffectivePermissions.mockReturnValue({
    permissions: [{ scope: { kind: 999 as EffectivePermissionScopeKind } }]
  });
  await expect(api.listEffectivePermissions('bot')).rejects.toThrow(
    'Unsupported effective permission scope'
  );
});
