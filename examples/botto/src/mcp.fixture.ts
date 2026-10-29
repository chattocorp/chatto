/** Minimal MCP server fixture using Pi's in-memory transport; no sockets or binaries. */
import { createInMemoryTransportPair } from '@earendil-works/pi-mcp/testing';
import type { CallToolResult, Tool } from '@earendil-works/pi-mcp';

/** Return a client transport connected to a small JSON-RPC tool server. */
export function mcpFixture(
  tools: Tool[],
  call: (name: string, args: Record<string, unknown>) => Promise<CallToolResult>
) {
  const { client, server } = createInMemoryTransportPair();
  server.onMessage((message) => {
    if (!('method' in message) || !('id' in message)) return;
    void (async () => {
      const params = (message.params ?? {}) as Record<string, unknown>;
      let result: unknown;
      if (message.method === 'initialize')
        result = {
          protocolVersion: '2025-11-25',
          capabilities: { tools: {} },
          serverInfo: { name: 'fixture', version: '1' },
          instructions: 'Fixture workflow'
        };
      else if (message.method === 'tools/list') result = { tools };
      else if (message.method === 'tools/call')
        result = await call(
          String(params.name),
          (params.arguments ?? {}) as Record<string, unknown>
        );
      else result = {};
      await server.send({ jsonrpc: '2.0', id: message.id, result });
    })();
  });
  void server.start();
  return client;
}
