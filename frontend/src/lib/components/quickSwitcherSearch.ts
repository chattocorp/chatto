import { fuzzyMatch } from '$lib/fuzzyMatch';

export type Searchable = {
  label: string;
  detail: string;
  instanceName: string;
};

const DETAIL_WEIGHT = 0.5;
const INSTANCE_WEIGHT = 0.4;

/**
 * Score a Quick Switcher item against a multi-token query.
 *
 * Whitespace-separated tokens are matched independently; each token must
 * match at least one of `label`, `detail`, or `instanceName` (best-of-three,
 * weighted). All tokens must match for the item to be a candidate, and the
 * total score is the sum of per-token best matches.
 *
 * Returns `null` when the query is empty or any token fails to match.
 */
export function scoreItem(query: string, item: Searchable): number | null {
  const tokens = query.split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return null;

  let total = 0;
  for (const token of tokens) {
    const labelScore = fuzzyMatch(token, item.label) ?? 0;
    const detailScore = (fuzzyMatch(token, item.detail) ?? 0) * DETAIL_WEIGHT;
    const instanceScore = (fuzzyMatch(token, item.instanceName) ?? 0) * INSTANCE_WEIGHT;
    const tokenBest = Math.max(labelScore, detailScore, instanceScore);
    if (tokenBest === 0) return null;
    total += tokenBest;
  }
  return total;
}
