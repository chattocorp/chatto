/** Load initial Means memories; Pi's native MCP extension owns later lookups and writes. */
import { constants } from 'node:fs';
import { access } from 'node:fs/promises';
import { join } from 'node:path';
import { McpClient, StdioTransport } from '@earendil-works/pi-mcp';
import { untilAborted } from './abort.ts';
import type { Settings } from './settings.ts';
import { safeStep } from './steps.ts';

/** One owner per bot process. Discovery and native tool hooks share this lock. */
export function createMemory(settings: Settings) {
  let pending = Promise.resolve();
  const acquire = async (signal: AbortSignal): Promise<() => void> => {
    signal.throwIfAborted();
    const previous = pending;
    const gate = Promise.withResolvers<void>();
    pending = previous.then(() => gate.promise);
    try {
      await untilAborted(previous, signal);
      signal.throwIfAborted();
      return () => gate.resolve();
    } catch (error) {
      gate.resolve();
      throw error;
    }
  };
  const initialize = async (
    threadId: string | undefined,
    userId: string | undefined,
    signal: AbortSignal
  ) => {
    const release = await acquire(signal);
    const client = new McpClient({ name: 'botto-memory-setup', version: '0.1.0' });
    const abort = () => {
      void client.close().catch(() => {});
    };
    signal.addEventListener('abort', abort, { once: true });
    try {
      const directory = settings.memoryPath;
      try {
        await access(join(directory, 'MEANS.md'), constants.R_OK);
      } catch {
        throw new Error(
          'Cannot read MEANS.md in memory.path. Copy the collection configuration first.'
        );
      }
      signal.throwIfAborted();
      await untilAborted(
        client.connect(
          new StdioTransport({
            command: settings.meansCommand,
            args: ['mcp', '--path', directory],
            stderr: 'pipe'
          })
        ),
        signal
      );
      const documents: Array<{ id: string; scope: string; user_id?: string; body: string }> = [];
      let remaining = 24000;
      let truncated = false;
      const scopes = [{ scope: 'global' }, ...(userId ? [{ scope: 'user', user_id: userId }] : [])];
      for (const scope of scopes) {
        const ids = await findDocuments(client, scope, signal, threadId);
        for (const id of ids) {
          // Bound both text and document overhead; the agent can retrieve the rest later.
          if (remaining === 0 || documents.length === 32) {
            truncated = true;
            break;
          }
          const document = await callMemory(client, 'read', { id }, signal, threadId);
          if (typeof document.body !== 'string')
            throw new Error('Means returned an invalid document');
          const body = document.body.slice(0, remaining);
          truncated ||= body.length < document.body.length;
          remaining -= body.length;
          documents.push({ id, ...scope, body });
        }
      }
      return { documents, truncated };
    } finally {
      signal.removeEventListener('abort', abort);
      try {
        await client.close();
      } finally {
        release();
      }
    }
  };
  return { directory: settings.memoryPath, acquire, initialize };
}

/** Record operation names only; memory contents must not enter run logs. */
async function callMemory(
  client: McpClient,
  name: string,
  args: Record<string, unknown>,
  signal: AbortSignal,
  threadId?: string
): Promise<Record<string, unknown>> {
  return safeStep(
    `mcp__memory__${name}`,
    async () => {
      const result = await client.callTool(name, args, { signal });
      if (
        result.isError ||
        !result.structuredContent ||
        typeof result.structuredContent !== 'object' ||
        Array.isArray(result.structuredContent)
      )
        throw new Error('Means memory operation failed');
      return result.structuredContent as Record<string, unknown>;
    },
    { threadId, tool: `mcp__memory__${name}` }
  );
}

/** Find all documents in a scope, including legacy files with several memories. */
async function findDocuments(
  client: McpClient,
  data: { scope: string; user_id?: string },
  signal: AbortSignal,
  threadId?: string
): Promise<string[]> {
  const found = await callMemory(client, 'search', { filter: data, limit: 0 }, signal, threadId);
  if (
    !Array.isArray(found.documents) ||
    !Array.isArray(found.diagnostics) ||
    found.diagnostics.length
  )
    throw new Error('Means memory has invalid documents');
  return found.documents.map((document: unknown) => {
    if (
      !document ||
      typeof document !== 'object' ||
      !('id' in document) ||
      typeof document.id !== 'string'
    )
      throw new Error('Means returned an invalid document');
    return document.id;
  });
}
