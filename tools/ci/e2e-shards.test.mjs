import assert from "node:assert/strict";
import test from "node:test";
import { balance, inventory } from "./e2e-shards.mjs";

test("all collected files, including new files, run exactly once", () => {
  const files = [
    { file: "slow.ts", count: 2 },
    { file: "fast.ts", count: 4 },
    { file: "new.ts", count: 1 },
  ];
  const shards = balance(
    files,
    { "slow.ts": 20000, "fast.ts": 1000, "deleted.ts": 500 },
    4,
  );
  assert.deepEqual(
    shards.flatMap((shard) => shard.files).sort(),
    files.map(({ file }) => file).sort(),
  );
  assert.deepEqual(
    balance(
      [...files].reverse(),
      { "slow.ts": 20000, "fast.ts": 1000, "deleted.ts": 500 },
      4,
    ),
    shards,
  );
});
test("long and short files balance by time instead of equal test counts", () => {
  const shards = balance(
    ["a", "b", "c", "d"].map((file) => ({ file, count: 1 })),
    { a: 9, b: 8, c: 2, d: 1 },
    2,
  );
  assert.deepEqual(
    shards.map((shard) => shard.durationMs),
    [10, 10],
  );
});
test("inventory includes nested and parameterized tests", () => {
  assert.deepEqual(
    inventory({
      suites: [
        {
          specs: [{ file: "e2e/a.test.ts", tests: [{}, {}] }],
          suites: [{ specs: [{ file: "a.test.ts", tests: [{}] }] }],
        },
      ],
    }),
    [{ file: "a.test.ts", count: 3 }],
  );
});
test("invalid hints and shard counts fail instead of omitting tests", () => {
  assert.throws(() => balance([], {}, 0));
  assert.throws(() => balance([], { bad: -1 }, 4));
});
