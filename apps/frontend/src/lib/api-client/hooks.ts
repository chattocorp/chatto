import type { UserSummary } from './userSummary.js';

/** Cache-priming user snapshot; structurally the shared `UserSummary`. */
export type UserSummaryForCache = UserSummary;

/** Read user summaries at an optional realtime consistency boundary. */
export type UserSummaryReader = (ids: string[], minimumCursor?: string) => Promise<UserSummary[]>;

export type ApiClientHooks = {
  onAuthenticationRequired?: (serverId: string) => void;
  onUserSummaries?: (serverId: string | undefined, users: UserSummaryForCache[]) => void;
  /** Own cache reads, writes, and invalidation fences for message user data. */
  resolveUserSummaries?: (
    serverId: string, ids: string[], read: UserSummaryReader, minimumCursor?: string
  ) => Promise<UserSummary[]>;
};

let configuredHooks: ApiClientHooks = {};

export function configureApiClientHooks(hooks: ApiClientHooks): void {
  configuredHooks = hooks;
}

/** Reuse session-owned user summaries when the application has installed its cache. */
export async function resolveUserSummaries(
  serverId: string | undefined, ids: string[], read: UserSummaryReader, minimumCursor?: string,
  localHook?: (serverId: string | undefined, users: UserSummaryForCache[]) => void
): Promise<UserSummary[]> {
  if (serverId && configuredHooks.resolveUserSummaries) {
    const users = await configuredHooks.resolveUserSummaries(serverId, ids, read, minimumCursor);
    // The resolver owns cache writes and their fences. Do not prime the same
    // result again after yielding: a profile event can invalidate it meanwhile.
    localHook?.(serverId, users);
    return users;
  }
  const users = await read(ids, minimumCursor);
  notifyUserSummaries(serverId, users, localHook);
  return users;
}

export function notifyAuthenticationRequired(
  serverId: string | undefined,
  localHook?: (serverId: string) => void
): void {
  if (!serverId) return;
  localHook?.(serverId);
  configuredHooks.onAuthenticationRequired?.(serverId);
}

export function notifyUserSummaries(
  serverId: string | undefined,
  users: UserSummaryForCache[],
  localHook?: (serverId: string | undefined, users: UserSummaryForCache[]) => void
): void {
  localHook?.(serverId, users);
  configuredHooks.onUserSummaries?.(serverId, users);
}
