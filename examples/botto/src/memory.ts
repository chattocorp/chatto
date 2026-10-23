/** Own a private Means collection and serialize its MCP mutations within this process. */
import { constants } from 'node:fs';
import { copyFile, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js';
import type { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { callMcpTool, connectMcp } from './mcp.ts';
import type { Settings } from './settings.ts';
import { createHash } from 'node:crypto';
import { untilAborted } from '@chatto/client/util/untilAborted';

/** One memory owner per bot process. No subprocess is started until a turn needs it. */
export function createMemory(settings: Settings) {
  let pending: Promise<unknown> = Promise.resolve();
  // Means has no write lock. Serialize calls and whole find-or-create operations;
  // stale read/update pairs still use Means hashes to detect intervening changes.
  const run = <T>(work: () => Promise<T>): Promise<T> => {
    const result = pending.then(work);
    pending = result.catch(() => {});
    return result;
  };
  return async (identity: readonly string[], userIds: readonly string[], signal: AbortSignal) => {
    signal.throwIfAborted();
    const queued = <T>(work: () => Promise<T>): Promise<T> =>
      untilAborted(
        run(() => {
          signal.throwIfAborted();
          return work();
        }),
        signal
      );
    const key = createHash('sha256')
      .update(JSON.stringify(identity.slice(0, 2)))
      .digest('hex');
    const directory = join(settings.directory, 'memory', key);
    // Another turn must not start Means while template files are still being copied.
    await queued(async () => {
      signal.throwIfAborted();
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
    });
    const connection = await connectMcp(
      new StdioClientTransport({
        command: settings.meansCommand,
        args: ['mcp', '--path', directory],
        stderr: 'ignore'
      }),
      signal,
      { prefix: 'memory_', run: queued, threadId: identity[3] }
    );
    try {
      const documents = await queued(async () => {
        const global = await ensureDocument(
          connection.client,
          { scope: 'global' },
          signal,
          identity[3]
        );
        const users: Record<string, string> = Object.create(null);
        for (const userId of new Set(userIds)) {
          users[userId] = await ensureDocument(
            connection.client,
            { scope: 'user', user_id: userId },
            signal,
            identity[3]
          );
        }
        return { global, users };
      });
      return { ...connection, documents };
    } catch (error) {
      await connection.close();
      throw error;
    }
  };
}

/** Host-owned document discovery makes repeated turns and restarts reuse the same IDs. */
async function ensureDocument(
  client: Client,
  data: { scope: string; user_id?: string },
  signal: AbortSignal,
  threadId?: string
): Promise<string> {
  const call = async (name: string, args: Record<string, unknown>) => {
    const result = await callMcpTool(client, name, args, signal, {
      threadId,
      tool: `memory_${name}`
    });
    if (
      result.isError ||
      !result.structuredContent ||
      typeof result.structuredContent !== 'object' ||
      Array.isArray(result.structuredContent)
    )
      throw new Error('Means memory operation failed');
    return result.structuredContent as Record<string, unknown>;
  };
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
