import { EXACT_MATCH_SCORE, fuzzyMatch, fuzzyMatchAny, PREFIX_MATCH_SCORE } from '$lib/fuzzyMatch';
import type { RoomMember } from '$lib/state/room';
import type { MentionRole } from './autocomplete.svelte';

/** Lower groups appear first. Users win ties with room-wide handles. */
enum MatchGroup {
  ExactThreadParticipant,
  ExactUser,
  ExactVirtual,
  ThreadParticipant,
  PrefixUser,
  PrefixVirtual,
  OtherUser,
  OtherVirtual,
  Role
}

/** One ranked @mention autocomplete candidate. */
export type MentionResult =
  | { type: 'user'; handle: string; member: RoomMember; score: number; priority: MatchGroup }
  | { type: 'virtual'; handle: 'all' | 'here'; score: number; priority: MatchGroup }
  | { type: 'role'; handle: string; role: MentionRole; score: number; priority: MatchGroup };

/**
 * Rank the @mention candidates that match `query`.
 *
 * Exact member and virtual matches come first, followed by thread participants.
 * A virtual prefix ranks above weaker member matches. Roles come last.
 *
 * An empty query lists available candidates for fields that open on bare @.
 * The popup and Tab completion both use this order, so they stay consistent.
 */
export function rankMentionCandidates(
  query: string,
  members: readonly RoomMember[],
  roles: readonly MentionRole[],
  prioritizedUserIds: ReadonlySet<string> = new Set()
): MentionResult[] {
  const results: MentionResult[] = [];

  for (const member of members) {
    if (member.deleted || !member.login) continue;

    const score = query ? (fuzzyMatchAny(query, [member.login, member.displayName]) ?? 0) : 1;

    if (score > 0) {
      let priority = MatchGroup.OtherUser;
      const prioritized = prioritizedUserIds.has(member.id);
      if (score === EXACT_MATCH_SCORE) {
        priority = prioritized ? MatchGroup.ExactThreadParticipant : MatchGroup.ExactUser;
      } else if (prioritized) priority = MatchGroup.ThreadParticipant;
      else if (score >= PREFIX_MATCH_SCORE) priority = MatchGroup.PrefixUser;
      results.push({ type: 'user', handle: member.login, member, score, priority });
    }
  }

  for (const handle of ['all', 'here'] as const) {
    const score = query ? fuzzyMatch(query, handle) : 1;
    if (score && score > 0) {
      let priority = MatchGroup.OtherVirtual;
      if (score === EXACT_MATCH_SCORE) priority = MatchGroup.ExactVirtual;
      else if (score >= PREFIX_MATCH_SCORE) priority = MatchGroup.PrefixVirtual;
      results.push({ type: 'virtual', handle, score, priority });
    }
  }

  for (const role of roles) {
    if (!role.pingable || role.name === 'everyone') continue;
    const score = query ? fuzzyMatch(query, role.name) : 1;
    if (score && score > 0) {
      results.push({ type: 'role', handle: role.name, role, score, priority: MatchGroup.Role });
    }
  }

  return results.sort(
    (a, b) => a.priority - b.priority || b.score - a.score || a.handle.localeCompare(b.handle)
  );
}
