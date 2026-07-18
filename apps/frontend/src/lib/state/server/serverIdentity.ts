/** Return the canonical origin used as the portable identity of a Chatto server. */
export function canonicalServerOrigin(url: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return url.replace(/\/$/, '');
  }
}

/** Generate a URL-safe, device-local server ID, resolving local collisions. */
export function generateServerId(url: string, existingIds: string[] = []): string {
  let hostname: string;
  try {
    hostname = new URL(url).hostname;
  } catch {
    hostname = url.replace(/[^a-z0-9-]/gi, '-');
  }

  const base = hostname.replace(/\./g, '-').replace(/^-+|-+$/g, '');
  if (!existingIds.includes(base)) return base;

  let suffix = 2;
  while (existingIds.includes(`${base}-${suffix}`)) suffix++;
  return `${base}-${suffix}`;
}

/** Keep synced discovery metadata from modifying local routing or credentials. */
export function portableMetadataUpdate(server: { name: string; iconUrl?: string }): {
  name: string;
  iconUrl: string | null;
} {
  return { name: server.name, iconUrl: server.iconUrl ?? null };
}

/**
 * Treat absence as deletion only after the entry appeared in a successful
 * device-local sync baseline. An empty baseline therefore produces a union.
 */
export function wasRemovedSinceLastSync(
  origin: string,
  previousOrigins: ReadonlySet<string>,
  currentOrigins: ReadonlySet<string>
): boolean {
  return previousOrigins.has(origin) && !currentOrigins.has(origin);
}

export type PendingHomeMove = {
  newOrigin: string;
  previousUserId: string;
};

/** Collapse pending redirects onto the latest selected home without cycles. */
export function recordPendingHomeMove(
  moves: Record<string, PendingHomeMove>,
  previousOrigin: string,
  newOrigin: string,
  previousUserId: string
): Record<string, PendingHomeMove> {
  const next = Object.fromEntries(
    Object.entries(moves).map(([origin, move]) => [origin, { ...move }])
  );
  for (const [origin, move] of Object.entries(next)) {
    if (move.newOrigin === previousOrigin) move.newOrigin = newOrigin;
    if (origin === move.newOrigin || origin === newOrigin) delete next[origin];
  }
  if (previousOrigin !== newOrigin) next[previousOrigin] = { newOrigin, previousUserId };
  return next;
}
