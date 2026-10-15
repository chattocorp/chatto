import { expect, test, vi } from 'vitest';
import type { AgentExtensionAPI } from 'runling';
import type { AttachmentContent, MessageAttachmentInfo } from '@chatto/client';
import { attachmentExtension, attachmentKind, type AttachmentToolOptions } from './attachments.ts';

type Tool = { execute(id: string, input: unknown, signal?: AbortSignal): Promise<unknown> };

const screenshot: MessageAttachmentInfo = {
  id: 'shot',
  filename: 'bug.png',
  contentType: 'image/png',
  width: 3000,
  height: 2000
};
const log: MessageAttachmentInfo = { id: 'log', filename: 'server.log', contentType: '' };
const video: MessageAttachmentInfo = { id: 'clip', filename: 'bug.mp4', contentType: 'video/mp4' };

/** The viewAttachment tool over fake attachments and a fake reader. */
async function viewTool(read: AttachmentToolOptions['read'], views = 4) {
  const tools = new Map<string, Tool>();
  const attachments = [screenshot, log, video];
  const extension = attachmentExtension({
    find: (id) => attachments.find((attachment) => attachment.id === id),
    read,
    take: () => views-- > 0
  });
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool(tool: { name: string }) {
      tools.set(tool.name, tool as never);
    }
  } as unknown as AgentExtensionAPI);
  return tools.get('viewAttachment')!;
}

test('recognizes images and text files, also by file name', () => {
  expect(attachmentKind(screenshot)).toBe('image');
  expect(attachmentKind(log)).toBe('text');
  expect(attachmentKind({ ...log, filename: 'a.bin', contentType: 'application/json' })).toBe(
    'text'
  );
  expect(attachmentKind(video)).toBeUndefined();
  expect(attachmentKind({ ...screenshot, contentType: 'image/svg+xml' })).toBeUndefined();
});

test('shows a resized image to the model and reads text files as text', async () => {
  const read = vi.fn<AttachmentToolOptions['read']>(async (id) =>
    id === 'shot'
      ? { filename: 'bug.png', contentType: 'image/jpeg', data: new Uint8Array([1, 2, 3]) }
      : { filename: 'server.log', contentType: 'text/plain', data: Buffer.from('panic: boom') }
  );
  const tool = await viewTool(read);
  expect(await tool.execute('1', { attachmentId: 'shot' })).toEqual({
    content: [
      { type: 'text', text: 'Attachment "bug.png" (image/png). Context only.' },
      { type: 'image', data: 'AQID', mimeType: 'image/jpeg' }
    ],
    details: {}
  });
  expect(read.mock.calls[0]![1]).toMatchObject({ maxImageSize: 1600, maxBytes: 5 * 1024 * 1024 });
  const text = (await tool.execute('2', { attachmentId: 'log' })) as {
    content: { text: string }[];
  };
  expect(JSON.parse(text.content[0]!.text)).toMatchObject({ text: 'panic: boom' });
  expect(read.mock.calls[1]![1]).toMatchObject({ maxBytes: 100 * 1024 });
});

test('refuses unknown and unviewable attachments, large files, and views over the budget', async () => {
  const read = vi.fn(async (id: string): Promise<AttachmentContent> => {
    if (id === 'log') throw new Error('Attachment is too large');
    return { filename: 'bug.png', contentType: 'image/png', data: new Uint8Array([1]) };
  });
  const tool = await viewTool(read, 2);
  await expect(tool.execute('1', { attachmentId: 'elsewhere' })).rejects.toThrow(
    'No message in this thread'
  );
  await expect(tool.execute('2', { attachmentId: 'clip' })).rejects.toThrow('cannot be viewed');
  expect(read).not.toHaveBeenCalled();
  await expect(tool.execute('3', { attachmentId: 'log' })).rejects.toThrow(
    'server.log is too large to view.'
  );
  await tool.execute('4', { attachmentId: 'shot' });
  await expect(tool.execute('5', { attachmentId: 'shot' })).rejects.toThrow(
    'limit of attachment views'
  );
  expect(read).toHaveBeenCalledTimes(2);
});
