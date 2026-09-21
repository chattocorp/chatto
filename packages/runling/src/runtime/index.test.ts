import { expect, test } from "vitest";
import * as runling from "./index.ts";

test("keeps run accounting controls out of the public API", () => {
  expect(runling).not.toHaveProperty("recordTokenUsage");
  expect(runling).not.toHaveProperty("resetTokenUsage");
  expect(runling).not.toHaveProperty("getRecordedTokenUsage");
  expect(runling).not.toHaveProperty("withTokenUsage");
});

test("retains public helpers for working with returned usage", () => {
  const usage = runling.emptyTokenUsage();
  runling.accumulateTokenUsage(usage, {
    input: 10, output: 2, cacheRead: 3, cacheWrite: 0, cost: 0.01,
  });
  expect(runling.isTokenUsage(usage)).toBe(true);
  expect(runling.totalTokens(usage)).toBe(15);
  expect(runling.formatTokenUsage(usage)).toContain("in 10, out 2");
  expect(usage.cost).toBe(0.01);
});

test("exports explicit context creation and direct helpers", () => {
  expect(runling).not.toHaveProperty("createRunling");
  expect(runling.createWorkflowContext().usage).toEqual(runling.emptyTokenUsage());
  for (const name of ["exec", "shell", "input", "agent", "runAgent", "step"] as const) {
    expect(runling[name]).toBeTypeOf("function");
  }
});
