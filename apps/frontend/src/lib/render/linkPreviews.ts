/**
 * Link-preview data normalized for rendering. Generated protobuf timestamps
 * and empty scalar values are converted at the API boundary.
 */
export type LinkPreviewView = {
  url: string;
  title?: string | null;
  description?: string | null;
  imageUrl?: string | null;
  siteName?: string | null;
  embedType?: string | null;
  embedId?: string | null;
  socialPost?: SocialPostPreviewView | null;
};

export type SocialPostPreviewView = {
  provider: string;
  url?: string | null;
  author?: {
    displayName: string;
    handle: string;
    avatarUrl?: string | null;
  } | null;
  text: string;
  publishedAt?: string | null;
  externalLink?: {
    url: string;
    title?: string | null;
    description?: string | null;
    imageUrl?: string | null;
  } | null;
  contentWarning?: string | null;
  images: Array<{
    url: string;
    alt?: string | null;
    width?: number | null;
    height?: number | null;
  }>;
  quotedPost?: SocialPostPreviewView | null;
};
