import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { AdminPermissionService } from '@chatto/api-types/admin/v1/permissions_connect';
import {
  PermissionDecision,
  PermissionScopeKind,
  type SetUserPermissionRequest
} from '@chatto/api-types/admin/v1/permissions_pb';
import { createPermissionAPI } from '@chatto/client/api/permissions';
import { fakeServer, mockService } from '@chatto/client/testing/fakeServer';
import { PERMISSION_METADATA } from '$lib/permissions';
import {
  BOT_CAPABILITIES,
  applyBotCapabilities,
  botCapabilityAvailable,
  botCapabilityGrants
} from './botCapabilities';

const mocks = mockService(AdminPermissionService);

function api() {
  return createPermissionAPI(fakeServer((router) => router.service(AdminPermissionService, mocks)));
}

describe('BOT_CAPABILITIES', () => {
  it('only grants known permissions at a scope where they apply', () => {
    for (const capability of BOT_CAPABILITIES) {
      for (const grant of capability.grants) {
        const metadata = PERMISSION_METADATA[grant.permission];
        expect(metadata, grant.permission).toBeDefined();
        expect(metadata.scopes, grant.permission).toContain(grant.scope.tier);
        expect(metadata.privileged ?? false, grant.permission).toBe(false);
      }
    }
  });
});

describe('botCapabilityGrants', () => {
  it('returns nothing when no capability is selected', () => {
    expect(botCapabilityGrants([])).toEqual([]);
  });

  it('keeps server and DM grants of the same permission apart', () => {
    const grants = botCapabilityGrants(['read_all', 'direct_messages']);
    expect(grants.map((grant) => `${grant.scope.tier}:${grant.permission}`)).toEqual([
      'dm:message.read',
      'dm:message.post',
      'server:message.read'
    ]);
  });

  it('writes a shared grant once', () => {
    const grants = botCapabilityGrants(['post', 'post']);
    expect(grants.map((grant) => grant.permission)).toEqual(['message.post', 'message.echo']);
  });
});

describe('botCapabilityAvailable', () => {
  it('needs every permission of the capability', () => {
    expect(botCapabilityAvailable('join_rooms', { 'room.list': true, 'room.join': true })).toBe(
      true
    );
    expect(botCapabilityAvailable('join_rooms', { 'room.list': true })).toBe(false);
    expect(botCapabilityAvailable('join_rooms', { 'room.list': true, 'room.join': false })).toBe(
      false
    );
  });
});

describe('applyBotCapabilities', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('writes one allow per grant, one after another', async () => {
    const received: SetUserPermissionRequest[] = [];
    let inFlight = 0;
    mocks.setUserPermission.mockImplementation(async (request: SetUserPermissionRequest) => {
      inFlight += 1;
      expect(inFlight).toBe(1);
      received.push(request);
      await Promise.resolve();
      inFlight -= 1;
      return {
        decision: {
          permission: request.permission,
          scope: request.scope,
          decision: PermissionDecision.ALLOW
        }
      };
    });

    const result = await applyBotCapabilities(api(), 'B1', ['interactions', 'direct_messages']);

    expect(result.failed).toEqual([]);
    expect(
      received.map((request) => ({
        userId: request.userId,
        permission: request.permission,
        decision: request.decision,
        scope: request.scope?.kind
      }))
    ).toEqual([
      {
        userId: 'B1',
        permission: 'message.read-interactions',
        decision: PermissionDecision.ALLOW,
        scope: PermissionScopeKind.SERVER
      },
      {
        userId: 'B1',
        permission: 'message.post-in-interactions',
        decision: PermissionDecision.ALLOW,
        scope: PermissionScopeKind.SERVER
      },
      {
        userId: 'B1',
        permission: 'message.read',
        decision: PermissionDecision.ALLOW,
        scope: PermissionScopeKind.DM
      },
      {
        userId: 'B1',
        permission: 'message.post',
        decision: PermissionDecision.ALLOW,
        scope: PermissionScopeKind.DM
      }
    ]);
  });

  it('continues after a rejected grant and reports it', async () => {
    mocks.setUserPermission.mockImplementation((request: SetUserPermissionRequest) => {
      if (request.permission === 'room.list') {
        throw new ConnectError('owner ceiling', Code.PermissionDenied);
      }
      return {
        decision: {
          permission: request.permission,
          scope: request.scope,
          decision: PermissionDecision.ALLOW
        }
      };
    });

    const result = await applyBotCapabilities(api(), 'B1', ['join_rooms']);

    expect(mocks.setUserPermission).toHaveBeenCalledTimes(2);
    expect(result.failed).toMatchObject([{ capabilityId: 'join_rooms', permission: 'room.list' }]);
  });
});
