/** Check crop pixels, scoped access, resource bounds, and request-only tool images. */
import type {
  ContextEvent,
  ExtensionAPI,
  ExtensionContext,
  ExtensionToolContext
} from '@earendil-works/pi-coding-agent';
import sharp from 'sharp';
import { randomBytes } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import { viewImageExtension } from './view-image.ts';

async function setup() {
  // Red left half and blue right half make crop coordinates observable in decoded pixels.
  const data = await sharp({ create: { width: 100, height: 60, channels: 3, background: 'red' } })
    .composite([
      {
        input: await sharp({ create: { width: 50, height: 60, channels: 3, background: 'blue' } })
          .png()
          .toBuffer(),
        left: 50,
        top: 0
      }
    ])
    .png()
    .toBuffer();
  const attachment = { id: 'image', contentType: 'image/png', filename: 'image.png' };
  const server = {
    getMessage: vi.fn().mockResolvedValue({ attachments: [attachment] }),
    readAttachment: vi.fn().mockResolvedValue({ contentType: 'image/png', data })
  };
  const registerTool = vi.fn<ExtensionAPI['registerTool']>();
  let transform!: (
    event: ContextEvent,
    ctx: ExtensionContext
  ) => Promise<{ messages: ContextEvent['messages'] }>;
  const controller = new AbortController();
  await viewImageExtension(
    server,
    'room',
    {
      messages: [
        { id: 'message', text: '', attachments: [attachment] },
        {
          id: 'EOixXw1lA53OS4M',
          text: 'And this?',
          attachments: [{ ...attachment, id: 'AL1OTYiP2g7EfGL' }]
        }
      ],
      olderContextOmitted: false
    },
    controller.signal
  )({
    registerTool,
    getActiveTools: () => ['view_image'],
    setActiveTools: vi.fn(),
    on: (_event: string, handler: typeof transform) => {
      transform = handler;
    }
  } as unknown as ExtensionAPI);
  const tool = registerTool.mock.calls[0]![0];
  const ctx = { model: { input: ['text', 'image'] } } as ExtensionToolContext;
  const call = (crop?: { x: number; y: number; width: number; height: number }, id = 'call') =>
    tool.execute(id, { attachmentId: 'image', crop }, undefined, undefined, ctx);
  const history = (result: Awaited<ReturnType<typeof call>>): ContextEvent['messages'] => [
    {
      role: 'toolResult',
      toolCallId: 'call',
      toolName: 'view_image',
      content: result.content,
      isError: false,
      timestamp: 1
    }
  ];
  const request = async (messages: ContextEvent['messages']) =>
    (await transform({ type: 'context', messages }, ctx)).messages;
  return { server, call, tool, ctx, controller, request, history, data };
}

test('crops the original before resizing and only adds pixels to request copies', async () => {
  const { call, request, history, server } = await setup();
  const result = await call({ x: 0.5, y: 0, width: 0.5, height: 1 });
  expect(result.content.every((part) => part.type === 'text')).toBe(true);
  const stored = history(result);
  const original = structuredClone(stored);
  const outgoing = await request(stored);
  const message = outgoing[0]!;
  if (message.role !== 'toolResult') throw new Error('Expected tool result');
  const image = message.content.find((part) => part.type === 'image');
  if (image?.type !== 'image') throw new Error('Expected image');
  const { data, info } = await sharp(Buffer.from(image.data, 'base64'))
    .removeAlpha()
    .raw()
    .toBuffer({ resolveWithObject: true });
  expect([info.width, info.height]).toEqual([50, 60]);
  expect([...data.subarray(0, 3)]).toEqual([0, 0, 255]);
  expect(stored).toEqual(original);
  expect(await request(stored)).toEqual(outgoing);
  expect(server.readAttachment).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', attachmentId: 'image' },
    { signal: expect.any(AbortSignal), maxBytes: 16 * 1024 * 1024 }
  );
});

test('coordinates apply after EXIF orientation and whole images fit within 2048 pixels', async () => {
  const { server, data, call } = await setup();
  server.readAttachment.mockResolvedValueOnce({
    data: await sharp(data).withMetadata({ orientation: 6 }).png().toBuffer()
  });
  expect((await call({ x: 0, y: 0.5, width: 1, height: 0.5 })).details).toMatchObject({
    width: 60,
    height: 50
  });
  server.readAttachment.mockResolvedValueOnce({
    data: await sharp({ create: { width: 3000, height: 1000, channels: 3, background: 'red' } })
      .png()
      .toBuffer()
  });
  expect((await call()).details).toMatchObject({ width: 2048, height: 683 });
});

