/**
 * Shared mapping from the generated `User` protobuf message to the plain
 * user-summary shape used across API client modules.
 *
 * This module is intentionally a leaf: it imports generated protobuf types
 * only, so any api-client module can depend on it without shifting runtime
 * chunk graphs. Every caller previously re-implemented these field maps by
 * hand; this is now the single place that decides missing-value fallbacks
 * (`avatarUrl` empty string becomes `null`) and presence/customStatus views.
 */

import type { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { presenceStatusOrOffline } from './enumDefaults.js';

/** Lightweight identity snapshot of one user, safe to cache and prime stores with. */
export type UserSummary = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  isBot?: boolean;
  avatarUrl: string | null;
  /** Public self-authored bio; `null` when unset. */
  bio?: string | null;
  /** Public IANA time zone the user shares; `null` when unset. */
  timezone?: string | null;
};

/**
 * Map a generated user to its summary.
 *
 * Normalization contract: an unset or empty-string protobuf avatar URL maps
 * to `null` (proto3 strings default to `''`, so "unset" and "empty" are the
 * same fact). Callers that previously emitted `''` (`?? null`) now emit
 * `null`; consumers treat both as no-avatar through truthiness checks.
 */
export function mapUserSummary(user: APIUser): UserSummary {
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    deleted: user.deleted,
    isBot: user.isBot,
    avatarUrl: user.avatarUrl || null,
    bio: user.bio || null,
    timezone: user.timezone || null
  };
}

/**
 * Map an optional generated user; `null` means the response omitted the user.
 */
export function mapOptionalUserSummary(user: APIUser | undefined): UserSummary | null {
  return user ? mapUserSummary(user) : null;
}

/** Live-presence view of one user: normalized presence plus custom status. */
export type UserPresenceView = {
  presenceStatus: PresenceStatus;
  customStatus: {
    emoji: string;
    text: string;
    expiresAt: string | null;
  } | null;
};

/**
 * Map a generated user's presence and custom status for UI rendering.
 * Unknown/unset presence falls back to offline; custom status timestamps are
 * converted to ISO strings.
 */
export function mapUserPresenceView(user: APIUser | undefined): UserPresenceView {
  return {
    presenceStatus: presenceStatusOrOffline(user?.presenceStatus ?? PresenceStatus.UNSPECIFIED),
    customStatus: user?.customStatus
      ? {
          emoji: user.customStatus.emoji,
          text: user.customStatus.text,
          expiresAt: user.customStatus.expiresAt?.toDate().toISOString() ?? null
        }
      : null
  };
}
