/** Connect one turn to Chatto MCP. The caller owns and closes the connection. */
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import type { ToolDefinition } from '@earendil-works/pi-coding-agent';
import type { Settings } from './settings.ts';

/** Discover the complete tool catalog. No tool calls are retried by this bridge. */
export async function connectChattoMcp(settings: Settings, signal: AbortSignal) {
  const client = new Client({ name: 'botto', version: '0.1.0' });
  const close = () => client.close();
  const abort = () => {
    void close().catch(() => {});
  };
  signal.throwIfAborted();
  signal.addEventListener('abort', abort, { once: true });
  try {
    await client.connect(
      new StreamableHTTPClientTransport(new URL('/mcp', settings.serverUrl), {
        requestInit: {
          headers: { Authorization: `Bearer ${settings.apiKey}` },
          redirect: 'error'
        }
      }),
      { signal }
    );
    const tools: ToolDefinition[] = [];
    let cursor: string | undefined;
    do {
      const page = await client.listTools({ cursor }, { signal });
      for (const tool of page.tools) {
        tools.push({
          name: tool.name,
          label: tool.title ?? tool.name,
          description: tool.description ?? tool.name,
          // Pi consumes JSON Schema; MCP supplies the same schema without TypeBox's type marker.
          parameters: tool.inputSchema as ToolDefinition['parameters'],
          async execute(_id, args, toolSignal) {
            const result = await client.callTool(
              { name: tool.name, arguments: args as Record<string, unknown> },
              undefined,
              { signal: toolSignal ? AbortSignal.any([signal, toolSignal]) : signal }
            );
            if (result.isError) throw new Error(JSON.stringify(result.content));
            return {
              content: [{ type: 'text', text: JSON.stringify(result) }],
              details: result
            };
          }
        });
      }
      cursor = page.nextCursor;
    } while (cursor);
    signal.throwIfAborted();
    return {
      tools,
      async close() {
        signal.removeEventListener('abort', abort);
        await close();
      }
    };
  } catch (error) {
    signal.removeEventListener('abort', abort);
    await close().catch(() => {});
    throw error;
  }
}
