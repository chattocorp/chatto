/** Verify follow-ups cannot create channel roots and return receipts only after successful sends. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import type { Server } from '@chatto/client';
import { expect, test, vi } from 'vitest';
import { announcementThreadTools } from './announcement-threads.ts';

async function setup() {
  const controller = new AbortController();
  const getMessage = vi.fn<Server['getMessage']>().mockResolvedValue({
    id: 'root',
    roomId: 'room',
    authorId: 'reporter',
    body: 'Report'
  });
  const createMessage = vi.fn<Server['createMessage']>().mockResolvedValue({ id: 'notification' });
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  await announcementThreadTools(
    { getMessage, createMessage },
    'owner/repo',
    controller.signal
  )({ registerTool } as unknown as ExtensionAPI);
  const call = (overrides = {}) =>
    registerTool.mock.calls[0]![0].execute(
      'call',
      {
        originMemoryId: 'abc12',
        roomId: 'room',
        threadRootId: 'root',
        text: 'Fix landed.',
        ...overrides
      },
      undefined,
      undefined,
      {} as ExtensionToolContext
    );
  return { controller, getMessage, createMessage, call };
}

test('posts to the recorded thread, returns its receipt, and reuses the origin key on retry', async () => {
  const { call, createMessage } = await setup();
  const result = await call();
  expect(result.content).toEqual([
    {
      type: 'text',
      text: JSON.stringify({ messageId: 'notification', roomId: 'room', threadRootId: 'root' })
    }
  ]);
  await call();
  expect(createMessage).toHaveBeenCalledWith(
    { roomId: 'room', threadRootId: 'root' },
    'Fix landed.',
    {
      signal: expect.any(AbortSignal),
      idempotencyKey: expect.stringMatching(
        /^[a-f0-9]{8}-[a-f0-9]{4}-8[a-f0-9]{3}-a[a-f0-9]{3}-[a-f0-9]{12}$/
      )
    }
  );
  expect(createMessage.mock.calls[0]![2]!.idempotencyKey).toBe(
    createMessage.mock.calls[1]![2]!.idempotencyKey
  );
  await call({ originMemoryId: 'other' });
  expect(createMessage.mock.calls[2]![2]!.idempotencyKey).not.toBe(
    createMessage.mock.calls[0]![2]!.idempotencyKey
  );
});

test('rejects empty targets, missing messages, replies used as roots, and cancellation without posting', async () => {
  const { call, getMessage, createMessage, controller } = await setup();
  await expect(call({ threadRootId: '' })).rejects.toThrow('existing thread');
  expect(getMessage).not.toHaveBeenCalled();
  getMessage.mockResolvedValue(undefined);
  await expect(call()).rejects.toThrow('root is unavailable');
  getMessage.mockResolvedValue({
    id: 'root',
    roomId: 'room',
    authorId: 'reporter',
    threadRootId: 'actual-root'
  });
  await expect(call()).rejects.toThrow('root is unavailable');
  controller.abort();
  await expect(call()).rejects.toThrow();
  expect(createMessage).not.toHaveBeenCalled();
});

test('propagates an uncertain send instead of returning a receipt', async () => {
  const { call, createMessage } = await setup();
  createMessage.mockRejectedValue(
    Object.assign(new Error('private response body'), { code: 'ECONNREFUSED' })
  );
  await expect(call()).rejects.toThrow('Announcement thread delivery failed. Connection refused.');
});
