import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { LinkPreview } from '@chatto/api-types/api/v1/link_previews_pb';
import type { LinkPreviewView, SocialPostPreviewView } from '../timeline/linkPreviews.js';

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
  socialPost?: SocialPostPreviewView | null;
};

export function createLinkPreviewAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(MessageService, config);
  return {
    /** Read display metadata without creating an unused composer token on supporting servers. */
    async fetchLinkMetadata(
      url: string,
      options?: { signal?: AbortSignal }
    ): Promise<LinkPreviewView | null> {
      const response = await client.fetchLinkPreview({ url, omitPreviewToken: true }, options);
      return response.preview ? previewFields(response.preview) : null;
    },
    /** Fetch metadata and a token for attaching the preview to a new message. */
    async fetchLinkPreview(url: string): Promise<ComposerLinkPreview | null> {
      const response = await client.fetchLinkPreview({ url });
      return composerLinkPreview(response.preview, response.previewToken);
    }
  };
}

function composerLinkPreview(
  preview: LinkPreview | undefined,
  previewToken: string
): ComposerLinkPreview | null {
  if (!preview || !previewToken) return null;
  return { ...previewFields(preview), previewToken };
}

/** Normalize the shared metadata independently of composer-token availability. */
function previewFields(preview: LinkPreview): Omit<ComposerLinkPreview, 'previewToken'> {
  return {
    url: preview.url,
    title: preview.title || null,
    description: preview.description || null,
    imageUrl: preview.imageUrl || null,
    imageAssetId: preview.imageAssetId || null,
    siteName: preview.siteName || null,
    embedType: preview.embedType || null,
    embedId: preview.embedId || null,
    socialPost: socialPostView(preview.socialPost)
  };
}

function socialPostView(
  post: LinkPreview['socialPost'],
  quoteDepth = 0
): SocialPostPreviewView | null {
  if (!post) return null;
  return {
    provider: post.provider,
    url: post.url || null,
    author: post.author
      ? {
          displayName: post.author.displayName,
          handle: post.author.handle,
          avatarUrl: post.author.avatarUrl || null
        }
      : null,
    text: post.text,
    publishedAt: post.publishedAt?.toDate().toISOString() ?? null,
    externalLink: post.externalLink
      ? {
          url: post.externalLink.url,
          title: post.externalLink.title || null,
          description: post.externalLink.description || null,
          imageUrl: post.externalLink.imageUrl || null
        }
      : null,
    contentWarning: post.contentWarning || null,
    images: post.images.map((image) => ({
      url: image.url,
      alt: image.alt || null,
      width: image.width || null,
      height: image.height || null
    })),
    quotedPost: quoteDepth === 0 ? socialPostView(post.quotedPost, quoteDepth + 1) : null
  };
}
