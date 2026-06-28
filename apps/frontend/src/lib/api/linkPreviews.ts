import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { LinkPreviewService } from '$lib/pb/chatto/api/v1/link_previews_connect';
import type { LinkPreview } from '$lib/pb/chatto/api/v1/link_previews_pb';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type LinkPreviewAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
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
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(LinkPreviewService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
      serverRegistry.handleAuthenticationRequired(config.serverId);
    }
    throw err;
  }

  return {
    async fetchLinkPreview(url: string): Promise<ComposerLinkPreview | null> {
      try {
        const response = await client.fetchLinkPreview({ url }, { headers: headers() });
        return composerLinkPreview(response.preview);
      } catch (err) {
        return handleAuthError(err);
      }
    }
  };
}

function composerLinkPreview(preview: LinkPreview | undefined): ComposerLinkPreview | null {
  if (!preview) return null;
  return {
    url: preview.url,
    title: preview.title || null,
    description: preview.description || null,
    imageUrl: preview.imageUrl || null,
    imageAssetId: preview.imageAssetId || null,
    siteName: preview.siteName || null,
    embedType: preview.embedType || null,
    embedId: preview.embedId || null
  };
}
