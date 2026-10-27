/** Image context stays bounded, authorized, cancellable, and separate from saved text. */
import { expect, test, vi } from 'vitest';
import type { Server } from '@chatto/client';
import { readImages } from './images.ts';
import { includeCurrentMessages } from './context.ts';
import { mergeThreadContext, type ThreadContext } from './context-messages.ts';

const message = (id: string) => ({
  id,
  text: '',
  attachments: [{ id: `image-${id}`, filename: 'image.png', contentType: 'image/png' }]
});
function server() {
  return {
    getMessage: vi.fn<Server['getMessage']>().mockResolvedValue(undefined),
    readAttachment: vi.fn<Server['readAttachment']>().mockResolvedValue({
      filename: 'image.png',
      contentType: 'image/png',
      data: new Uint8Array([1, 2, 3])
    })
  };
}

test('prioritizes the current batch, bounds downloads, and labels real image blocks without mutating history', async () => {
  const api = server();
  const snapshot: ThreadContext = {
    messages: ['root', 'one', 'two', 'three', 'four', 'current'].map(message),
    olderContextOmitted: false
  };
  const original = structuredClone(snapshot);
  const signal = new AbortController().signal;
  const result = await readImages(api, 'room', snapshot, ['current'], signal);
  expect(result.images.map((image) => image.messageId)).toEqual([
    'current',
    'four',
    'three',
    'two'
  ]);
  expect(api.readAttachment).toHaveBeenCalledTimes(4);
  expect(api.readAttachment).toHaveBeenCalledWith(
    { roomId: 'room', attachmentId: 'image-current' },
    { signal, maxImageSize: 2048, maxBytes: 4 * 1024 * 1024 }
  );
  expect(result.imageNotes).toHaveLength(2);
  const history = [
    {
      role: 'user' as const,
      content: JSON.stringify({ currentMessages: [{ id: 'current', text: '' }] }),
      timestamp: 1
    }
  ];
  const request = mergeThreadContext(history, { ...snapshot, ...result });
  const first = request[0]!;
  if (first.role !== 'user') throw new Error('Expected user request');
  const content = first.content;
  expect(Array.isArray(content) && content.filter((part) => part.type === 'image')).toHaveLength(4);
  expect(
    Array.isArray(content) &&
      content.filter((part) => part.type === 'text').some((part) => part.text.includes('AQID'))
  ).toBe(false);
  expect(JSON.stringify(history)).not.toContain('AQID');
  expect(snapshot).toEqual(original);
});

test('unavailable and unsupported images produce safe notes; cancellation stops the turn', async () => {
  const api = server();
  api.readAttachment.mockRejectedValueOnce(new Error('private signed URL')).mockResolvedValueOnce({
    filename: 'image.svg',
    contentType: 'image/svg+xml',
    data: new Uint8Array()
  });
  const snapshot = { messages: [message('one'), message('two')], olderContextOmitted: false };
  const result = await readImages(
    api,
    'room',
    snapshot,
    ['one', 'two'],
    new AbortController().signal
  );
  expect(result.images).toEqual([]);
  expect(result.imageNotes).toHaveLength(2);
  expect(JSON.stringify(result)).not.toContain('private');
  const controller = new AbortController();
  api.readAttachment.mockImplementationOnce(async () => {
    controller.abort(new Error('Stopped'));
    throw new Error('request aborted');
  });
  await expect(readImages(api, 'room', snapshot, ['one'], controller.signal)).rejects.toThrow(
    'Stopped'
  );
});

test('unavailable batch members do not block other images or expose raw lookup errors', async () => {
  const api = server();
  api.getMessage.mockRejectedValue(new Error('private service details'));
  const context = await includeCurrentMessages(
    api,
    'room',
    { messages: [message('visible')], olderContextOmitted: true },
    ['removed'],
    new AbortController().signal
  );
  const result = await readImages(api, 'room', context, ['removed'], new AbortController().signal);
  expect(result.images.map((image) => image.messageId)).toEqual(['visible']);
  expect(context.imageNotes).toEqual([
    'Images for message removed could not be checked. Do not infer their contents.'
  ]);
  expect(JSON.stringify(context)).not.toContain('private');
});

test('loads current batch images outside the recent window and deduplicates attachment IDs', async () => {
  const api = server();
  api.getMessage.mockResolvedValue({ roomId: 'room', authorId: 'human', ...message('current') });
  const context = await includeCurrentMessages(
    api,
    'room',
    { messages: [message('other'), message('other')], olderContextOmitted: true },
    ['current'],
    new AbortController().signal
  );
  const result = await readImages(api, 'room', context, ['current'], new AbortController().signal);
  expect(result.images.map((image) => image.messageId)).toEqual(['current', 'other']);
  expect(api.getMessage).toHaveBeenCalledOnce();
});
