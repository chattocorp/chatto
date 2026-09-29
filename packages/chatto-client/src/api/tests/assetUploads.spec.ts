import { ConnectError, Code } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AssetUploadService } from '@chatto/api-types/api/v1/asset_uploads_connect';
import { createAssetUploadAPI } from '../assetUploads.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(AssetUploadService);
const api = () =>
  createAssetUploadAPI(fakeServer((router) => router.service(AssetUploadService, mocks)));
const asset = {
  id: 'asset-1',
  filename: 'notes.txt',
  contentType: 'text/plain',
  size: 10n,
  width: 0,
  height: 0
};

beforeEach(() => vi.resetAllMocks());

describe('createAssetUploadAPI', () => {
  it('uploads a file in chunks and reports progress', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1', maxChunkSize: 4 } });
    mocks.uploadChunk.mockImplementation((request) => ({
      upload: { committedOffset: request.offset + BigInt(request.content.length) }
    }));
    mocks.completeUpload.mockReturnValue({ asset });
    const progress = vi.fn();
    const file = new File(['0123456789'], 'notes.txt', { type: 'text/plain' });

    await expect(
      api().uploadAttachment({ roomId: 'R1', file, onProgress: progress })
    ).resolves.toEqual({
      assetId: 'asset-1',
      filename: 'notes.txt',
      contentType: 'text/plain',
      size: 10n,
      width: 0,
      height: 0
    });
    expect(receivedRequest(mocks.createUpload)).toMatchObject({
      roomId: 'R1',
      filename: 'notes.txt',
      contentType: 'text/plain',
      size: 10n,
      sha256: '84d89877f0d4041efb6bf91a16f0248f2fd573e6af05c19f96bedb9f882f7882'
    });
    expect(mocks.uploadChunk).toHaveBeenCalledTimes(3);
    expect(progress.mock.calls.map(([committed]) => committed)).toEqual([0, 4, 8, 10]);
  });

  it('names an unnamed file and uses the default chunk size', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockReturnValue({});
    mocks.completeUpload.mockReturnValue({ asset });
    await api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], '') });
    expect(receivedRequest(mocks.createUpload)).toMatchObject({
      filename: 'attachment',
      contentType: 'application/octet-stream'
    });
    expect(mocks.uploadChunk).toHaveBeenCalledOnce();
  });

  it('resumes from the offset that the server committed after a failed chunk', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1', maxChunkSize: 5 } });
    mocks.uploadChunk
      .mockImplementationOnce(() => {
        throw new ConnectError('lost', Code.Unavailable);
      })
      .mockImplementation((request) => ({
        upload: { committedOffset: request.offset + BigInt(request.content.length) }
      }));
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 5n } });
    mocks.completeUpload.mockReturnValue({ asset });
    const progress = vi.fn();
    await api().uploadAttachment({
      roomId: 'R1',
      file: new File(['0123456789'], 'n.txt'),
      onProgress: progress
    });
    // The first chunk reached the server; only the second is sent again.
    expect(mocks.uploadChunk.mock.calls.map(([request]) => Number(request.offset))).toEqual([0, 5]);
    expect(progress.mock.calls.map(([committed]) => committed)).toEqual([0, 5, 10]);
  });

  it('retries a failed chunk twice and then fails', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockImplementation(() => {
      throw new ConnectError('lost', Code.Unavailable);
    });
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 0n } });
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'n.txt') })
    ).rejects.toThrow('lost');
    expect(mocks.uploadChunk).toHaveBeenCalledTimes(3);
  });

  it('rejects answers without an upload or asset ID', async () => {
    mocks.createUpload.mockReturnValue({});
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'n.txt') })
    ).rejects.toThrow('upload id');

    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockReturnValue({});
    mocks.completeUpload.mockReturnValue({});
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'n.txt') })
    ).rejects.toThrow('asset id');
  });
});
