import { serverRegistry } from '$lib/state/server/registry.svelte';
import { primeUserSummaryCache } from '$lib/state/userSummaries.svelte';
import type { UserSummaryForCache } from '@chatto/api-client/events';
import type { RoomTimelineAPIConfig } from '@chatto/api-client/roomTimeline';

type AuthenticationAwareConfig = {
  onAuthenticationRequired?: (serverId: string) => void;
};

export function withAuthenticationRequired<TConfig extends AuthenticationAwareConfig>(
  config: TConfig
): TConfig {
  return {
    ...config,
    onAuthenticationRequired(serverId) {
      config.onAuthenticationRequired?.(serverId);
      serverRegistry.handleAuthenticationRequired(serverId);
    }
  };
}

export function withRoomTimelineHooks<TConfig extends RoomTimelineAPIConfig>(
  config: TConfig
): TConfig {
  return {
    ...withAuthenticationRequired(config),
    onUserSummaries(serverId, users: UserSummaryForCache[]) {
      config.onUserSummaries?.(serverId, users);
      primeUserSummaryCache(serverId, users);
    }
  };
}
