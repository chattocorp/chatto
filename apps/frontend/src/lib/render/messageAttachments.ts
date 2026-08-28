import type { ExpiringAssetUrl } from '$lib/attachments/attachmentUrls';

export enum VideoProcessingStatus {
  Completed = 'COMPLETED',
  Failed = 'FAILED',
  Pending = 'PENDING',
  Processing = 'PROCESSING'
}

export type VideoVariantView = {
  quality: string;
  width: number;
  height: number;
  size: number;
  assetUrl?: ExpiringAssetUrl | null;
};

export type VideoProcessingView = {
  status: VideoProcessingStatus;
  durationMs?: number | string | null;
  width?: number | null;
  height?: number | null;
  thumbnailAssetUrl?: ExpiringAssetUrl | null;
  sourceAvailable: boolean;
  variants: VideoVariantView[];
  hlsMasterPlaylistUrl?: ExpiringAssetUrl | null;
  reasonCode?: string | null;
};

export type MessageAttachmentView = {
  id: string;
  filename: string;
  contentType: string;
  width: number;
  height: number;
  assetUrl?: ExpiringAssetUrl | null;
  thumbnailAssetUrl?: ExpiringAssetUrl | null;
  videoProcessing?: VideoProcessingView | null;
};
