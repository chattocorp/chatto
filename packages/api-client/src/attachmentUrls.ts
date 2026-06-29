export type ExpiringAssetUrl = {
  url: string;
  expiresAt: string;
};

export type RefreshedAttachmentUrls = {
  assetUrl: ExpiringAssetUrl;
  thumbnailAssetUrl: ExpiringAssetUrl | null;
  videoThumbnailAssetUrl: ExpiringAssetUrl | null;
  variantAssetUrls: Map<string, ExpiringAssetUrl>;
};
