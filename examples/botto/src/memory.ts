/** Find Means memories; Pi's native MCP extension owns model-facing tools. */
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
  return async (threadId: string | undefined, userIds: readonly string[], signal: AbortSignal) => {
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
      const global = await findDocuments(client, { scope: 'global' }, signal, threadId);
      const users: Record<string, string[]> = Object.create(null);
      for (const userId of new Set(userIds)) {
        users[userId] = await findDocuments(
          client,
          { scope: 'user', user_id: userId },
          signal,
          threadId
        );
      }
      return {
        directory,
        documents: { global, users },
        acquire
      };
    } finally {
      signal.removeEventListener('abort', abort);
      try {
        await client.close();
      } finally {
        release();
      }
    }
  };
}

/** Find all documents in a scope, including legacy files with several memories. */
async function findDocuments(
  client: McpClient,
  data: { scope: string; user_id?: string },
  signal: AbortSignal,
  threadId?: string
): Promise<string[]> {
  const call = async (name: string, args: Record<string, unknown>) =>
    safeStep(
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
  const found = await call('search', { filter: data, limit: 0 });
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