test('rejects unknown IDs, invalid crops and text-only models before download', async () => {
  const { tool, ctx, call, server } = await setup();
  await expect(
    tool.execute('call', { attachmentId: 'outside' }, undefined, undefined, ctx)
  ).rejects.toThrow('outside');
  for (const crop of [
    { x: -1, y: 0, width: 1, height: 1 },
    { x: 0.8, y: 0, width: 0.5, height: 1 },
    { x: 0, y: 0, width: 0, height: 1 },
    { x: NaN, y: 0, width: 1, height: 1 }
  ])
    await expect(call(crop)).rejects.toThrow('Crop');
  ctx.model = { ...ctx.model!, input: ['text'] };
  await expect(call()).rejects.toThrow('cannot read images');
  expect(server.getMessage).not.toHaveBeenCalled();
  expect(server.readAttachment).not.toHaveBeenCalled();
});

test('resolves the owning message from the selected attachment without a model-supplied message ID', async () => {
  const { tool, server, ctx } = await setup();
  server.getMessage.mockResolvedValueOnce({
    attachments: [{ id: 'AL1OTYiP2g7EfGL', contentType: 'image/png' }]
  });
  expect(tool.parameters).toMatchObject({
    required: ['attachmentId'],
    properties: { attachmentId: { anyOf: [{ const: 'image' }, { const: 'AL1OTYiP2g7EfGL' }] } }
  });
  await tool.execute('call', { attachmentId: 'AL1OTYiP2g7EfGL' }, undefined, undefined, ctx);
  expect(server.getMessage).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', messageId: 'EOixXw1lA53OS4M' },
    { signal: expect.any(AbortSignal) }
  );
  expect(server.readAttachment).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', attachmentId: 'AL1OTYiP2g7EfGL' },
    { signal: expect.any(AbortSignal), maxBytes: 16 * 1024 * 1024 }
  );
});

test('rechecks membership, sanitizes failures, and stops cancelled requests', async () => {
  const { server, call, controller } = await setup();
  server.getMessage.mockResolvedValueOnce({ attachments: [] });
  await expect(call()).rejects.toThrow('could not be read');
  expect(server.readAttachment).not.toHaveBeenCalled();
  server.readAttachment.mockRejectedValueOnce(new Error('secret signed URL'));
  await expect(call()).rejects.toThrow('could not be read');
  server.readAttachment.mockResolvedValueOnce({ data: Buffer.from('invalid') });
  await expect(call()).rejects.toThrow('could not be read');
  controller.abort(new Error('Stopped'));
  await expect(call()).rejects.toThrow('Stopped');
});

test('parallel calls cannot exceed the four-attempt budget', async () => {
  const { server, call } = await setup();
  const results = await Promise.allSettled(
    Array.from({ length: 5 }, (_, i) => call(undefined, String(i)))
  );
  expect(results.filter((result) => result.status === 'fulfilled')).toHaveLength(5);
  expect(server.readAttachment).toHaveBeenCalledTimes(4);
});

test('rejects decoded images beyond the pixel limit', async () => {
  const { server, call } = await setup();
  server.readAttachment.mockResolvedValueOnce({
    data: await sharp({ create: { width: 6500, height: 6500, channels: 3, background: 'red' } })
      .png()
      .toBuffer()
  });
  await expect(call()).rejects.toThrow('could not be read');
});

test('rejects unsupported formats and outputs over four MiB', async () => {
  const { server, call } = await setup();
  server.readAttachment.mockResolvedValueOnce({
    data: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>')
  });
  await expect(call()).rejects.toThrow('could not be read');
  server.readAttachment.mockResolvedValueOnce({
    data: await sharp(randomBytes(2048 * 2048 * 3), {
      raw: { width: 2048, height: 2048, channels: 3 }
    })
      .png()
      .toBuffer()
  });
  await expect(call()).rejects.toThrow('could not be read');
});

test('cancellation during a download cannot publish late pixels', async () => {
  const { server, call, controller, request, history } = await setup();
  server.readAttachment.mockImplementationOnce(async () => {
    controller.abort(new Error('Stopped'));
    throw new Error('private URL');
  });
  await expect(call()).rejects.toThrow('Stopped');
  const stored = history({ content: [{ type: 'text', text: 'cancelled' }], details: {} });
  expect(await request(stored)).toEqual(stored);
});
