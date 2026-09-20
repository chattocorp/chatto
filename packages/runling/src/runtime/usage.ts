export interface TokenUsage {
  /** Non-cached input tokens. */
  input: number;
  /** Output tokens (including reasoning tokens when reported). */
  output: number;
  /** Input tokens served from the prompt cache. */
  cacheRead: number;
  /** Input tokens written to the prompt cache. */
  cacheWrite: number;
  /** Provider/model-aware cost in US dollars, when reported by Pi. */
  cost?: number;
  /** Some recorded tokens have no reported price. */
  costIncomplete?: boolean;
}

export type TokenUsageInput = Omit<TokenUsage, "cost"> & {
  cost?: number | { total?: number };
};

export function emptyTokenUsage(): TokenUsage {
  return { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };
}

/** Accumulate `usage` into `total` in place. Missing or malformed usage is ignored. */
export function accumulateTokenUsage(
  total: TokenUsage,
  usage: TokenUsageInput | undefined,
): TokenUsage {
  if (!hasValidTokenCounts(usage)) {
    return total;
  }

  total.input += usage.input;
  total.output += usage.output;
  total.cacheRead += usage.cacheRead;
  total.cacheWrite += usage.cacheWrite;
  const cost = readCost(usage.cost);
  if (cost !== undefined) total.cost = (total.cost ?? 0) + cost;
  if (
    usage.costIncomplete ||
    (cost === undefined &&
      usage.input + usage.output + usage.cacheRead + usage.cacheWrite > 0)
  )
    total.costIncomplete = true;
  return total;
}

/** Sum of all tracked token counts; zero means nothing was recorded. */
export function totalTokens(usage: TokenUsage): number {
  return usage.input + usage.output + usage.cacheRead + usage.cacheWrite;
}

function formatCount(value: number): string {
  return value.toLocaleString("en-US");
}

export function formatTokenUsage(usage: TokenUsage): string {
  const parts = [
    `in ${formatCount(usage.input)}`,
    `out ${formatCount(usage.output)}`,
  ];
  if (usage.cacheRead > 0 || usage.cacheWrite > 0) {
    parts.push(`cache read ${formatCount(usage.cacheRead)}`);
    parts.push(`cache write ${formatCount(usage.cacheWrite)}`);
  }
  return parts.join(", ");
}

export function isTokenUsage(value: unknown): value is TokenUsage {
  if (!hasValidTokenCounts(value)) return false;

  const usage = value as TokenUsage;
  return (
    (usage.costIncomplete === undefined ||
      typeof usage.costIncomplete === "boolean") &&
    (usage.cost === undefined ||
      (Number.isFinite(usage.cost) && usage.cost >= 0))
  );
}

export const hasValidTokenCounts = (value: unknown): value is TokenUsageInput => {
  if (typeof value !== "object" || value === null) return false;

  const usage = value as TokenUsageInput;
  return [usage.input, usage.output, usage.cacheRead, usage.cacheWrite].every(
    (count) => Number.isSafeInteger(count) && count >= 0,
  );
};

const readCost = (cost: TokenUsageInput["cost"]): number | undefined => {
  const value = typeof cost === "number" ? cost : cost?.total;
  return value !== undefined && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
};
