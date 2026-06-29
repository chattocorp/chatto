export enum FitMode {
  Contain = 'CONTAIN',
  Cover = 'COVER',
  Exact = 'EXACT'
}

export enum NotificationLevel {
  AllMessages = 'ALL_MESSAGES',
  Default = 'DEFAULT',
  Muted = 'MUTED',
  Normal = 'NORMAL'
}

export enum PresenceStatus {
  Away = 'AWAY',
  DoNotDisturb = 'DO_NOT_DISTURB',
  Offline = 'OFFLINE',
  Online = 'ONLINE'
}

export enum TimeFormat {
  Auto = 'AUTO',
  TwelveHour = 'TWELVE_HOUR',
  TwentyFourHour = 'TWENTY_FOUR_HOUR'
}

export enum VideoProcessingStatus {
  Completed = 'COMPLETED',
  Failed = 'FAILED',
  Pending = 'PENDING',
  Processing = 'PROCESSING'
}

export type AssetURL = {
  url: string;
  expiresAt: string;
};

export type LinkPreviewInput = {
  url: string;
  title?: string | null;
  description?: string | null;
  imageUrl?: string | null;
  imageAssetId?: string | null;
  siteName?: string | null;
  embedType?: string | null;
  embedId?: string | null;
};

export type LinkPreviewView = {
  url: string;
  title?: string | null;
  description?: string | null;
  imageUrl?: string | null;
  siteName?: string | null;
  embedType?: string | null;
  embedId?: string | null;
};

export type CustomUserStatusView = {
  emoji: string;
  text: string;
  expiresAt?: string | null;
};

export type UserAvatarUserView = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus | string;
  customStatus?: CustomUserStatusView | null;
};

export type VideoVariantView = {
  quality: string;
  width: number;
  height: number;
  size: number;
  assetUrl: AssetURL;
};

export type VideoProcessingView = {
  status: VideoProcessingStatus;
  durationMs?: number | string | null;
  width?: number | null;
  height?: number | null;
  thumbnailAssetUrl?: AssetURL | null;
  sourceAvailable: boolean;
  variants: VideoVariantView[];
  reasonCode?: string | null;
};

export type MessageAttachmentView = {
  id: string;
  filename: string;
  contentType: string;
  width: number;
  height: number;
  assetUrl: AssetURL;
  thumbnailAssetUrl?: AssetURL | null;
  videoProcessing?: VideoProcessingView | null;
};

export type ReactionSummaryView = {
  emoji: string;
  count: number;
  hasReacted: boolean;
  users: Array<{ id: string; displayName: string }>;
};

export type RoomEventPayload = {
  kind: string;
  [key: string]: unknown;
};

export type RoomEventView = {
  id: string;
  createdAt: string;
  actorId?: string | null;
  actor?: UserAvatarUserView | null;
  event: RoomEventPayload | null;
};
