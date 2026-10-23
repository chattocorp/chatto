/** Shared identity check for ChattoBot delegation. Task scope and limits stay with each tool. */
import { log } from 'runling';
import type { IncomingMessage } from '../chatto/message.ts';

/** Return a current, addressed maintainer request or log a denial and throw. Only host-owned
 * messages and maintainer IDs establish identity; model-supplied IDs never enter denial logs. */
export function requireMaintainerRequest(
  capability: 'GitHub' | 'Investigation' | 'Paseo',
  requestId: string,
  messages: readonly IncomingMessage[],
  maintainers: ReadonlySet<string>
): IncomingMessage {
  const request = messages.find((message) => message.id === requestId);
  const reason = !request
    ? 'no-current-message'
    : request.trigger
      ? 'not-addressed'
      : !maintainers.has(request.authorId)
        ? 'not-maintainer'
        : undefined;
  if (!request || reason) {
    log.info(
      `${capability} authorization denied: request=${request?.id ?? 'not-current'}; reason=${reason}`
    );
    throw new Error(
      `${capability} requires a message from a maintainer in this turn, addressed to the bot.`
    );
  }
  return request;
}
