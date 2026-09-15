import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createMessageAPI, type AttachmentUploadUpdate } from './messages';

const uploadAttachmentMock = vi.hoisted(() => vi.fn());
const createMessageMock = vi.hoisted(() => vi.fn());
const setAttachmentDescriptionMock = vi.hoisted(() => vi.fn());

vi.mock('./assetUploads.js', () => ({
  createAssetUploadAPI: () => ({ uploadAttachment: uploadAttachmentMock })
}));

vi.mock('./connect.js', () => ({
  authHeaders: () => new Headers(),
  createChattoClient: () => ({
    createMessage: createMessageMock,
    setAttachmentDescription: setAttachmentDescriptionMock
  }),
  handleAuthError: (_config: unknown, error: unknown) => {
    throw error;
  }
}));

vi.mock('./roomTimeline.js', () => ({
  messageToTimelineEvent: vi.fn(),
  timelineUsersForMessages: vi.fn(async () => new Map())
}));

describe('message attachment uploads', () => {
  beforeEach(() => {
    uploadAttachmentMock.mockReset();
    createMessageMock.mockReset();
    setAttachmentDescriptionMock.mockReset();
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

  it('associates descriptions with asset IDs only after uploads complete', async () => {
    const first = new File([new Uint8Array(4)], 'first.png', { type: 'image/png' });
    const second = new File([new Uint8Array(4)], 'second.png', { type: 'image/png' });
    uploadAttachmentMock.mockImplementation(async ({ file }) => ({
      assetId: `asset-${file.name}`
    }));

    await createMessageAPI({ baseUrl: '/api/connect', bearerToken: null }).createMessage({
      roomId: 'room-1',
      body: '',
      attachments: [first, second],
      attachmentDescriptions: [{ file: second, description: 'Second image' }]
    });

    expect(uploadAttachmentMock).toHaveBeenCalledTimes(2);
    expect(createMessageMock).toHaveBeenCalledWith(
      expect.objectContaining({
        attachmentAssetIds: ['asset-first.png', 'asset-second.png'],
        attachmentDescriptions: [{ assetId: 'asset-second.png', description: 'Second image' }]
      }),
      expect.anything()
    );
  });
});

describe('attachment description updates', () => {
  beforeEach(() => {
    setAttachmentDescriptionMock.mockReset();
    setAttachmentDescriptionMock.mockResolvedValue({ message: null });
  });

  it('sends the canonical message and attachment IDs', async () => {
    const result = await createMessageAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    }).setAttachmentDescription('room-1', 'message-1', 'asset-1', 'Replacement');

    expect(setAttachmentDescriptionMock).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        eventId: 'message-1',
        attachmentId: 'asset-1',
        description: 'Replacement'
      },
      expect.anything()
    );
    expect(result).toEqual({ updated: true, event: null });
  });
});

describe('message thread creation', () => {
  beforeEach(() => {
    uploadAttachmentMock.mockReset();
    createMessageMock.mockReset();
    createMessageMock.mockResolvedValue({ message: null });
  });

  it('sends the explicit thread creation flag', async () => {
    await createMessageAPI({ baseUrl: '/api/connect', bearerToken: null }).createMessage({
      roomId: 'room-1',
      body: 'Discuss this',
      createThread: true
    });

    expect(createMessageMock).toHaveBeenCalledWith(
      expect.objectContaining({ roomId: 'room-1', createThread: true }),
      expect.anything()
    );
  });
});
