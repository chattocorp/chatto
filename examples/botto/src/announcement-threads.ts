/** Post operator-authorized follow-ups to existing threads without creating channel roots. */
import { createHash } from 'node:crypto';
import type { Server } from '@chatto/client';
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';
import { deliverFollowup, withFollowupMemory, FollowupError } from './followup-memory.ts';
import type { AuthorizedMcpServer } from './mcp-settings.ts';
import { announcementStep } from './diagnostics.ts';

/** The agent selects origins from memory; the host validates the target and returns a receipt.
 * The memory origin ID gives retries the same Chatto idempotency key. No separate store is used. */
export function announcementThreadTools(
  server: Pick<Server, 'getMessage' | 'createMessage'>,
  repository: string,
  signal: AbortSignal,
  followups: { serverUrl: string; memory: AuthorizedMcpServer }
): ExtensionFactory {
  return (pi) => {
    pi.registerTool({
      name: 'reply_to_thread',
      label: 'Notify the original thread',
      description:
        'Post a short issue-resolution update to an existing Chatto thread recorded in memory. Use only after verifying that the recorded issue has a fixing commit included at the immutable after revision of this push; the fix may come from an earlier push. Supply the original issue-origin memory document ID, not a new receipt ID. The text is posted immediately and verbatim: supply the complete final fix notification, never a placeholder, test send, or destination metadata. Keep room and thread IDs in their dedicated arguments. After success, do not send revised wording with the same origin; the host records the receipt. An already-delivered result means no new message was sent. This cannot post a channel root.',
      parameters: Type.Object(
        {
          commitSha: Type.String({
            pattern: '^[a-f0-9]{40}$',
            description: 'Verified fixing commit on the announced branch.'
          }),
          originMemoryId: Type.String({ minLength: 1, maxLength: 100 }),
          roomId: Type.String({ minLength: 1, maxLength: 200 }),
          threadRootId: Type.String({ minLength: 1, maxLength: 200 }),
          text: Type.String({
            minLength: 1,
            maxLength: 1000,
            description:
              'Complete user-facing fix notification, posted immediately. No placeholders or routing metadata.'
          })
        },
        { additionalProperties: false }
      ),
      async execute(_id, { originMemoryId, roomId, threadRootId, text, commitSha }) {
        signal.throwIfAborted();
        if (
          !originMemoryId.trim() ||
          originMemoryId.length > 100 ||
          !roomId.trim() ||
          roomId.length > 200 ||
          !threadRootId.trim() ||
          threadRootId.length > 200 ||
          !text.trim() ||
          text.length > 1000
        )
          throw new Error('Provide a memory origin, existing thread, and short notification.');
        // Reject routing-only output before sending: a bad send would consume the origin's key.
        const body = text.trim();
        const metadataOnly = body
          .split(/\r?\n/)
          .filter((line) => line.trim())
          .every((line) =>
            /^(?:thread_root_id|threadRootId|room_id|roomId|origin_memory_id|originMemoryId)\s*[:=]?\s+\S+$/i.test(
              line.trim()
            )
          );
        if ([threadRootId, roomId, originMemoryId].includes(body) || metadataOnly)
          throw new Error(
            'Nothing was sent. Provide the complete user-facing fix notification in text; keep destination metadata in its dedicated arguments.'
          );
        const result = await withFollowupMemory(followups.memory, signal, (call) =>
          deliverFollowup(
            call,
            { repository, serverUrl: followups.serverUrl },
            { originId: originMemoryId, roomId, threadId: threadRootId, commitSha, body },
            async () => {
              const root = await announcementStep('thread lookup', () =>
                server.getMessage({ roomId, messageId: threadRootId }, { signal })
              );
              if (!root || root.id !== threadRootId || root.roomId !== roomId || root.threadRootId)
                throw new FollowupError(
                  'The recorded thread root is unavailable. Do not choose another destination.'
                );
              signal.throwIfAborted();
              const digest = createHash('sha256')
                .update(
                  JSON.stringify([
                    'issue-followup',
                    repository,
                    originMemoryId,
                    roomId,
                    threadRootId
                  ])
                )
                .digest('hex');
              // Chatto requires a hyphenated UUID. Version 8 carries our deterministic digest.
              const idempotencyKey = `${digest.slice(0, 8)}-${digest.slice(8, 12)}-8${digest.slice(13, 16)}-a${digest.slice(17, 20)}-${digest.slice(20, 32)}`;
              const sent = await announcementStep('thread delivery', () =>
                server.createMessage({ roomId, threadRootId }, body, {
                  signal,
                  idempotencyKey
                })
              );
              return sent.id;
            }
          )
        );
        return {
          content: [{ type: 'text', text: JSON.stringify({ ...result, roomId, threadRootId }) }],
          details: {}
        };
      }
    });
  };
}
