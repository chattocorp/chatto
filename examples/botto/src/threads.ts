/** Botto's thread keys and persisted transcript identity. Keep the tuple order stable. */
import type { ThreadLocation } from '@chatto/client';

/** Room-scoped key for batching and active-thread checks within one bot account. */
export function threadKey({ roomId, threadRootId }: ThreadLocation): string {
  return JSON.stringify([roomId, threadRootId]);
}

/** Account-scoped identity shared by workflow routing and existing transcript filenames. */
export function conversationIdentity(
  serverUrl: string,
  botId: string,
  { roomId, threadRootId }: ThreadLocation
): readonly string[] {
  return [new URL(serverUrl).origin, botId, roomId, threadRootId];
}
