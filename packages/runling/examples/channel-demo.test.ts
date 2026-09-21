import { expect, test } from "vitest";
import { createWorkflowContext, runWorkflow } from "runling";
import demo from "./channel-demo.ts";

test("the parent changes a running child's instructions and receives ordered data", async () => {
  const result = await demo(createWorkflowContext(), { initial: 4 });

  expect(result).toEqual({
    label: "Updated by parent",
    total: 15,
    updates: [
      { label: "Counting", total: 5 },
      { label: "Updated by parent", total: 5 },
      { label: "Updated by parent", total: 15 },
    ],
  });
});

test("independent demo executions have separate state", async () => {
  const [a, b] = await Promise.all([
    runWorkflow(demo, { input: {} }),
    runWorkflow(demo, { input: { initial: 100 } }),
  ]);

  expect(a.output?.total).toBe(11);
  expect(b.output?.total).toBe(111);
});
