/**
 * Pure unit tests for the permissionMutations dispatch helper. Verifies
 * that each scope is encoded into the protobuf wire request.
 */

import { describe, it, expect, vi } from 'vitest';
import {
  PermissionEditState,
  type SetRolePermissionStateRequest,
  type SetUserPermissionStateRequest
} from '$lib/pb/chatto/api/v1/chat_pb';
import type { WireClient } from '$lib/wire/client';
import { setRolePermission } from './permissionMutations';
import { setUserPermission } from './userPermissionMutations';

function mockRoleClient(result: Promise<unknown> = Promise.resolve({ changed: true })) {
  const setRolePermissionState = vi.fn(() => result);
  return {
    client: { setRolePermissionState } as unknown as WireClient,
    setRolePermissionState
  };
}

function mockUserClient(result: Promise<unknown> = Promise.resolve({ changed: true })) {
  const setUserPermissionState = vi.fn(() => result);
  return {
    client: { setUserPermissionState } as unknown as WireClient,
    setUserPermissionState
  };
}

function lastRequest<T>(callable: ReturnType<typeof vi.fn>): T {
  return callable.mock.calls[callable.mock.calls.length - 1]?.[0] as T;
}

describe('setRolePermission dispatch', () => {
  describe('room scope', () => {
    it.each([
      ['allow', PermissionEditState.ALLOW],
      ['deny', PermissionEditState.DENY],
      ['neutral', PermissionEditState.NEUTRAL]
    ] as const)('uses room mutations for %s', async (state, expected) => {
      const { client, setRolePermissionState } = mockRoleClient();
      await setRolePermission(
        client,
        { tier: 'room', roleName: 'admin', roomId: 'R1' },
        'message.post',
        state
      );
      const request = lastRequest<SetRolePermissionStateRequest>(setRolePermissionState);
      expect(request.roleName).toBe('admin');
      expect(request.roomId).toBe('R1');
      expect(request.permission).toBe('message.post');
      expect(request.state).toBe(expected);
    });
  });

  describe('server scope', () => {
    it.each([
      ['allow', PermissionEditState.ALLOW],
      ['deny', PermissionEditState.DENY],
      ['neutral', PermissionEditState.NEUTRAL]
    ] as const)('uses server-tier mutations for %s', async (state, expected) => {
      const { client, setRolePermissionState } = mockRoleClient();
      await setRolePermission(
        client,
        { tier: 'server', roleName: 'admin' },
        'message.post',
        state
      );
      const request = lastRequest<SetRolePermissionStateRequest>(setRolePermissionState);
      expect(request.roleName).toBe('admin');
      expect(request.roomId).toBe('');
      expect(request.groupId).toBe('');
      expect(request.permission).toBe('message.post');
      expect(request.state).toBe(expected);
    });
  });

  it('encodes group scope', async () => {
    const { client, setRolePermissionState } = mockRoleClient();
    await setRolePermission(
      client,
      { tier: 'group', roleName: 'moderator', groupId: 'G1' },
      'room.join',
      'allow'
    );
    const request = lastRequest<SetRolePermissionStateRequest>(setRolePermissionState);
    expect(request.roleName).toBe('moderator');
    expect(request.groupId).toBe('G1');
    expect(request.state).toBe(PermissionEditState.ALLOW);
  });

  it('encodes user permission scope', async () => {
    const { client, setUserPermissionState } = mockUserClient();
    await setUserPermission(
      client,
      'U1',
      { tier: 'group', groupId: 'G1' },
      'room.join',
      'deny'
    );
    const request = lastRequest<SetUserPermissionStateRequest>(setUserPermissionState);
    expect(request.userId).toBe('U1');
    expect(request.groupId).toBe('G1');
    expect(request.permission).toBe('room.join');
    expect(request.state).toBe(PermissionEditState.DENY);
  });

  it('returns the error message when the role request fails', async () => {
    const { client } = mockRoleClient(Promise.reject(new Error('boom')));
    const result = await setRolePermission(
      client,
      { tier: 'server', roleName: 'admin' },
      'message.post',
      'allow'
    );
    expect(result.error).toBe('boom');
  });

  it('returns the error message when the user request fails', async () => {
    const { client } = mockUserClient(Promise.reject(new Error('boom')));
    const result = await setUserPermission(
      client,
      'U1',
      { tier: 'server' },
      'message.post',
      'allow'
    );
    expect(result.error).toBe('boom');
  });

  it('returns no error when the role request succeeds', async () => {
    const { client } = mockRoleClient();
    const result = await setRolePermission(
      client,
      { tier: 'server', roleName: 'admin' },
      'message.post',
      'allow'
    );
    expect(result.error).toBeUndefined();
  });

  it('returns no error when the user request succeeds', async () => {
    const { client } = mockUserClient();
    const result = await setUserPermission(
      client,
      'U1',
      { tier: 'server' },
      'message.post',
      'allow'
    );
    expect(result.error).toBeUndefined();
  });
});
