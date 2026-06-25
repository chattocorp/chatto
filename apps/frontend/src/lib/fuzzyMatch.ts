/**
 * Fuzzy matching utility for @mention tab completion.
 *
 * Matches a query against a target string with scoring:
 * - Exact match: highest score
 * - Prefix match: high score
 * - Contains match: medium score
 * - Fuzzy subsequence: score based on match quality
 *
 * @param query - The search query (e.g., "fb")
 * @param target - The target to match against (e.g., "foobar")
 * @returns A positive score if matched, or null if no match
 */
export function fuzzyMatch(query: string, target: string): number | null {
  if (query.length === 0) return null;
  if (target.length === 0) return null;

  const lowerQuery = query.toLowerCase();
  const lowerTarget = target.toLowerCase();

  // Exact match - highest priority
  if (lowerQuery === lowerTarget) {
    return 1000;
  }

  // Prefix match - high priority
  if (lowerTarget.startsWith(lowerQuery)) {
    // Bonus for longer prefix coverage (more of the target matched)
    const coverage = lowerQuery.length / lowerTarget.length;
    return 500 + Math.floor(coverage * 100);
  }

  // Contains match - medium priority
  const containsIndex = lowerTarget.indexOf(lowerQuery);
  if (containsIndex !== -1) {
    // Bonus for earlier position in string
    const positionBonus = Math.max(0, 50 - containsIndex * 2);
    return 200 + positionBonus;
  }

  // Fuzzy subsequence match
  // All characters in query must appear in target in order
  const score = fuzzySubsequenceScore(lowerQuery, lowerTarget);
  return score;
}

/**
 * Calculate a fuzzy subsequence match score.
 * Characters must appear in order but don't need to be consecutive.
 *
 * Scoring:
 * - Base score for each matched character
 * - Bonus for consecutive matches
 * - Bonus for matches at word boundaries (after non-alphanumeric)
 * - Penalty for gaps between matches
 */
function fuzzySubsequenceScore(query: string, target: string): number | null {
  let queryIdx = 0;
  let prevMatchIdx = -1;
  let score = 0;
  let consecutiveBonus = 0;

  for (let targetIdx = 0; targetIdx < target.length && queryIdx < query.length; targetIdx++) {
    if (target[targetIdx] === query[queryIdx]) {
      // Base score for match
      score += 10;

      // Consecutive match bonus
      if (prevMatchIdx === targetIdx - 1) {
        consecutiveBonus += 5;
      } else {
        consecutiveBonus = 0;
      }
      score += consecutiveBonus;

      // Word boundary bonus (match after non-alphanumeric or at start)
      if (targetIdx === 0 || !isAlphanumeric(target[targetIdx - 1])) {
        score += 15;
      }

      // Penalty for gap (distance from previous match)
      if (prevMatchIdx !== -1) {
        const gap = targetIdx - prevMatchIdx - 1;
        score -= gap * 2;
      }

      prevMatchIdx = targetIdx;
      queryIdx++;
    }
  }

  // All query characters must be matched
  if (queryIdx !== query.length) {
    return null;
  }

  // Ensure minimum positive score for valid matches
  return Math.max(1, score);
}

function isAlphanumeric(char: string): boolean {
  const code = char.charCodeAt(0);
  return (
    (code >= 48 && code <= 57) || // 0-9
    (code >= 65 && code <= 90) || // A-Z
    (code >= 97 && code <= 122) // a-z
  );
}
