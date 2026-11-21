/** Agent-selected final responses; Chatto effects remain owned by the turn handler. */
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';
import { emojiNameFromUnicode, resolveEmoji } from '@chatto/client/messaging/emojiShortcodes';

/** Silence and reactions are successful decisions, not empty or failed model output. */
export type ConversationResponse =
  { action: 'reply'; text: string } | { action: 'react'; emoji: string } | { action: 'silent' };

/** Terminal tools end Pi's loop without another model request or cancellation. */
export function responseTools(
  signal: AbortSignal,
  allowReaction: boolean,
  postUpdate?: (text: string) => void | Promise<void>
) {
  let decision: Exclude<ConversationResponse, { action: 'reply' }> | undefined;
  // A reaction is a complete response choice, not the start of a progress-reporting workflow.
  let reactionAttempted = false;
  const finish = (value: NonNullable<typeof decision>) => {
    signal.throwIfAborted();
    if (decision) throw new Error('A final response was already selected.');
    decision = value;
    return {
      content: [
        {
          type: 'text' as const,
          text:
            value.action === 'silent'
              ? 'Turn ended without a final reply. Earlier tool effects remain.'
              : 'Reaction requested. The host will attempt delivery after this turn; delivery is not yet confirmed.'
        }
      ],
      details: {},
      terminate: true
    };
  };
  const extension: ExtensionFactory = (pi) => {
    // Sequential execution prevents another tool in the batch from acting after a terminal choice.
    pi.on('tool_call', () =>
      decision
        ? {
            block: true,
            reason: 'The turn has ended; do not execute further tools.',
            terminate: true
          }
        : undefined
    );
    pi.registerTool({
      name: 'stay_silent',
      label: 'Stay silent',
      description:
        'End this turn without a final reply or reaction. Use when participation adds nothing. Call alone without accompanying text. Earlier tool effects remain.',
      parameters: Type.Object({}, { additionalProperties: false }),
      async execute() {
        return finish({ action: 'silent' });
      }
    });
    if (allowReaction)
      pi.registerTool({
        name: 'react_to_message',
        label: 'Choose an emoji reaction',
        description:
          'End this turn by requesting a reaction to the latest incoming message instead of text. Call alone without accompanying text. Accepts one Unicode emoji (👏) or a built-in name (clap, thumbsup, heart), optionally wrapped in colons (:clap:). Do not duplicate the host acknowledgement.',
        parameters: Type.Object(
          {
            emoji: Type.String({
              minLength: 1,
              maxLength: 100,
              description: 'One Unicode emoji or a built-in reaction name, for example 👏 or clap.'
            })
          },
          { additionalProperties: false }
        ),
        async execute(_id, { emoji }) {
          signal.throwIfAborted();
          reactionAttempted = true;
          const input = emoji.trim();
          const name =
            emojiNameFromUnicode(input) ??
            (input.startsWith(':') && input.endsWith(':') ? input.slice(1, -1) : input);
          if (resolveEmoji(name).kind !== 'unicode')
            throw new Error(
              'Use one supported emoji or built-in name, such as 👏 or clap. Correct the reaction, choose silence, or give one final reply; do not post a progress update.'
            );
          return finish({ action: 'react', emoji: name });
        }
      });
    if (postUpdate)
      pi.registerTool({
        name: 'post_update',
        label: 'Post progress update',
        description:
          'Post a short update to this thread before substantial work or when there is meaningful progress. Continue work afterward. Do not use for simple answers or narrate tool calls.',
        parameters: Type.Object(
          { text: Type.String({ minLength: 1, maxLength: 1000 }) },
          { additionalProperties: false }
        ),
        async execute(_id, { text }) {
          signal.throwIfAborted();
          if (decision) throw new Error('A final response was already selected.');
          if (reactionAttempted)
            throw new Error(
              'A reaction attempt cannot become a progress update. Correct the reaction, choose silence, or give one final reply.'
            );
          if (!text.trim()) throw new Error('Provide a non-empty progress update.');
          await postUpdate(text.trim());
          return { content: [{ type: 'text', text: 'Progress update delivered.' }], details: {} };
        }
      });
  };
  return {
    extension,
    result(text: string): ConversationResponse {
      signal.throwIfAborted();
      if (decision) return decision;
      if (!text.trim())
        throw new Error('Agent ended without a reply or explicit response decision.');
      return { action: 'reply', text: text.trim() };
    }
  };
}
