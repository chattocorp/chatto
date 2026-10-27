/** Supply request-only context through Pi's hook, without changing its transcript. */
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { mergeThreadContext, type ThreadContext } from './context-messages.ts';
import type { readImages } from './images.ts';
export type { ThreadContext } from './context-messages.ts';

/** Pi supplies a request copy and the active model. Load images once per session, only for vision. */
export function threadContextExtension(
  snapshot: ThreadContext,
  loadImages?: () => ReturnType<typeof readImages>
): ExtensionFactory {
  return (pi) => {
    let loaded: ReturnType<typeof readImages> | undefined;
    pi.on('context', async (event, ctx) => {
      let context = snapshot;
      if (!ctx.model?.input.includes('image')) {
        context = {
          ...snapshot,
          images: [],
          imageNotes: [
            ...(snapshot.imageNotes ?? []),
            'The selected model cannot read images. Tell the user if images are relevant; do not infer image contents.'
          ]
        };
      } else if (loadImages) {
        const images = await (loaded ??= loadImages());
        context = {
          ...snapshot,
          images: images.images,
          imageNotes: [...(snapshot.imageNotes ?? []), ...images.imageNotes]
        };
      }
      return { messages: mergeThreadContext(event.messages, context) };
    });
  };
}
