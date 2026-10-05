import { describe, expect, it } from 'vitest';
import { flushSync } from 'svelte';
import {
  removeRegisteredAdminQueries,
  removeRegisteredServerQueries
} from '$lib/query/cacheRegistry';
import type { ServerScope } from './scope.svelte';
import type { ServerConnection } from '@chatto/client/server/serverConnection';
import { createSessionGuard, type SessionGuard } from './sessionGuard.svelte';

function makeScope(serverId = 'S1') {
  const scope = $state({
    serverId,
    connection: { queryScope: 'session-1' } as ServerConnection,
    current: true
  });
  return {
    scope,
    serverScope: {
      get serverId() {
        return scope.serverId;
      },
      get connection() {
        return scope.connection;
      },
      store: {} as ServerScope['store'],
      isCurrent: () => scope.current
    } satisfies ServerScope
  };
}

function withGuard(fence?: 'private-data' | 'server-session') {
  const { scope, serverScope } = makeScope();
  let guard!: SessionGuard;
  const destroy = $effect.root(() => {
    guard = createSessionGuard(serverScope, fence);
  });
  flushSync();
  return { scope, guard, destroy };
}

describe('SessionGuard', () => {
  it('keeps a snapshot current until the session changes', () => {
    const { guard, destroy } = withGuard();
    const snapshot = { ...guard.snapshot(), extra: 'value' };

    expect(snapshot).toMatchObject({ serverId: 'S1', generation: 0, extra: 'value' });
    expect(guard.isCurrent(snapshot)).toBe(true);
    expect(guard.isCurrent(undefined)).toBe(false);
    destroy();
  });

  it.each([
    [
      'the route subtree is no longer current',
      (scope: ReturnType<typeof makeScope>['scope']) => {
        scope.current = false;
      }
    ]
  ])('makes a snapshot stale when %s', (_case, change) => {
    const { scope, guard, destroy } = withGuard();
    const snapshot = guard.snapshot();

    change(scope);

    expect(guard.isCurrent(snapshot)).toBe(false);
    destroy();
  });

  it('makes earlier snapshots stale on invalidate and on destroy', () => {
    const { guard, destroy } = withGuard();
    const first = guard.snapshot();
    guard.invalidate();
    const second = guard.snapshot();

    expect(guard.isCurrent(first)).toBe(false);
    expect(guard.isCurrent(second)).toBe(true);
    destroy();
    expect(guard.isCurrent(second)).toBe(false);
  });

  it('ends the session when a removal and the destroy happen in the same tick', () => {
    // Sign-out removes private data and then destroys the route before a flush.
    const { guard, destroy } = withGuard();
    removeRegisteredAdminQueries('S1');
    const afterRemoval = guard.snapshot();

    destroy();

    expect(guard.isCurrent(afterRemoval)).toBe(false);
  });

  it('ends a private-data session when the server removes its private or admin queries', () => {
    const { guard, destroy } = withGuard();
    const snapshot = guard.snapshot();

    removeRegisteredAdminQueries('OTHER');
    expect(guard.isCurrent(snapshot)).toBe(true);
    removeRegisteredAdminQueries('S1');
    expect(guard.isCurrent(snapshot)).toBe(false);
    destroy();
  });

  it('ends a server-session guard with the server session, not with an admin data removal', () => {
    const { guard, destroy } = withGuard('server-session');
    const snapshot = guard.snapshot();

    removeRegisteredAdminQueries('S1');
    expect(guard.isCurrent(snapshot)).toBe(true);
    removeRegisteredServerQueries('S1');
    expect(guard.isCurrent(snapshot)).toBe(false);
    destroy();
  });

  it('stops listening after destroy', () => {
    const { guard, destroy } = withGuard();
    destroy();
    const after = guard.snapshot();

    removeRegisteredAdminQueries('S1');

    expect(guard.isCurrent(after)).toBe(true);
  });
});
