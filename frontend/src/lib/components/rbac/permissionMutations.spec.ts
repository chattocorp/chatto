/**
 * Pure unit tests for the permissionMutations dispatch helper. Verifies
 * that each scope (server, room) calls the right grant/deny/clear triple.
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
    ] as const)('uses room mutations for %s', async (state, expected) => {
      const { client, mutation } = mockClient();
      await setRolePermission(
        client,
        { tier: 'room', roleName: 'admin', roomId: 'R1' },
        'message.post',
        state
      );
      expect(operationName(lastDoc(mutation))).toBe(expected);
    });
  });

  describe('server scope', () => {
    it.each([
      ['allow', 'MatrixGrantServerPerm'],
      ['deny', 'MatrixDenyServerPerm'],
      ['neutral', 'MatrixClearServerPerm']
    ] as const)('uses server-tier mutations for %s', async (state, expected) => {
      const { client, mutation } = mockClient();
      await setRolePermission(
        client,
        { tier: 'server', roleName: 'admin' },
        'dm.write',
        state
      );
      expect(operationName(lastDoc(mutation))).toBe(expected);
    });
  });

  it('returns the error message when the mutation fails', async () => {
    const { client } = mockClient({ error: { message: 'boom' } });
    const result = await setRolePermission(
      client,
      { tier: 'server', roleName: 'admin' },
      'dm.write',
      'allow'
    );
    expect(result.error).toBe('boom');
  });

  it('returns no error when the mutation succeeds', async () => {
    const { client } = mockClient();
    const result = await setRolePermission(
      client,
      { tier: 'server', roleName: 'admin' },
      'dm.write',
      'allow'
    );
    expect(result.error).toBeUndefined();
  });
});
