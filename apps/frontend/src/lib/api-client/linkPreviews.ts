import { authHeaders, createChattoClient, handleAuthError } from "./connect.js";
import { LinkPreviewService } from "@chatto/api-types/api/v1/link_previews_connect";
import type { LinkPreview } from "@chatto/api-types/api/v1/link_previews_pb";

export type LinkPreviewAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type ComposerLinkPreview = {
  url: string;
  title: string | null;
  description: string | null;
  imageUrl: string | null;
  imageAssetId: string | null;
  siteName: string | null;
  embedType: string | null;
  embedId: string | null;
};

export function createLinkPreviewAPI(config: LinkPreviewAPIConfig) {
  const client = createChattoClient(LinkPreviewService, config);
  const headers = () => authHeaders(config);
  return {
    async fetchLinkPreview(url: string): Promise<ComposerLinkPreview | null> {
      try {
        const response = await client.fetchLinkPreview(
          { url },
          { headers: headers() },
        );
        return composerLinkPreview(response.preview);
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
  };
}

function composerLinkPreview(
  preview: LinkPreview | undefined,
): ComposerLinkPreview | null {
  if (!preview) return null;
  return {
    url: preview.url,
    title: preview.title || null,
    description: preview.description || null,
    imageUrl: preview.imageUrl || null,
    imageAssetId: preview.imageAssetId || null,
    siteName: preview.siteName || null,
    embedType: preview.embedType || null,
    embedId: preview.embedId || null,
  };
}
