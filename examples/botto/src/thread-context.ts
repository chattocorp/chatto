/** Supply request-only context through Pi's hook, without changing its transcript. */
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { mergeThreadContext, type ThreadContext } from './context-messages.ts';
export type { ThreadContext } from './context-messages.ts';

/** Pi supplies a request copy on each call, including retries and compacted turns. */
export function threadContextExtension(snapshot: ThreadContext): ExtensionFactory {
  return (pi) => {
    pi.on('context', (event, ctx) => ({
      messages: mergeThreadContext(
        event.messages,
        snapshot.images?.length && !ctx.model?.input.includes('image')
          ? {
              ...snapshot,
              images: [],
              imageNotes: [
                ...(snapshot.imageNotes ?? []),
                'The selected model cannot read images. Tell the user; do not infer image contents.'
              ]
            }
          : snapshot
      )
    }));
  };
}
