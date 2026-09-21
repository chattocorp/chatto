import { expect, test } from "vitest";
import { estimatedCost, mergeUsage, tokenCount } from "./usage.ts";

test("counts cache tokens and marks mixed pricing as partial in either order", () => {
  const priced = {
    input: 10,
    output: 2,
    cacheRead: 20,
    cacheWrite: 3,
    cost: 0.025,
  };
  const unknown = { input: 5, output: 1, cacheRead: 0, cacheWrite: 0 };
  for (const items of [
    [priced, unknown],
    [unknown, priced],
  ]) {
    const total = mergeUsage(items)!;
    expect(tokenCount(total)).toBe(41);
    expect(estimatedCost(total)).toBe("$0.0250 est. (partial)");
  }
  expect(estimatedCost(unknown)).toBe("Cost unknown");
  expect(estimatedCost({ ...unknown, cost: 0 })).toBe("$0.0000 est.");
  expect(mergeUsage([])).toBeUndefined();
});
