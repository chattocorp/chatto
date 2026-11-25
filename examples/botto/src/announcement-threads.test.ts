/** Verify follow-ups cannot create channel roots and return receipts only after successful sends. */
import type { ExtensionAPI, ExtensionToolContext } from '@earendil-works/pi-coding-agent';
import type { Server } from '@chatto/client';
import { expect, test, vi } from 'vitest';
import type { MeansCall } from './followup-memory.ts';
import { announcementThreadTools } from './announcement-threads.ts';

vi.mock('./followup-memory.ts', async (original) => ({
  ...(await original<typeof import('./followup-memory.ts')>()),
  withFollowupMemory: async (
    _entry: unknown,
    _signal: AbortSignal,
    work: (call: MeansCall) => Promise<unknown>
  ) => {
    const origin = {
      id: 'abc12',
      hash: 'hash',
      frontmatter: {
        data: {
          issue_origin: {
            repository: 'owner/repo',
            server_url: 'https://chat.example.com',
            issue_number: 1,
            room_id: 'room',
            thread_id: 'root'
          }
        }
      }
    };
    let id = 'abc12';
    return work(async (name, args) => {
      if (name === 'read') {
        id = String(args.id);
        return { ...origin, id };
      }
      if (name === 'update') return { ...origin, id };
      return {
        documents: (args.filter as Record<string, unknown>).issue_origin ? [{ ...origin, id }] : [],
        diagnostics: [],
        has_more: false
      };
    });
  }
}));

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
  await announcementThreadTools({ getMessage, createMessage }, 'owner/repo', controller.signal, {
    serverUrl: 'https://chat.example.com',
    memory: {} as never
  })({ registerTool } as unknown as ExtensionAPI);
  const call = (overrides = {}) =>
    registerTool.mock.calls[0]![0].execute(
      'call',
      {
        commitSha: 'a'.repeat(40),
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
      text: JSON.stringify({
        messageId: 'notification',
        alreadyDelivered: false,
        roomId: 'room',
        threadRootId: 'root'
      })
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

test('rejects routing-only text before lookup and permits a corrected notification', async () => {
  const { call, getMessage, createMessage } = await setup();
  getMessage.mockResolvedValue(undefined);
  for (const text of [
    'thread_root_id EQV0DKTBlhnnvyM',
    'threadRootId: root',
    'room_id room\nthread_root_id root',
    'root'
  ]) {
    await expect(call({ text })).rejects.toThrow('Nothing was sent');
  }
  expect(getMessage).not.toHaveBeenCalled();
  expect(createMessage).not.toHaveBeenCalled();
  getMessage.mockResolvedValue({ id: 'root', roomId: 'room', authorId: 'reporter' });
  await call({ text: '✅ The syntax highlighting fix has landed on main.' });
  expect(createMessage).toHaveBeenCalledTimes(1);
});
