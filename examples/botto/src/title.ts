/** Let the root agent name its conversation without changing routing or posting a message. */
import type { TaskContext } from 'runling';
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { Type } from 'typebox';

/** Only the root session installs this tool; delegated sessions do not receive it. */
export function titleExtension(ctx: TaskContext, turnSignal: AbortSignal): ExtensionFactory {
  return (pi) => {
    if (!ctx.run) return;
    pi.registerTool({
      name: 'set_conversation_title',
      label: 'Set conversation title',
      description:
        'Set a short descriptive title for this conversation in the Runling console. Use the overall topic, not a status update. Avoid personal details and secrets.',
      parameters: Type.Object({ title: Type.String({ minLength: 1, maxLength: 120 }) }),
      async execute(_id, { title }, signal) {
        turnSignal.throwIfAborted();
        signal?.throwIfAborted();
        ctx.run!.setTitle(title);
        return { content: [{ type: 'text', text: 'Conversation title updated.' }], details: {} };
      }
    });
  };
}
