/** Configure Pi's native MCP extension and observe calls without exposing their data. */
import { devNull } from 'node:os';
import {
  createMcpExtension,
  type ExtensionFactory,
  type McpServerEntry,
  type McpTransportFactory
} from '@earendil-works/pi-coding-agent';
import type { Settings } from './settings.ts';

/** Exa's hosted MCP endpoint, pinned to the search and fetch tools. */
export const EXA_MCP_URL = 'https://mcp.exa.ai/mcp?tools=web_search_exa,web_fetch_exa';

/** List Botto's explicit servers. Ambient project or global MCP configuration is never used. */
export function mcpServers(settings: Settings, memoryDirectory?: string): McpServerEntry[] {
  const servers: McpServerEntry[] = [];
  if (settings.mcp)
    servers.push({
      name: 'chatto',
      source: 'botto.toml',
      scope: 'extension',
      config: {
        url: new URL('/mcp', settings.serverUrl).href,
        headers: { Authorization: `Bearer ${settings.apiKey}` },
        exposure: 'direct'
      }
    });
  if (settings.web)
    servers.push({
      name: 'exa',
      source: 'botto.toml',
      scope: 'extension',
      config: {
        url: EXA_MCP_URL,
        // A header keeps the key out of URLs, which servers and proxies often log.
        headers: settings.exaApiKey ? { 'x-api-key': settings.exaApiKey } : {},
        exposure: 'direct'
      }
    });
  if (memoryDirectory)
    servers.push({
      name: 'memory',
      source: 'botto.toml',
      scope: 'extension',
      config: {
        command: settings.meansCommand,
        args: ['mcp', '--path', memoryDirectory],
        exposure: 'direct'
      }
    });
  return servers;
}

/** Configure Pi's native MCP extension with Botto's servers. */
export function mcpExtension(
  settings: Settings,
  memoryDirectory?: string,
  createTransport?: McpTransportFactory
): ExtensionFactory {
  const servers = mcpServers(settings, memoryDirectory);
  return createMcpExtension({
    loadConfig: () => ({ servers, errors: [], autoEnableCodemode: false }),
    // Server-generated diagnostic text can contain secrets. Runling observes tool activity.
    logPath: devNull,
    createTransport
  });
}

/** Serialize Means calls across sessions. Runling owns tool activity logging. */
export function memoryLockExtension(
  signal: AbortSignal,
  acquireMemory?: (signal: AbortSignal) => Promise<() => void>
): ExtensionFactory {
  return (pi) => {
    const calls = new Map<string, () => void>();
    pi.on('tool_call', async (event) => {
      if (!event.toolName.startsWith('mcp__memory__') || !acquireMemory) return;
      signal.throwIfAborted();
      const release = await acquireMemory(signal);
      calls.set(event.toolCallId, release);
    });
    const finish = (id: string) => {
      const release = calls.get(id);
      if (!release) return;
      calls.delete(id);
      release();
    };
    pi.on('tool_result', (event) => finish(event.toolCallId));
    // Shutdown follows model cancellation, so any unfinished call releases its lock.
    pi.on('session_shutdown', () => {
      for (const id of calls.keys()) finish(id);
    });
  };
}
