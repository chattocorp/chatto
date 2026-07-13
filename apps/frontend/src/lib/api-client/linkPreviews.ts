import { authHeaders, createChattoClient, handleAuthError } from './connect.js';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { LinkPreview } from '@chatto/api-types/api/v1/link_previews_pb';
import type { ImportedLinkAttachment } from '@chatto/api-types/api/v1/link_previews_pb';

export type LinkPreviewAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type ComposerLinkPreview = {
  url: string;
  previewToken: string;
  title: string | null;
  description: string | null;
  imageUrl: string | null;
  imageAssetId: string | null;
  siteName: string | null;
  embedType: string | null;
  embedId: string | null;
};

export type ComposerImportedAttachment = {
  assetId: string;
  filename: string;
  contentType: string;
  size: bigint;
  width: number;
  height: number;
  previewUrl: string;
};

export type ComposerLinkResult =
  | { kind: 'preview'; preview: ComposerLinkPreview }
  | { kind: 'attachment'; attachment: ComposerImportedAttachment };

export function createLinkPreviewAPI(config: LinkPreviewAPIConfig) {
  const client = createChattoClient(MessageService, config);
  const headers = () => authHeaders(config);
  return {
    async fetchLinkPreview(url: string, roomId?: string): Promise<ComposerLinkResult | null> {
      try {
        const response = await client.fetchLinkPreview({ url, roomId }, { headers: headers() });
        const attachment = composerImportedAttachment(response.importedAttachment);
        if (attachment) return { kind: 'attachment', attachment };
        const preview = composerLinkPreview(response.preview, response.previewToken);
        return preview ? { kind: 'preview', preview } : null;
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

function composerLinkPreview(
  preview: LinkPreview | undefined,
  previewToken: string
): ComposerLinkPreview | null {
  if (!preview || !previewToken) return null;
  return {
    url: preview.url,
    previewToken,
    title: preview.title || null,
    description: preview.description || null,
    imageUrl: preview.imageUrl || null,
    imageAssetId: preview.imageAssetId || null,
    siteName: preview.siteName || null,
    embedType: preview.embedType || null,
    embedId: preview.embedId || null
  };
}

function composerImportedAttachment(
  asset: ImportedLinkAttachment | undefined
): ComposerImportedAttachment | null {
  if (!asset?.assetId || !asset.previewUrl) return null;
  return {
    assetId: asset.assetId,
    filename: asset.filename,
    contentType: asset.contentType,
    size: asset.size,
    width: asset.width,
    height: asset.height,
    previewUrl: asset.previewUrl
  };
}
