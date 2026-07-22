import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createMessageAPI, type AttachmentUploadUpdate } from './messages';

const uploadAttachmentMock = vi.hoisted(() => vi.fn());
const createMessageMock = vi.hoisted(() => vi.fn());

vi.mock('./assetUploads.js', () => ({
  createAssetUploadAPI: () => ({ uploadAttachment: uploadAttachmentMock })
}));

vi.mock('./connect.js', () => ({
  authHeaders: () => new Headers(),
  createChattoClient: () => ({ createMessage: createMessageMock }),
  handleAuthError: (_config: unknown, error: unknown) => {
    throw error;
  }
}));

vi.mock('./roomTimeline.js', () => ({
  messageToRawEvent: vi.fn(),
  timelineUsersForMessages: vi.fn(async () => new Map())
}));

describe('message attachment uploads', () => {
  beforeEach(() => {
    uploadAttachmentMock.mockReset();
    createMessageMock.mockReset();
    createMessageMock.mockResolvedValue({ message: null });
  });

  it('reports committed progress and completion for each attachment', async () => {
    const first = new File([new Uint8Array(4)], 'first.png', { type: 'image/png' });
    const second = new File([new Uint8Array(8)], 'second.mp4', { type: 'video/mp4' });
    const updates: AttachmentUploadUpdate[] = [];
    uploadAttachmentMock.mockImplementation(async ({ file, onProgress }) => {
      onProgress(file.size / 2, file.size);
      return { assetId: `asset-${file.name}` };
    });

    await createMessageAPI({ baseUrl: '/api/connect', bearerToken: null }).createMessage({
      roomId: 'room-1',
      body: 'attachments',
      attachments: [first, second],
      onAttachmentUploadUpdate: (update) => updates.push(update)
    });

    expect(updates).toEqual([
      { file: first, phase: 'uploading', committedBytes: 2, totalBytes: 4 },
      { file: second, phase: 'uploading', committedBytes: 4, totalBytes: 8 },
      { file: first, phase: 'uploaded' },
      { file: second, phase: 'uploaded' }
    ]);
    expect(createMessageMock).toHaveBeenCalledWith(
      expect.objectContaining({
        roomId: 'room-1',
        attachmentAssetIds: ['asset-first.png', 'asset-second.mp4']
      }),
      expect.anything()
    );
  });

  it('reports the attachment that failed', async () => {
    const file = new File([new Uint8Array(4)], 'failed.png', { type: 'image/png' });
    const updates: AttachmentUploadUpdate[] = [];
    uploadAttachmentMock.mockRejectedValue(new Error('upload failed'));

    await expect(
      createMessageAPI({ baseUrl: '/api/connect', bearerToken: null }).createMessage({
        roomId: 'room-1',
        body: '',
        attachments: [file],
        onAttachmentUploadUpdate: (update) => updates.push(update)
      })
    ).rejects.toThrow('upload failed');

    expect(updates).toEqual([{ file, phase: 'failed' }]);
    expect(createMessageMock).not.toHaveBeenCalled();
  });
});
