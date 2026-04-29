/**
 * Pure unit tests for the permissionMutations dispatch helper. Verifies
 * that each (tier, isInstanceRole) combination calls the right grant /
 * deny / clear mutation triple. The mock client.mutation captures the
 * GraphQL document so we can assert on its operation name.
 */

import { describe, it, expect, vi } from 'vitest';
import type { Client } from '@urql/svelte';
import { setRolePermission } from './permissionMutations';

function thenable(value: unknown) {
  // urql's `client.mutation()` returns a thenable OperationResultSource, so
  // `await client.mutation(...)` resolves to the OperationResult directly
  // (not via `.toPromise()`). Match that shape so the dispatcher's `await`
  // sees the same thing it sees in production.
  return {
    then: (resolve: (v: unknown) => void) => Promise.resolve(value).then(resolve),
    toPromise: () => Promise.resolve(value)
  };
}

function mockClient(result: { error: null | { message: string } } = { error: null }) {
  const mutation = vi.fn(() => thenable(result));
  return {
    client: {
      query: vi.fn(),
      mutation,
      subscription: vi.fn()
    } as unknown as Client,
    mutation
  };
}

function operationName(doc: unknown): string {
  const d = doc as { definitions?: Array<{ name?: { value?: string } }> } | undefined;
  return d?.definitions?.[0]?.name?.value ?? '';
}

function lastDoc(mutation: ReturnType<typeof vi.fn>): unknown {
  const call = mutation.mock.calls[mutation.mock.calls.length - 1];
  return call?.[0];
}

describe('setRolePermission dispatch', () => {
  describe('room scope', () => {
    it.each([
      ['allow', 'MatrixGrantRoomPerm'],
      ['deny', 'MatrixDenyRoomPerm'],
      ['neutral', 'MatrixClearRoomPerm']
    ] as const)('uses room mutations for %s (space role)', async (state, expected) => {
      const { client, mutation } = mockClient();
      await setRolePermission(
        client,
        {
          tier: 'room',
          roleName: 'admin',
          isInstanceRole: false,
          spaceId: 'S1',
          roomId: 'R1'
        },
        'message.post',
        state
      );
      expect(operationName(lastDoc(mutation))).toBe(expected);
    });

    it('uses room mutations even for instance roles (room overrides are uniform)', async () => {
      const { client, mutation } = mockClient();
      await setRolePermission(
        client,
        {
          tier: 'room',
          roleName: 'instance-admin',
          isInstanceRole: true,
          spaceId: 'S1',
          roomId: 'R1'
        },
        'message.post',
        'allow'
      );
      expect(operationName(lastDoc(mutation))).toBe('MatrixGrantRoomPerm');
    });
  });

  describe('space scope', () => {
    it.each([
      ['allow', 'MatrixGrantInstanceRoleSpacePerm'],
      ['deny', 'MatrixDenyInstanceRoleSpacePerm'],
      ['neutral', 'MatrixClearInstanceRoleSpacePerm']
    ] as const)(
      'instance role at space → instance-role-space mutations for %s',
      async (state, expected) => {
        const { client, mutation } = mockClient();
        await setRolePermission(
          client,
          { tier: 'space', roleName: 'instance-admin', isInstanceRole: true, spaceId: 'S1' },
          'message.post',
          state
        );
        expect(operationName(lastDoc(mutation))).toBe(expected);
      }
    );

    it.each([
      ['allow', 'MatrixGrantSpacePerm'],
      ['deny', 'MatrixDenySpacePerm'],
      ['neutral', 'MatrixClearSpacePerm']
    ] as const)(
      'space role at space → plain space mutations for %s',
      async (state, expected) => {
        const { client, mutation } = mockClient();
        await setRolePermission(
          client,
          { tier: 'space', roleName: 'admin', isInstanceRole: false, spaceId: 'S1' },
          'message.post',
          state
        );
        expect(operationName(lastDoc(mutation))).toBe(expected);
      }
    );
  });

  describe('instance scope', () => {
    it.each([
      ['allow', 'MatrixGrantInstancePerm'],
      ['deny', 'MatrixDenyInstancePerm'],
      ['neutral', 'MatrixClearInstancePerm']
    ] as const)('uses instance mutations for %s', async (state, expected) => {
      const { client, mutation } = mockClient();
      await setRolePermission(
        client,
        { tier: 'instance', roleName: 'instance-admin', isInstanceRole: true },
        'space.create',
        state
      );
      expect(operationName(lastDoc(mutation))).toBe(expected);
    });
  });

  it('returns the error message when the mutation fails', async () => {
    const { client } = mockClient({ error: { message: 'boom' } });
    const result = await setRolePermission(
      client,
      { tier: 'instance', roleName: 'instance-admin', isInstanceRole: true },
      'space.create',
      'allow'
    );
    expect(result.error).toBe('boom');
  });

  it('returns no error when the mutation succeeds', async () => {
    const { client } = mockClient();
    const result = await setRolePermission(
      client,
      { tier: 'instance', roleName: 'instance-admin', isInstanceRole: true },
      'space.create',
      'allow'
    );
    expect(result.error).toBeUndefined();
  });
});
