/** Load bounded image context through Chatto's authorized, same-origin attachment reader. */
import type { Server } from '@chatto/client';
import type { ThreadContext } from './context-messages.ts';

const MAX_IMAGES = 4;
const MAX_IMAGE_BYTES = 4 * 1024 * 1024;
const IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif']);

/** Prioritize the current batch, then recent thread images. Bytes exist for this turn only.
 * Failed downloads are reported to the model without exposing URLs or external errors. */
export async function readImages(
  server: Pick<Server, 'readAttachment' | 'getMessage'>,
  roomId: string,
  context: ThreadContext,
  currentIds: readonly string[],
  signal: AbortSignal
): Promise<ThreadContext> {
  const current = new Set(currentIds);
  // A debounced batch can exceed the recent context window. Read missing batch members
  // through the client so their images still have priority and current authorization.
  const additional: ThreadContext['messages'] = [];
  const imageNotes: string[] = [];
  for (const id of currentIds) {
    if (context.messages.some((message) => message.id === id)) continue;
    signal.throwIfAborted();
    try {
      const message = await server.getMessage({ roomId, messageId: id }, { signal });
      signal.throwIfAborted();
      if (!message) throw new Error('Message unavailable');
      additional.push({
        id,
        author: message.authorId,
        text: message.body ?? '',
        attachments: message.attachments
      });
    } catch {
      signal.throwIfAborted();
      imageNotes.push(
        `Images for message ${id} could not be checked. Do not infer their contents.`
      );
    }
  }
  context = { ...context, messages: [...additional, ...context.messages] };
  const messages = [
    ...context.messages.filter((message) => current.has(message.id)),
    ...context.messages.filter((message) => !current.has(message.id)).reverse()
  ];
  const images: NonNullable<ThreadContext['images']> = [];
  let attempted = 0;
  const seen = new Set<string>();
  for (const message of messages) {
    for (const attachment of message.attachments ?? []) {
      if (!attachment.contentType.startsWith('image/') || seen.has(attachment.id)) continue;
      seen.add(attachment.id);
      signal.throwIfAborted();
      if (attempted++ >= MAX_IMAGES) {
        imageNotes.push(
          `Image ${attachment.id} in message ${message.id} omitted: four-image limit.`
        );
        continue;
      }
      try {
        const image = await server.readAttachment(
          { roomId, attachmentId: attachment.id },
          {
            signal,
            maxImageSize: 2048,
            maxBytes: MAX_IMAGE_BYTES
          }
        );
        signal.throwIfAborted();
        if (!IMAGE_TYPES.has(image.contentType)) {
          imageNotes.push(
            `Image ${attachment.id} in message ${message.id} has an unsupported format.`
          );
          continue;
        }
        images.push({
          messageId: message.id,
          attachmentId: attachment.id,
          mimeType: image.contentType,
          data: Buffer.from(image.data).toString('base64')
        });
      } catch {
        signal.throwIfAborted();
        imageNotes.push(
          `Image ${attachment.id} in message ${message.id} could not be read. Do not infer its contents.`
        );
      }
    }
  }
  return {
    ...context,
    ...(images.length ? { images } : {}),
    ...(imageNotes.length ? { imageNotes } : {})
  };
}
