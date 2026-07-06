export type ExpiringAssetUrl = {
  url: string;
  expiresAt: string;
};

export type RefreshedAttachmentUrls = {
  assetUrl: ExpiringAssetUrl | null;
  thumbnailAssetUrl: ExpiringAssetUrl | null;
  videoThumbnailAssetUrl: ExpiringAssetUrl | null;
  variantAssetUrls: Map<string, ExpiringAssetUrl | null>;
};
