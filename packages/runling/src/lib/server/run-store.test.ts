import { log } from "runling";
import { afterEach, expect, test } from "vitest";
import {
  mkdir,
  mkdtemp,
  readFile,
  writeFile,
  rm,
  readdir,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import { task, Type } from "runling";
import { historyDirectory, RunStore } from "./run-store.ts";
import { buildTimeline } from "../timeline.ts";
import type { RunRecord } from "../runs.ts";

const directories: string[] = [];
afterEach(async () => {
  for (const dir of directories.splice(0))
    await rm(dir, { recursive: true, force: true });
});
async function store() {
  const directory = await mkdtemp(resolve(tmpdir(), "runling-runs-test-"));
  directories.push(directory);
  const store = new RunStore(directory);
  await store.init();
  return store;
}

test("uses Runling history for new projects and preserves existing Factory history", async () => {
  const cwd = await mkdtemp(resolve(tmpdir(), "runling-history-test-"));
  directories.push(cwd);
  const current = resolve(cwd, ".runling/runs");
  const legacy = resolve(cwd, ".factory/runs");
  expect(await historyDirectory(cwd)).toBe(current);
  await mkdir(legacy, { recursive: true });
  expect(await historyDirectory(cwd)).toBe(legacy);
  await mkdir(current, { recursive: true });
  expect(await historyDirectory(cwd)).toBe(current);
});

test("streams ordered nested events and restores the completed run", async () => {
  const original = await store();
  const records: RunRecord[] = [];
  const unsubscribe = original.subscribe((_id, record) => records.push(record));
  const nested = task(
    { name: "Nested", input: Type.String(), output: Type.String() },
    async (ctx, input) => {
      log.info("Inside nested workflow");
      return input.toUpperCase();
    },
  );
  const parent = task(
    { name: "Parent", input: Type.String(), output: Type.String() },
    (ctx, input) => nested(ctx, input),
  );
  const run = await original.start("test", parent, "hello", "web");
  await run.completion;
  unsubscribe();
  const result = (await original.get(run.id))!;
  expect(result.status).toBe("completed");
  expect(result.output).toBe("HELLO");
  expect(records[0]?.type).toBe("started");
  expect(records.at(-1)?.type).toBe("finished");
  const timeline = buildTimeline(result.events, result.status);
  expect(timeline[0]?.label).toBe("Parent");
  expect(timeline[0]?.children[0]?.label).toBe("Nested");
  expect(timeline[0]?.children[0]?.status).toBe("completed");
  expect(timeline[0]?.children[0]?.logs).toContain("Inside nested workflow");
  const restored = new RunStore(original.directory);
  await restored.init();
  expect(await restored.get(run.id)).toEqual(JSON.parse(JSON.stringify(result)));
  expect(await restored.get("../../outside")).toBeUndefined();
});

test("records failures and keeps concurrent token totals separate", async () => {
  const history = await store();
  const ready = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const slow = task(
    { name: "Slow", input: Type.String(), output: Type.String() },
    async (ctx) => {
      ctx.recordUsage({ input: 10, output: 1, cacheRead: 0, cacheWrite: 0 });
      ready.resolve();
      await release.promise;
      ctx.recordUsage({ input: 20, output: 2, cacheRead: 0, cacheWrite: 0 });
      return "Slow done";
    },
  );
  const fast = task(
    { name: "Fast", input: Type.String(), output: Type.String() },
    (ctx) => {
      ctx.recordUsage({ input: 100, output: 5, cacheRead: 0, cacheWrite: 0 });
      throw new Error("Expected failure");
    },
  );
  const first = await history.start("slow", slow, "", "webhook");
  await ready.promise;
  const second = await history.start("fast", fast, "", "web");
  await second.completion;
  release.resolve();
  await first.completion;
  expect((await history.get(first.id))?.usage.input).toBe(30);
  expect((await history.get(second.id))?.usage.input).toBe(100);
  expect((await history.get(second.id))?.error).toBe("Expected failure");
  expect((await history.get(second.id))?.status).toBe("failed");
});

test("recovers a truncated journal as interrupted and saves the recovery", async () => {
  const history = await store();
  const quick = task(
    { name: "Quick", input: Type.String(), output: Type.String() },
    (ctx) => "done",
  );
  const run = await history.start("quick", quick, "", "web");
  await run.completion;
  const path = resolve(history.directory, `${run.id}.jsonl`);
  const lines = (await readFile(path, "utf8")).trimEnd().split("\n");
  await writeFile(path, `${lines.slice(0, -1).join("\n")}\n{"type":`);
  const recovered = new RunStore(history.directory);
  await recovered.init();
  expect((await recovered.get(run.id))?.status).toBe("interrupted");
  expect(
    buildTimeline((await recovered.get(run.id))!.events, "interrupted")[0]?.status,
  ).toBe("completed");
  const again = new RunStore(history.directory);
  await again.init();
  expect(await again.get(run.id)).toEqual(await recovered.get(run.id));
  expect(await readdir(history.directory)).toEqual([`${run.id}.jsonl`]);
});

test("loads completed details on demand without retaining event arrays", async () => {
  const history = await store();
  const workflow = task(
    { name: "Logs", input: Type.String(), output: Type.String() },
    (ctx, input) => { log.info("A retained journal event"); return input; },
  );
  const started = await history.start("logs", workflow, "first output", "web");
  await started.completion;
  const journal = await readFile(resolve(history.directory, `${started.id}.jsonl`), "utf8");
  const restored = new RunStore(history.directory);
  await restored.init();
  for (const reader of [history, restored]) {
    expect(reader.list()[0]).not.toHaveProperty("events");
    const first = (await reader.get(started.id))!;
    expect(first.output).toBe("first output");
    first.events.length = 0;
    expect((await reader.get(started.id))!.events.length).toBeGreaterThan(0);
    // Both a just-finished run and a restored run read the file on demand.
    await rm(resolve(history.directory, `${started.id}.jsonl`));
    await expect(reader.get(started.id)).rejects.toThrow();
    await writeFile(resolve(history.directory, `${started.id}.jsonl`), journal);
  }
});

test("cancels only the selected run, keeps cleanup and usage, and restores cancellation", async () => {
  const history = await store();
  const ready = Promise.withResolvers<void>();
  let cleaned = false;
  const workflow = task(
    { name: "Cancellable", input: Type.String(), output: Type.String() },
    async (ctx) => {
      ctx.recordUsage({ input: 10, output: 1, cacheRead: 0, cacheWrite: 0 });
      try {
        ready.resolve();
        await new Promise<void>((resolve) => ctx.signal.addEventListener("abort", () => resolve(), { once: true }));
        return "Caught cancellation";
      } finally {
        cleaned = true;
      }
    },
  );
  const first = await history.start("cancel", workflow, "", "web");
  await ready.promise;
  const other = await history.start("other", task(
    { name: "Other", input: Type.String(), output: Type.String() },
    (ctx) => { expect(ctx.signal.aborted).toBe(false); return "done"; },
  ), "", "web");
  expect(history.cancel("missing")).toBe(false);
  expect(history.cancel(first.id)).toBe(true);
  expect(history.cancel(first.id)).toBe(true);
  expect(await first.completion).toMatchObject({ ok: false, output: null, error: "Workflow cancelled by user." });
  await other.completion;
  expect(cleaned).toBe(true);
  expect(history.cancel(first.id)).toBe(false);
  expect((await history.get(first.id))).toMatchObject({ status: "cancelled", usage: { input: 10 } });
  expect((await history.get(other.id))?.status).toBe("completed");
  const restored = new RunStore(history.directory);
  await restored.init();
  expect(await restored.get(first.id)).toEqual(await history.get(first.id));
});
