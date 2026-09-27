import { fuzzyMatch } from '$lib/fuzzyMatch';
import type { RoomMember } from '$lib/state/room';
import type { MentionRole } from './autocomplete.svelte';

/**
 * Fuzzy score of an exact, case-insensitive match. See `fuzzyMatch`. A long
 * fuzzy match can score higher, so compare for equality.
 */
const EXACT_MATCH_SCORE = 1000;

/** One ranked @mention autocomplete candidate. */
export type MentionResult =
  | { type: 'user'; handle: string; member: RoomMember; score: number; priority: number }
  | { type: 'virtual'; handle: 'all' | 'here'; score: number; priority: number }
  | { type: 'role'; handle: string; role: MentionRole; score: number; priority: number };

/**
 * Rank the @mention candidates that match `query`.
 *
 * Users rank before the virtual `@all` and `@here` handles, and those rank
 * before pingable roles. Among users, an exact login or display-name match
 * ranks first, then users in `prioritizedUserIds` (for example, participants
 * of the current thread), then the fuzzy score. The handle breaks ties.
 *
 * The popup and Tab completion both use this order, so they stay consistent.
 */
export function rankMentionCandidates(
  query: string,
  members: readonly RoomMember[],
  roles: readonly MentionRole[],
  prioritizedUserIds: ReadonlySet<string> = new Set()
): MentionResult[] {
  const users: { result: MentionResult; exact: boolean; prioritized: boolean }[] = [];
  const others: MentionResult[] = [];

  for (const member of members) {
    if (member.deleted || !member.login) continue;

    const loginScore = fuzzyMatch(query, member.login);
    const displayScore = fuzzyMatch(query, member.displayName);
    const score = Math.max(loginScore ?? -1, displayScore ?? -1);

    if (score > 0) {
      users.push({
        result: { type: 'user', handle: member.login, member, score, priority: 0 },
        exact: loginScore === EXACT_MATCH_SCORE || displayScore === EXACT_MATCH_SCORE,
        prioritized: prioritizedUserIds.has(member.id)
      });
    }
  }

  for (const handle of ['all', 'here'] as const) {
    const score = fuzzyMatch(query, handle);
    if (score && score > 0) {
      others.push({ type: 'virtual', handle, score, priority: 1 });
    }
  }

  for (const role of roles) {
    if (!role.pingable || role.name === 'everyone') continue;
    const score = fuzzyMatch(query, role.name);
    if (score && score > 0) {
      others.push({ type: 'role', handle: role.name, role, score, priority: 2 });
    }
  }

  users.sort(
    (a, b) =>
      Number(b.exact) - Number(a.exact) ||
      Number(b.prioritized) - Number(a.prioritized) ||
      b.result.score - a.result.score ||
      a.result.handle.localeCompare(b.result.handle)
  );
  others.sort(
    (a, b) => a.priority - b.priority || b.score - a.score || a.handle.localeCompare(b.handle)
  );

  return [...users.map((user) => user.result), ...others];
}
