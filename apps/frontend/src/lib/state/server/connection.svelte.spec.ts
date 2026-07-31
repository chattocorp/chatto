import { describe, expect, it, vi } from 'vitest';
import type { ServerConnection } from './serverConnection.svelte';

const { serverScope } = vi.hoisted(() => ({
  serverScope: {
    connection: null as unknown as ServerConnection
  }
}));

vi.mock('./scope.svelte', () => ({
  useServerScope: () => serverScope
}));

import { useConnection } from './connection.svelte';

describe('useConnection', () => {
  it('reads the current connection from the server scope', () => {
    const originConnection = { serverId: 'anonymous-origin' } as ServerConnection;
    const remoteConnection = { serverId: 'authenticated-remote' } as ServerConnection;
    serverScope.connection = originConnection;

    const connection = useConnection();
    expect(connection()).toBe(originConnection);

    serverScope.connection = remoteConnection;
    expect(connection()).toBe(remoteConnection);
  });
});
