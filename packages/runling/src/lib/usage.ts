import type { TokenUsage } from "runling";

export const tokenCount = (usage: TokenUsage): number =>
  usage.input + usage.output + usage.cacheRead + usage.cacheWrite;

export function mergeUsage(items: TokenUsage[]): TokenUsage | undefined {
  if (!items.length) return undefined;
  const total: TokenUsage = {
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheWrite: 0,
  };
  for (const usage of items) {
    total.input += usage.input;
    total.output += usage.output;
    total.cacheRead += usage.cacheRead;
    total.cacheWrite += usage.cacheWrite;
    if (usage.cost !== undefined) total.cost = (total.cost ?? 0) + usage.cost;
    if (
      usage.costIncomplete ||
      (usage.cost === undefined && tokenCount(usage) > 0)
    )
      total.costIncomplete = true;
  }
  return total;
}

export function estimatedCost(usage: TokenUsage): string {
  if (usage.cost === undefined) return "Cost unknown";
  return `$${usage.cost.toFixed(4)} est.${usage.costIncomplete ? " (partial)" : ""}`;
}
