import { ConnectError, Code, createRouterTransport } from '@connectrpc/connect';
import { createHash } from 'node:crypto';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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

beforeEach(() => {
  vi.resetAllMocks();
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

/** Drive retry timers while preserving rejection handling before timers run. */
async function settle<T>(promise: Promise<T>): Promise<T> {
  const result = promise.then(
    (value) => ({ value }),
    (error: unknown) => ({ error })
  );
  await vi.runAllTimersAsync();
  const outcome = await result;
  if ('error' in outcome) throw outcome.error;
  return outcome.value;
}

describe('createAssetUploadAPI', () => {
  it('preserves every byte when a chunk is stored but its acknowledgement is lost', async () => {
    // Reproducible, varied bytes make offsets and ordering matter. Node's hash
    // implementation independently checks the client's js-sha256 digests.
    const original = new Uint8Array(2 * 1024 * 1024 + 137);
    let seed = 0x12345678;
    for (let i = 0; i < original.length; i++) {
      seed ^= seed << 13;
      seed ^= seed >>> 17;
      seed ^= seed << 5;
      original[i] = seed & 0xff;
    }
    const digest = (bytes: Uint8Array) => createHash('sha256').update(bytes).digest('hex');
    const chunks: Uint8Array[] = [];
    let committedOffset = 0n;
    let declaredHash = '';
    let declaredSize = 0n;
    const uploadId = 'stateful-upload';
    const progress = vi.fn();

    mocks.createUpload.mockImplementation((request) => {
      declaredHash = request.sha256;
      declaredSize = request.size;
      expect(declaredSize).toBe(BigInt(original.length));
      expect(declaredHash).toBe(digest(original));
      return { upload: { uploadId, committedOffset, maxChunkSize: 1024 * 1024 } };
    });
    mocks.uploadChunk.mockImplementation((request) => {
      expect(request.uploadId).toBe(uploadId);
      expect(request.offset).toBe(committedOffset);
      expect(request.content.length).toBeGreaterThan(0);
      expect(request.content.length).toBeLessThanOrEqual(1024 * 1024);
      expect(request.chunkSha256).toBe(digest(request.content));
      chunks.push(request.content.slice());
      committedOffset += BigInt(request.content.length);
      expect(committedOffset).toBeLessThanOrEqual(declaredSize);
      // Persist first, fail second: recovery must discover this committed
      // chunk rather than send its bytes again or create another session.
      if (chunks.length === 2) throw new ConnectError('lost acknowledgement', Code.Unavailable);
      return { upload: { uploadId, committedOffset } };
    });
    mocks.getUpload.mockImplementation((request) => {
      expect(request.uploadId).toBe(uploadId);
      return { upload: { uploadId, committedOffset } };
    });
    mocks.completeUpload.mockImplementation((request) => {
      expect(request.uploadId).toBe(uploadId);
      expect(committedOffset).toBe(declaredSize);
      const stored = Buffer.concat(chunks);
      expect(stored.equals(Buffer.from(original))).toBe(true);
      expect(digest(stored)).toBe(declaredHash);
      return { asset: { ...asset, size: declaredSize } };
    });

    const result = await settle(
      api().uploadAttachment({
        roomId: 'R1',
        file: new File([original], 'varied.bin'),
        onProgress: progress
      })
    );

    expect(result.size).toBe(BigInt(original.length));
    expect(mocks.createUpload).toHaveBeenCalledOnce();
    expect(mocks.getUpload).toHaveBeenCalledOnce();
    expect(mocks.completeUpload).toHaveBeenCalledOnce();
    expect(new Set(chunks.map((chunk) => chunk.length)).size).toBeGreaterThan(1);
    let cumulativeBytes = 0;
    const committedProgress = [0, ...chunks.map((chunk) => (cumulativeBytes += chunk.length))];
    expect(progress.mock.calls).toEqual(committedProgress.map((bytes) => [bytes, original.length]));
  });

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
    mocks.uploadChunk.mockReturnValue({ upload: { committedOffset: 3n } });
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

  it('retries a failed chunk four times and then fails', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockImplementation(() => {
      throw new ConnectError('lost', Code.Unavailable);
    });
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 0n } });
    await expect(
      settle(api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'n.txt') }))
    ).rejects.toThrow('lost');
    expect(mocks.uploadChunk).toHaveBeenCalledTimes(5);
  });

  it('rejects answers without an upload or asset ID', async () => {
    mocks.createUpload.mockReturnValue({});
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'n.txt') })
    ).rejects.toThrow('upload id');

    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockReturnValue({ upload: { committedOffset: 3n } });
    mocks.completeUpload.mockReturnValue({});
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'n.txt') })
    ).rejects.toThrow('asset id');
  });

  it('starts small, grows gradually on fast links, and respects the server ceiling', async () => {
    const sizes: number[] = [];
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1', maxChunkSize: 1024 * 1024 } });
    mocks.uploadChunk.mockImplementation((request) => {
      sizes.push(request.content.length);
      return { upload: { committedOffset: request.offset + BigInt(request.content.length) } };
    });
    mocks.completeUpload.mockReturnValue({ asset });
    await api().uploadAttachment({
      roomId: 'R1',
      file: new File([new Uint8Array(3 * 1024 * 1024)], 'f')
    });
    expect(sizes).toEqual([256 * 1024, 512 * 1024, 1024 * 1024, 1024 * 1024, 256 * 1024]);
  });

  it('reduces chunks on a slow link and adapts when bandwidth drops', async () => {
    let now = 0;
    vi.spyOn(performance, 'now').mockImplementation(() => now);
    const sizes: number[] = [];
    mocks.createUpload.mockReturnValue({
      upload: { uploadId: 'up-1', maxChunkSize: 10 * 1024 * 1024 }
    });
    mocks.uploadChunk.mockImplementation((request) => {
      sizes.push(request.content.length);
      // First request is fast; subsequent bandwidth is 64 kbit/s.
      now += sizes.length === 1 ? 1000 : (request.content.length / 8000) * 1000;
      return { upload: { committedOffset: request.offset + BigInt(request.content.length) } };
    });
    mocks.completeUpload.mockReturnValue({ asset });
    await api().uploadAttachment({
      roomId: 'R1',
      file: new File([new Uint8Array(1024 * 1024)], 'f')
    });
    expect(sizes.slice(0, 4)).toEqual([256 * 1024, 512 * 1024, 120_000, 120_000]);
  });

  it('recovers within the retry budget when a fast link drops to 64 kbit/s', async () => {
    let now = 0;
    let committedOffset = 0n;
    let slow = false;
    vi.spyOn(performance, 'now').mockImplementation(() => now);
    mocks.createUpload.mockReturnValue({
      upload: { uploadId: 'up-1', maxChunkSize: 10 * 1024 * 1024 }
    });
    mocks.uploadChunk.mockImplementation((request) => {
      slow ||= request.content.length === 10 * 1024 * 1024;
      const duration = slow ? (request.content.length / 8000) * 1000 : 1000;
      now += Math.min(duration, 60_000);
      if (duration > 60_000) throw new ConnectError('timeout', Code.DeadlineExceeded);
      committedOffset = request.offset + BigInt(request.content.length);
      return { upload: { committedOffset } };
    });
    mocks.getUpload.mockImplementation(() => ({ upload: { committedOffset } }));
    mocks.completeUpload.mockReturnValue({ asset });
    await settle(
      api().uploadAttachment({
        roomId: 'R1',
        file: new File([new Uint8Array(26 * 1024 * 1024)], 'f')
      })
    );
    expect(committedOffset).toBe(26n * 1024n * 1024n);
    expect(mocks.getUpload).toHaveBeenCalledTimes(3);
  });

  it('shrinks a timed-out chunk only after checking committed progress', async () => {
    mocks.createUpload.mockReturnValue({
      upload: { uploadId: 'up-1', maxChunkSize: 10 * 1024 * 1024 }
    });
    mocks.uploadChunk.mockRejectedValueOnce(new ConnectError('timeout', Code.DeadlineExceeded));
    mocks.uploadChunk.mockImplementation((request) => ({
      upload: { committedOffset: request.offset + BigInt(request.content.length) }
    }));
    mocks.getUpload.mockRejectedValueOnce(new ConnectError('offline', Code.Unavailable));
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 0n } });
    mocks.completeUpload.mockRejectedValueOnce(
      new ConnectError('lost completion', Code.Unavailable)
    );
    mocks.completeUpload.mockReturnValue({ asset });
    await settle(
      api().uploadAttachment({ roomId: 'R1', file: new File([new Uint8Array(256 * 1024)], 'f') })
    );
    expect(
      mocks.uploadChunk.mock.calls.map(([request]) => [
        Number(request.offset),
        request.content.length
      ])
    ).toEqual([
      [0, 256 * 1024],
      [0, 64 * 1024],
      [64 * 1024, 128 * 1024],
      [192 * 1024, 64 * 1024]
    ]);
    expect(mocks.getUpload).toHaveBeenCalledTimes(2);
    expect(mocks.completeUpload).toHaveBeenCalledTimes(2);
    expect(mocks.createUpload).toHaveBeenCalledOnce();
  });

  it.each([Code.PermissionDenied, Code.Unauthenticated, Code.Canceled])(
    'does not retry permanent failure %s',
    async (code) => {
      mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
      mocks.uploadChunk.mockRejectedValue(new ConnectError('stop', code));
      await expect(
        api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') })
      ).rejects.toThrow('stop');
      expect(mocks.uploadChunk).toHaveBeenCalledOnce();
      expect(mocks.getUpload).not.toHaveBeenCalled();
    }
  );

  it.each([undefined, -1n, 0n, 4n])('rejects invalid successful offsets (%s)', async (offset) => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockReturnValue(
      offset === undefined ? {} : { upload: { committedOffset: offset } }
    );
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') })
    ).rejects.toThrow('invalid committed offset');
    expect(mocks.uploadChunk).toHaveBeenCalledOnce();
    expect(mocks.completeUpload).not.toHaveBeenCalled();
  });

  it('recovers from a real request deadline without resending a committed chunk', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockImplementationOnce(async (_request, context) => {
      await new Promise<void>((resolve) =>
        context.signal.addEventListener('abort', () => resolve(), { once: true })
      );
      throw new ConnectError('request ended', Code.Canceled);
    });
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 3n } });
    mocks.completeUpload.mockReturnValue({ asset });
    await settle(api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') }));
    expect(mocks.uploadChunk).toHaveBeenCalledOnce();
    expect(mocks.getUpload).toHaveBeenCalledOnce();
    expect(mocks.completeUpload).toHaveBeenCalledOnce();
  });

  it('stops after bounded status retries without blindly resending a chunk', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockRejectedValue(new ConnectError('lost', Code.Unavailable));
    mocks.getUpload.mockRejectedValue(new ConnectError('still offline', Code.Unavailable));
    await expect(
      settle(api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') }))
    ).rejects.toThrow('still offline');
    expect(mocks.getUpload).toHaveBeenCalledTimes(5);
    expect(mocks.uploadChunk).toHaveBeenCalledOnce();
  });

  it('reconciles an offset conflict from an earlier attempt that committed late', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockRejectedValue(new ConnectError('offset conflict', Code.InvalidArgument));
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 3n } });
    mocks.completeUpload.mockReturnValue({ asset });
    await api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') });
    expect(mocks.uploadChunk).toHaveBeenCalledOnce();
  });

  it.each([Code.Internal, Code.ResourceExhausted, Code.Aborted])(
    'recovers from temporary server failure %s',
    async (code) => {
      mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
      mocks.uploadChunk.mockRejectedValueOnce(new ConnectError('temporary failure', code));
      mocks.uploadChunk.mockReturnValue({ upload: { committedOffset: 3n } });
      mocks.getUpload.mockReturnValue({ upload: { committedOffset: 0n } });
      mocks.completeUpload.mockReturnValue({ asset });
      await settle(api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') }));
      expect(mocks.uploadChunk).toHaveBeenCalledTimes(2);
    }
  );

  it('recovers from a fetch network error without shrinking the chunk', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockReturnValue({ upload: { committedOffset: 3n } });
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 0n } });
    mocks.completeUpload.mockReturnValue({ asset });
    let failed = false;
    const client = createAssetUploadAPI({
      baseUrl: 'https://chatto.test',
      bearerToken: null,
      transport: (interceptors) =>
        createRouterTransport((router) => router.service(AssetUploadService, mocks), {
          transport: {
            interceptors: [
              ...interceptors,
              (next) => async (request) => {
                if (request.method.name === 'UploadChunk' && !failed) {
                  failed = true;
                  throw new TypeError('Failed to fetch');
                }
                return next(request);
              }
            ]
          }
        })
    });
    await settle(client.uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') }));
    expect(mocks.getUpload).toHaveBeenCalledOnce();
    expect(receivedRequest(mocks.uploadChunk)?.content.length).toBe(3);
  });

  it('does not retry a validation failure when no progress was committed', async () => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockRejectedValue(new ConnectError('bad chunk', Code.InvalidArgument));
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: 0n } });
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') })
    ).rejects.toThrow('bad chunk');
    expect(mocks.uploadChunk).toHaveBeenCalledOnce();
  });

  it.each([-1n, 4n])('rejects invalid recovery offsets (%s)', async (offset) => {
    mocks.createUpload.mockReturnValue({ upload: { uploadId: 'up-1' } });
    mocks.uploadChunk.mockRejectedValue(new ConnectError('lost', Code.Unavailable));
    mocks.getUpload.mockReturnValue({ upload: { committedOffset: offset } });
    await expect(
      api().uploadAttachment({ roomId: 'R1', file: new File(['abc'], 'f') })
    ).rejects.toThrow('invalid committed offset');
    expect(mocks.uploadChunk).toHaveBeenCalledOnce();
  });
});
