import type { Interceptor } from '@connectrpc/connect';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { presentableDisplayName } from '../util/displayName.js';

/**
 * Replace every user display name in a decoded API message, in place, with
 * {@link presentableDisplayName}. Current servers apply the same rule when
 * they read a user. Older servers return stored names unchanged, so the client
 * applies it again.
 * Protobuf messages are trees, so the walk ends.
 */
export function applyPresentableDisplayNames(value: unknown): void {
  if (value === null || typeof value !== 'object' || ArrayBuffer.isView(value)) return;
  if (value instanceof User) {
    value.displayName = presentableDisplayName(value.displayName, value.login);
  }
  // Repeated fields are arrays; maps and oneof cases are plain objects.
  for (const child of Object.values(value)) applyPresentableDisplayNames(child);
}

async function* presentableStream<T>(messages: AsyncIterable<T>): AsyncIterable<T> {
  for await (const message of messages) {
    applyPresentableDisplayNames(message);
    yield message;
  }
}

/** Apply {@link applyPresentableDisplayNames} to every response message. */
export const presentableDisplayNamesInterceptor: Interceptor = (next) => async (request) => {
  const response = await next(request);
  if (response.stream) return { ...response, message: presentableStream(response.message) };
  applyPresentableDisplayNames(response.message);
  return response;
};
