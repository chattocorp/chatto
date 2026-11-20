/** Post operator-authorized follow-ups to existing threads without creating channel roots. */
import { createHash } from 'node:crypto';
import type { Server } from '@chatto/client';
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';
import { announcementStep } from './diagnostics.ts';

/** The agent selects origins from memory; the host validates the target and returns a receipt.
 * The memory origin ID gives retries the same Chatto idempotency key. No separate store is used. */
export function announcementThreadTools(
  server: Pick<Server, 'getMessage' | 'createMessage'>,
  repository: string,
  signal: AbortSignal
): ExtensionFactory {
  return (pi) => {
    pi.registerTool({
      name: 'reply_to_thread',
      label: 'Notify the original thread',
      description:
        'Post a short issue-resolution update to an existing Chatto thread recorded in memory. Use only after verifying that the recorded issue has a fixing commit included at the immutable after revision of this push; the fix may come from an earlier push. Supply the original issue-origin memory document ID, not a new receipt ID. Record the returned message ID in a new memory receipt after success. This cannot post a channel root.',
      parameters: Type.Object(
        {
          originMemoryId: Type.String({ minLength: 1, maxLength: 100 }),
          roomId: Type.String({ minLength: 1, maxLength: 200 }),
          threadRootId: Type.String({ minLength: 1, maxLength: 200 }),
          text: Type.String({ minLength: 1, maxLength: 1000 })
        },
        { additionalProperties: false }
      ),
      async execute(_id, { originMemoryId, roomId, threadRootId, text }) {
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
        const root = await announcementStep('thread lookup', () =>
          server.getMessage({ roomId, messageId: threadRootId }, { signal })
        );
        if (!root || root.id !== threadRootId || root.roomId !== roomId || root.threadRootId)
          throw new Error(
            'The recorded thread root is unavailable. Do not choose another destination.'
          );
        signal.throwIfAborted();
        const digest = createHash('sha256')
          .update(
            JSON.stringify(['issue-followup', repository, originMemoryId, roomId, threadRootId])
          )
          .digest('hex');
        // Chatto requires a hyphenated UUID. Version 8 carries our deterministic digest.
        const idempotencyKey = `${digest.slice(0, 8)}-${digest.slice(8, 12)}-8${digest.slice(13, 16)}-a${digest.slice(17, 20)}-${digest.slice(20, 32)}`;
        const sent = await announcementStep('thread delivery', () =>
          server.createMessage({ roomId, threadRootId }, text.trim(), {
            signal,
            idempotencyKey
          })
        );
        return {
          content: [
            { type: 'text', text: JSON.stringify({ messageId: sent.id, roomId, threadRootId }) }
          ],
          details: {}
        };
      }
    });
  };
}
