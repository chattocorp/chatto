import { sha256 } from 'js-sha256';
import { Code, ConnectError } from '@connectrpc/connect';
import { AssetUploadService } from '@chatto/api-types/api/v1/asset_uploads_connect';
import { createChattoClient, type ConnectAPIConfig } from './connect.js';

export type UploadedAsset = {
  assetId: string;
  filename: string;
  contentType: string;
  size: bigint;
  width: number;
  height: number;
};

export type UploadAttachmentOptions = {
  roomId: string;
  file: File;
  onProgress?: (committedBytes: number, totalBytes: number) => void;
};

const fallbackChunkSize = 512 * 1024;
const initialChunkSize = 256 * 1024;
const minimumChunkSize = 64 * 1024;
const targetChunkDurationMs = 15_000;
const chunkTimeoutMs = 60_000;
const maxRetries = 4;

/** Uploads one sequential, checksummed chunk at a time, adapting to request duration. */
export function createAssetUploadAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AssetUploadService, config);

  return {
    async uploadAttachment(options: UploadAttachmentOptions): Promise<UploadedAsset> {
      const fullHash = await fileSHA256(options.file);
      const created = await client.createUpload({
        roomId: options.roomId,
        filename: options.file.name || 'attachment',
        contentType: options.file.type || 'application/octet-stream',
        size: BigInt(options.file.size),
        sha256: fullHash
      });
      const upload = created.upload;
      if (!upload?.uploadId) {
        throw new Error('upload did not return an upload id');
      }

      let offset = checkedOffset(upload.committedOffset, 0, options.file.size);
      const maximum = Math.max(1, upload.maxChunkSize || fallbackChunkSize);
      const minimum = Math.min(minimumChunkSize, maximum);
      let chunkSize = Math.min(initialChunkSize, maximum);
      options.onProgress?.(offset, options.file.size);

      let chunkRetryCount = 0;
      while (offset < options.file.size) {
        const end = Math.min(offset + chunkSize, options.file.size);
        const chunk = new Uint8Array(await options.file.slice(offset, end).arrayBuffer());
        const chunkSha256 = sha256(chunk);
        const started = performance.now();
        try {
          const response = await client.uploadChunk(
            { uploadId: upload.uploadId, offset: BigInt(offset), content: chunk, chunkSha256 },
            { timeoutMs: chunkTimeoutMs }
          );
          offset = checkedOffset(response.upload?.committedOffset, end, end);
          // Include server/queue latency: this is a conservative throughput
          // estimate. Limit growth so one fast sample cannot cause a large jump.
          const duration = Math.max(1, performance.now() - started);
          chunkSize = Math.max(
            minimum,
            Math.min(
              maximum,
              chunkSize * 2,
              Math.floor((chunk.length * targetChunkDurationMs) / duration)
            )
          );
        } catch (err) {
          // An offset conflict can mean a timed-out attempt committed late.
          // Check it, but do not retry other permanent errors or auth loss.
          if (
            !isTransientUploadError(err) &&
            ConnectError.from(err).code !== Code.InvalidArgument
          ) {
            throw err;
          }
          const resumed = await retryUploadRequest(() =>
            client.getUpload({ uploadId: upload.uploadId }, { timeoutMs: chunkTimeoutMs })
          );
          const resumedOffset = checkedOffset(
            resumed.upload?.committedOffset,
            offset,
            options.file.size
          );
          if (ConnectError.from(err).code === Code.DeadlineExceeded) {
            // A full deadline is four target intervals. Reduce aggressively
            // enough to recover from a bandwidth drop within the retry budget.
            chunkSize = Math.max(
              minimum,
              Math.floor((chunk.length * targetChunkDurationMs) / chunkTimeoutMs)
            );
          }
          if (resumedOffset > offset) {
            offset = resumedOffset;
            chunkRetryCount = 0;
            options.onProgress?.(offset, options.file.size);
            continue;
          }
          if (!isTransientUploadError(err) || chunkRetryCount >= maxRetries) throw err;
          await uploadRetryDelay(chunkRetryCount++);
          continue;
        }
        chunkRetryCount = 0;
        options.onProgress?.(offset, options.file.size);
      }

      // Completion returns the same asset when its previous response was lost.
      const completed = await retryUploadRequest(() =>
        client.completeUpload({ uploadId: upload.uploadId })
      );
      if (!completed.asset?.id) {
        throw new Error('completed upload did not return an asset id');
      }
      return {
        assetId: completed.asset.id,
        filename: completed.asset.filename,
        contentType: completed.asset.contentType,
        size: completed.asset.size,
        width: completed.asset.width,
        height: completed.asset.height
      };
    }
  };
}

/** Reject missing, regressing, or out-of-file offsets instead of skipping bytes or looping. */
function checkedOffset(value: bigint | undefined, minimum: number, maximum: number): number {
  if (value === undefined || value < BigInt(minimum) || value > BigInt(maximum)) {
    throw new Error('upload returned an invalid committed offset');
  }
  return Number(value);
}

/** Retry transport failures and temporary server pressure, never authorization or validation errors. */
function isTransientUploadError(error: unknown): boolean {
  const err = ConnectError.from(error);
  return (
    err.code === Code.Unavailable ||
    err.code === Code.DeadlineExceeded ||
    err.code === Code.ResourceExhausted ||
    err.code === Code.Aborted ||
    // The server maps storage failures, including session OCC conflicts, to Internal.
    err.code === Code.Internal ||
    // Connect wraps fetch network failures as Unknown with a TypeError cause.
    (err.code === Code.Unknown && err.cause instanceof TypeError)
  );
}

/** Retry only operations whose session ID is already known. */
async function retryUploadRequest<T>(request: () => Promise<T>): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    try {
      return await request();
    } catch (err) {
      if (!isTransientUploadError(err) || attempt >= maxRetries) throw err;
      await uploadRetryDelay(attempt);
    }
  }
}

/** Bounded exponential delay with jitter avoids synchronized retries after an outage. */
async function uploadRetryDelay(attempt: number): Promise<void> {
  const delay = Math.min(8_000, 500 * 2 ** attempt) * (0.5 + Math.random() * 0.5);
  await new Promise<void>((resolve) => setTimeout(resolve, delay));
}

async function fileSHA256(file: File): Promise<string> {
  const hash = sha256.create();
  const reader = file.stream().getReader();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      hash.update(value);
    }
  } finally {
    reader.releaseLock();
  }
  return hash.hex();
}
