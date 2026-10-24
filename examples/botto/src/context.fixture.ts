/** In-memory thread read for Botto tests. */
import { vi } from 'vitest';
import type { Server, ThreadMessage } from '@chatto/client';

export function row(id: string, body: string, authorId = 'human'): ThreadMessage {
  return { id, authorId, body, fromViewer: authorId === 'bot' };
}

/** Each read returns the current `thread` contents. */
export function contextFixture() {
  const thread = {
    messages: [
      row('root', 'Root'),
      row('ordinary', 'Useful unaddressed context'),
      row('one', 'First'),
      row('two', 'Clarification', 'another')
    ],
    olderOmitted: false
  };
  const readThread = vi
    .fn<Server['readThread']>()
    .mockImplementation(async () => ({ ...thread, messages: [...thread.messages] }));
  return { thread, readThread, server: { readThread } };
}
