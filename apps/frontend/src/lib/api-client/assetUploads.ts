import { sha256 } from 'js-sha256';
import { AssetUploadService } from '@chatto/api-types/api/v1/asset_uploads_connect';
import {
  authHeaders,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig
} from './connect.js';

export type UploadedAttachmentAsset = {
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
  threadRootEventId?: string | null;
  alsoSendToChannel?: boolean;
  onProgress?: (committedBytes: number, totalBytes: number) => void;
};

const fallbackChunkSize = 512 * 1024;

export function createAssetUploadAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AssetUploadService, config);
  const headers = () => authHeaders(config);

  return {
    async uploadAttachment(options: UploadAttachmentOptions): Promise<UploadedAttachmentAsset> {
      try {
        const fullHash = await fileSHA256(options.file);
        const created = await client.createUpload(
          {
            roomId: options.roomId,
            filename: options.file.name || 'attachment',
            contentType: options.file.type || 'application/octet-stream',
            size: BigInt(options.file.size),
            sha256: fullHash,
            threadRootEventId: options.threadRootEventId ?? '',
            alsoSendToChannel: options.alsoSendToChannel ?? false
          },
          { headers: headers() }
        );
        const upload = created.upload;
        if (!upload?.uploadId) {
          throw new Error('upload did not return an upload id');
        }

        let offset = Number(upload.committedOffset);
        const chunkSize = Math.max(1, upload.maxChunkSize || fallbackChunkSize);
        options.onProgress?.(offset, options.file.size);

        let chunkRetryCount = 0;
        while (offset < options.file.size) {
          const end = Math.min(offset + chunkSize, options.file.size);
          const chunk = new Uint8Array(await options.file.slice(offset, end).arrayBuffer());
          try {
            const response = await client.uploadChunk(
              {
                uploadId: upload.uploadId,
                offset: BigInt(offset),
                content: chunk,
                chunkSha256: sha256(chunk)
              },
              { headers: headers() }
            );
            offset = Number(response.upload?.committedOffset ?? BigInt(end));
            chunkRetryCount = 0;
            options.onProgress?.(offset, options.file.size);
          } catch (err) {
            const resumed = await client.getUpload(
              { uploadId: upload.uploadId },
              { headers: headers() }
            );
            const resumedOffset = Number(resumed.upload?.committedOffset ?? BigInt(offset));
            if (resumedOffset > offset && resumedOffset <= options.file.size) {
              offset = resumedOffset;
              chunkRetryCount = 0;
              options.onProgress?.(offset, options.file.size);
              continue;
            }
            if (chunkRetryCount < 2) {
              chunkRetryCount += 1;
              continue;
            }
            throw err;
          }
        }

        const completed = await client.completeUpload(
          { uploadId: upload.uploadId },
          { headers: headers() }
        );
        if (!completed.asset?.assetId) {
          throw new Error('completed upload did not return an asset id');
        }
        return {
          assetId: completed.asset.assetId,
          filename: completed.asset.filename,
          contentType: completed.asset.contentType,
          size: completed.asset.size,
          width: completed.asset.width,
          height: completed.asset.height
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
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
