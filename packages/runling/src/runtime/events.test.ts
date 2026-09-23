import { expect, test } from "vitest";
import {
  emitRunlingEvent,
  observeRunlingEvents,
  withRunlingActivity,
  type RunlingEvent,
} from "./events.ts";
import { step } from "./step.ts";

test("delivers events with their current activity", () => {
  const events: RunlingEvent[] = [];

  observeRunlingEvents(
    (event) => events.push(event),
    () =>
      withRunlingActivity("step-1", () =>
        emitRunlingEvent({
          type: "command.started",
          id: "command-1",
          command: "pnpm test",
        }),
      ),
  );

  expect(events).toHaveLength(1);
  expect(events[0]).toMatchObject({
    type: "command.started",
    id: "command-1",
    activityId: "step-1",
    command: "pnpm test",
  });
});

test("composes nested event observers", () => {
  const outer: RunlingEvent[] = [];
  const inner: RunlingEvent[] = [];

  observeRunlingEvents((event) => outer.push(event), () => {
    observeRunlingEvents((event) => inner.push(event), () => {
      emitRunlingEvent({
        type: "command.started",
        id: "command-1",
        command: "pnpm test",
      });
    });
  });

  expect(outer).toHaveLength(1);
  expect(inner).toHaveLength(1);
});

test("tracks nested asynchronous step lifecycles", async () => {
  const events: RunlingEvent[] = [];

  await observeRunlingEvents(
    (event) => events.push(event),
    () =>
      step("Outer", async () => {
        await step("Inner", async () => Promise.resolve());
      }),
  );
  await Promise.resolve();

  const starts = events.filter((event) => event.type === "step.started");
  const finishes = events.filter((event) => event.type === "step.finished");
  expect(starts).toHaveLength(2);
  expect(finishes).toHaveLength(2);
  expect(starts[1]?.activityId).toBe(starts[0]?.id);
  expect(finishes.every((event) => event.status === "completed")).toBe(true);
});
