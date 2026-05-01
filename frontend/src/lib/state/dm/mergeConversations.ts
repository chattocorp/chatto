/**
 * Pure merge helper for the DM conversation list.
 *
 * Goals:
 * - Replacing entries for one instance must NOT move entries for OTHER
 *   instances around. Otherwise a per-instance refetch (e.g. from
 *   `bumpConversationToTop`'s "unknown room" path) destroys the
 *   chronological ordering established by previous bumps.
 * - Genuinely new conversations (server returned them but we hadn't seen
 *   them locally) go to the front, since the server returns them sorted
 *   most-recent-first.
 * - Conversations that no longer exist server-side for this instance are
 *   dropped.
 */
export function mergeInstanceConversations<T extends { id: string; instanceId: string }>(
  existing: T[],
  instanceId: string,
  loaded: T[]
): T[] {
  const newById = new Map(loaded.map((c) => [c.id, c]));
  const seenIds = new Set<string>();
  const merged: T[] = [];

  for (const conv of existing) {
    if (conv.instanceId !== instanceId) {
      merged.push(conv);
      continue;
    }
    const replacement = newById.get(conv.id);
    if (replacement) {
      merged.push(replacement);
      seenIds.add(conv.id);
    }
    // else: dropped (no longer exists server-side for this instance)
  }

  const additions = loaded.filter((c) => !seenIds.has(c.id));
  return [...additions, ...merged];
}
