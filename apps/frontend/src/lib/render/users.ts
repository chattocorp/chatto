import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

export type CustomUserStatusView = {
  emoji: string;
  text: string;
  expiresAt?: string | null;
};

/**
 * The narrow user shape shared by avatar-bearing chat surfaces.
 */
export type UserAvatarUserView = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl?: string | null;
  presenceStatus: PresenceStatus;
  customStatus?: CustomUserStatusView | null;
};
