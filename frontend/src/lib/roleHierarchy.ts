/**
 * Frontend mirror of the backend's role-hierarchy comparison.
 *
 * Position scheme (matches `cli/internal/core/rbac/engine.go`):
 * higher position = higher rank.
 *   owner=1000, admin=900, moderator=100, custom roles=1..99, everyone=0.
 *
 * The backend's `OutranksUser` is the source of truth for any mutation that
 * gates on rank. These helpers exist only for optimistic UI hints: they
 * answer "should we even show this affordance" so the user isn't presented
 * with a button that the mutation would reject. The server re-checks.
 */

export type RankRole = {
  name: string;
  position: number;
};

/**
 * Highest-rank position among the named roles. Returns `-Infinity` when the
 * role list is empty (i.e. the user holds no explicit roles), which makes
 * comparisons against `-Infinity` resolve sensibly: an unranked user is
 * outranked by anyone with even an `everyone`-position role.
 */
export function bestPosition(roleNames: string[], roles: RankRole[]): number {
  if (roleNames.length === 0) return -Infinity;
  const positions = roleNames.map((name) => {
    const role = roles.find((r) => r.name === name);
    return role?.position ?? -Infinity;
  });
  return Math.max(...positions);
}

/**
 * Whether the viewer strictly outranks the target — the rank half of the
 * "permission AND rank" gate for targeted user mutations (see
 * `.claude/rules/authorization.md`). Returns true only when the viewer's
 * best role has a strictly higher position than the target's best role.
 * Peer ranks return false, matching the backend's strict-outrank semantics
 * (peer-admins cannot manage each other).
 */
export function viewerOutranksTarget(
  viewerRoleNames: string[],
  targetRoleNames: string[],
  roles: RankRole[]
): boolean {
  return bestPosition(viewerRoleNames, roles) > bestPosition(targetRoleNames, roles);
}
