/** Check the model reaction tool's scope, cancellation, and safe failure behavior. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import { afterEach, expect, test, vi } from 'vitest';
import { reactionExtension } from './reactions.ts';

afterEach(() => vi.restoreAllMocks());

test('only reacts to known messages and does not expose server errors', async () => {
  vi.spyOn(console, 'info').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
  const server = { addReaction: vi.fn().mockResolvedValue(undefined) };
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  const controller = new AbortController();
  await reactionExtension(
    server,
    'room',
    'thread',
    new Set(['known']),
    controller.signal
  )({ registerTool } as unknown as ExtensionAPI);
  const tool = registerTool.mock.calls[0]![0];
  const call = (messageId: string) =>
    tool.execute(
      'call',
      { messageId, emoji: 'heart' },
      undefined,
      undefined,
      {} as ExtensionToolContext
    );
  await call('known');
  expect(server.addReaction).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', messageId: 'known' },
    'heart',
    { signal: expect.any(AbortSignal) }
  );
  await expect(call('outside')).rejects.toThrow('outside the current context');
  expect(server.addReaction).toHaveBeenCalledOnce();
  server.addReaction.mockRejectedValueOnce(new Error('private server details'));
  await expect(call('known')).rejects.toThrow(
    'Reaction could not be confirmed. Do not retry this write.'
  );
  controller.abort();
  await expect(call('known')).rejects.toThrow();
  expect(server.addReaction).toHaveBeenCalledTimes(2);
});
