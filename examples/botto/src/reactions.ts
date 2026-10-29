/** Expose reactions only for messages supplied to the current model turn. */
import type { Server } from '@chatto/client';
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';
import { safeStep } from './steps.ts';

/** Keep room selection host-owned and reject message IDs outside the turn's context. */
export function reactionExtension(
  server: Pick<Server, 'addReaction'>,
  roomId: string,
  threadId: string,
  messageIds: ReadonlySet<string>,
  turnSignal: AbortSignal
): ExtensionFactory {
  return (pi) => {
    pi.registerTool({
      name: 'add_reaction',
      label: 'React to a message',
      description:
        'Add an emoji reaction to a message in the current conversation context. A reaction can be the whole response; do not add text just to explain it. Do not duplicate the host acknowledgement.',
      parameters: Type.Object({
        messageId: Type.String({ description: 'Message ID from the current context.' }),
        emoji: Type.String({
          minLength: 1,
          description: 'Emoji shortcode, such as +1, heart, or joy.'
        })
      }),
      async execute(_id, { messageId, emoji }, signal) {
        if (!messageIds.has(messageId)) throw new Error('Message is outside the current context.');
        const requestSignal = AbortSignal.any([turnSignal, ...(signal ? [signal] : [])]);
        requestSignal.throwIfAborted();
        try {
          await safeStep(
            'Reaction tool',
            () => server.addReaction({ roomId, messageId }, emoji, { signal: requestSignal }),
            { threadId }
          );
        } catch {
          throw new Error('Reaction could not be confirmed. Do not retry this write.');
        }
        return { content: [{ type: 'text', text: 'Reaction added.' }], details: {} };
      }
    });
  };
}
