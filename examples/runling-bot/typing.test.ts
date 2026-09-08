import assert from "node:assert/strict";
import { test } from "node:test";
import { startTyping } from "./typing.ts";

test("refreshes every three seconds and stops without another update", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let updates = 0;
  const stop = await startTyping(async () => {
    updates++;
  });
  assert.equal(updates, 1);
  t.mock.timers.tick(3000);
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(updates, 2);
  stop();
  stop();
  t.mock.timers.tick(30_000);
  assert.equal(updates, 2);
});

test("does not overlap refreshes or reschedule after stopping an in-flight update", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let updates = 0;
  let finish!: () => void;
  const stop = await startTyping(async () => {
    updates++;
    if (updates > 1)
      await new Promise<void>((resolve) => {
        finish = resolve;
      });
  });
  t.mock.timers.tick(3000);
  t.mock.timers.tick(30_000);
  assert.equal(updates, 2);
  stop();
  finish();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  t.mock.timers.tick(30_000);
  assert.equal(updates, 2);
});

test("typing failures are best effort", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const stop = await startTyping(async () => {
    throw new Error("Unavailable");
  });
  stop();
});
