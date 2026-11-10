/** Agent-selected final responses; Chatto effects remain owned by the turn handler. */
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';

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
          'End this turn by requesting a reaction to the latest incoming message instead of text. Call alone without accompanying text. Do not duplicate the host acknowledgement.',
        parameters: Type.Object(
          { emoji: Type.String({ minLength: 1, maxLength: 100, pattern: '^[a-zA-Z0-9_+\\-]+$' }) },
          { additionalProperties: false }
        ),
        async execute(_id, { emoji }) {
          return finish({ action: 'react', emoji });
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
