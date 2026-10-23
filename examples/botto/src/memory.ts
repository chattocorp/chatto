/** Provision Means documents; Pi's native MCP extension owns model-facing tools. */
import { createHash } from 'node:crypto';
import { constants } from 'node:fs';
import { copyFile, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { McpClient, StdioTransport } from '@earendil-works/pi-mcp';
import { untilAborted } from '@chatto/client/util/untilAborted';
import type { Settings } from './settings.ts';
import { logged } from './log.ts';

/** One owner per bot process. Provisioning and native tool hooks share this lock. */
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
  return async (identity: readonly string[], userIds: readonly string[], signal: AbortSignal) => {
    const release = await acquire(signal);
    const client = new McpClient({ name: 'botto-memory-setup', version: '0.1.0' });
    const abort = () => {
      void client.close().catch(() => {});
    };
    signal.addEventListener('abort', abort, { once: true });
    try {
      const key = createHash('sha256')
        .update(JSON.stringify(identity.slice(0, 2)))
        .digest('hex');
      const directory = join(settings.directory, 'memory', key);
      await mkdir(directory, { recursive: true, mode: 0o700 });
      for (const file of ['means.schema.json', 'MEANS.md']) {
        try {
          await copyFile(
            new URL(`../memory/${file}`, import.meta.url),
            join(directory, file),
            constants.COPYFILE_EXCL
          );
        } catch (error) {
          if (!(error instanceof Error && 'code' in error && error.code === 'EEXIST')) throw error;
        }
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
      const global = await ensureDocument(client, { scope: 'global' }, signal, identity[3]);
      const users: Record<string, string> = Object.create(null);
      for (const userId of new Set(userIds)) {
        users[userId] = await ensureDocument(
          client,
          { scope: 'user', user_id: userId },
          signal,
          identity[3]
        );
      }
      return {
        directory,
        documents: { global, users },
        instructions: client.instructions,
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

/** Reuse persisted IDs; never create a second document for an existing memory scope. */
async function ensureDocument(
  client: McpClient,
  data: { scope: string; user_id?: string },
  signal: AbortSignal,
  threadId?: string
): Promise<string> {
  const call = async (name: string, args: Record<string, unknown>) =>
    logged(
      'MCP tool',
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
  if (found.documents.length > 1) throw new Error('Means memory has duplicate scopes');
  const document =
    found.documents[0] ??
    (await call('create', {
      frontmatter: {
        title: data.scope === 'global' ? 'Global memory' : `User ${data.user_id}`,
        data
      },
      body: ''
    }));
  if (
    !document ||
    typeof document !== 'object' ||
    !('id' in document) ||
    typeof document.id !== 'string'
  )
    throw new Error('Means returned an invalid document');
  return document.id;
}
