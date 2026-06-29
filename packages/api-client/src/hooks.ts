import type { UserSummaryForCache } from "./events.js";

export type ApiClientHooks = {
  onAuthenticationRequired?: (serverId: string) => void;
  onUserSummaries?: (
    serverId: string | undefined,
    users: UserSummaryForCache[],
  ) => void;
};

let configuredHooks: ApiClientHooks = {};

export function configureApiClientHooks(hooks: ApiClientHooks): void {
  configuredHooks = hooks;
}

export function notifyAuthenticationRequired(
  serverId: string | undefined,
  localHook?: (serverId: string) => void,
): void {
  if (!serverId) return;
  localHook?.(serverId);
  configuredHooks.onAuthenticationRequired?.(serverId);
}

export function notifyUserSummaries(
  serverId: string | undefined,
  users: UserSummaryForCache[],
  localHook?: (
    serverId: string | undefined,
    users: UserSummaryForCache[],
  ) => void,
): void {
  localHook?.(serverId, users);
  configuredHooks.onUserSummaries?.(serverId, users);
}
