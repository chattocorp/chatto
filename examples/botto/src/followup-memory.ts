/** Means owns follow-up records; the host enforces delivery state through its MCP API.
 * One Botto process owns this workflow. A persisted sending state blocks uncertain retries. */
import { dirname, resolve } from 'node:path';
import { McpClient, StdioTransport, StreamableHttpTransport } from '@earendil-works/pi-mcp';
import { AnnouncementError, announcementError } from './diagnostics.ts';
import type { AuthorizedMcpServer } from './mcp-settings.ts';

/** Fixed operator-safe validation errors; never construct with remote response text. */
export class FollowupError extends Error {}

export type MeansCall = (
  name: string,
  args: Record<string, unknown>
) => Promise<Record<string, unknown>>;

function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new FollowupError('Invalid follow-up record.');
  return value as Record<string, unknown>;
}
function text(value: unknown): string {
  if (typeof value !== 'string' || !value.trim())
    throw new FollowupError('Incomplete follow-up record.');
  return value;
}
function data(doc: Record<string, unknown>): Record<string, unknown> {
  return object(object(doc.frontmatter).data);
}
async function search(call: MeansCall, filter: Record<string, unknown>) {
  const result = await call('search', { filter, limit: 0 });
  if (
    result.has_more !== false ||
    !Array.isArray(result.documents) ||
    !Array.isArray(result.diagnostics) ||
    result.diagnostics.length
  )
    throw new FollowupError('Incomplete follow-up lookup; no message was sent.');
  return result.documents.map(object);
}

/** Validate the origin, suppress prior receipts, and persist intent before the external write.
 * No automatic retry is safe once intent exists without a confirmed delivered state. */
export async function deliverFollowup(
  call: MeansCall,
  scope: { repository: string; serverUrl: string },
  input: { originId: string; roomId: string; threadId: string; commitSha: string; body: string },
  send: () => Promise<string>
): Promise<{ messageId: string; alreadyDelivered: boolean }> {
  if (!/^[a-f0-9]{40}$/.test(input.commitSha))
    throw new FollowupError('Provide the verified fixing commit SHA.');
  const origin = await call('read', { id: input.originId });
  const info = object(data(origin).issue_origin);
  if (
    info.repository !== scope.repository ||
    info.server_url !== scope.serverUrl ||
    info.room_id !== input.roomId ||
    info.thread_id !== input.threadId ||
    !Number.isSafeInteger(info.issue_number) ||
    Number(info.issue_number) < 1
  )
    throw new FollowupError(
      'Follow-up origin does not match the configured repository and destination.'
    );
  // Old status notes can repeat issue_origin. Check all aliases, not only the model-selected ID.
  const aliases = await search(call, { issue_origin: info });
  if (!aliases.some((doc) => doc.id === input.originId))
    throw new FollowupError('Follow-up origin lookup is incomplete.');
  let pending = false;
  for (const alias of aliases) {
    const id = text(alias.id);
    const full = id === input.originId ? origin : await call('read', { id });
    const state = data(full).delivery;
    if (state !== undefined) {
      const delivery = object(state);
      if (delivery.status === 'delivered')
        return { messageId: text(delivery.message_id), alreadyDelivered: true };
      if (delivery.status !== 'sending')
        throw new FollowupError('Invalid follow-up delivery state.');
      pending = true;
    }
    for (const receipt of await search(call, { issue_notification: { origin_id: id } })) {
      const fullReceipt = await call('read', { id: text(receipt.id) });
      const notification = object(data(fullReceipt).issue_notification);
      if (notification.origin_id !== id) throw new FollowupError('Invalid follow-up receipt.');
      return { messageId: text(notification.message_id), alreadyDelivered: true };
    }
  }
  if (pending)
    throw new FollowupError('Follow-up delivery needs operator review; no message was sent.');
  const delivery = {
    status: 'sending',
    commit_sha: input.commitSha,
    body: input.body,
    started_at: new Date().toISOString()
  };
  const intent = await call('update', {
    id: input.originId,
    hash: text(origin.hash),
    frontmatter: { data: { delivery } }
  });
  const hash = text(intent.hash);
  const messageId = text(await send());
  // If this fails, the persisted intent remains held. Never turn an uncertain send into a retry.
  await call('update', {
    id: input.originId,
    hash,
    frontmatter: {
      data: {
        delivery: { ...delivery, status: 'delivered', message_id: messageId },
        issue_notification: {
          origin_id: input.originId,
          commit_sha: input.commitSha,
          message_id: messageId
        }
      }
    }
  });
  return { messageId, alreadyDelivered: false };
}

// Serialize host sends across sessions. Means still uses hashes to detect concurrent edits.
let tail: Promise<unknown> = Promise.resolve();
/** Open only the operator-selected connection. Do not execute Pi configuration expressions. */
export async function withFollowupMemory<T>(
  entry: AuthorizedMcpServer,
  signal: AbortSignal,
  work: (call: MeansCall) => Promise<T>
): Promise<T> {
  const run = async () => {
    signal.throwIfAborted();
    // parseMcpServers escapes literals for Pi. Undo only that escaping, never expand commands.
    const literal = (v: string) => v.replace(/^\$!/, '!').replaceAll('$$', '$');
    const values = (v: Record<string, string> | undefined) =>
      Object.fromEntries(Object.entries(v ?? {}).map(([k, value]) => [k, literal(value)]));
    const config = entry.config;
    const transport =
      'url' in config
        ? new StreamableHttpTransport({
            url: literal(config.url),
            headers: values(config.headers),
            openGetStream: false
          })
        : new StdioTransport({
            command: literal(config.command!),
            args: config.args?.map(literal),
            env: values(config.env),
            inheritEnv: true,
            cwd: dirname(resolve(entry.source)),
            stderr: 'pipe'
          });
    const client = new McpClient({
      name: 'botto-followups',
      version: '1',
      requestTimeoutMs: 30_000
    });
    const abort = () => {
      void client.close();
    };
    signal.addEventListener('abort', abort, { once: true });
    try {
      signal.throwIfAborted();
      await client.connect(transport);
      return await work(async (name, args) => {
        signal.throwIfAborted();
        const result = await client.callTool(name, args, { signal });
        if (result.isError) throw new FollowupError('Follow-up memory operation failed.');
        if (result.structuredContent) return object(result.structuredContent);
        const blocks = result.content.filter((block) => block.type === 'text');
        if (blocks.length !== 1) throw new FollowupError('Invalid follow-up memory response.');
        return object(JSON.parse(blocks[0]!.text));
      });
    } catch (error) {
      if (error instanceof FollowupError || error instanceof AnnouncementError) throw error;
      throw announcementError('follow-up memory', error);
    } finally {
      signal.removeEventListener('abort', abort);
      await client.close();
    }
  };
  const result = tail.then(run, run);
  tail = result.catch(() => undefined);
  return result;
}
