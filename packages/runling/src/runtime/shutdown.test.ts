import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "vitest";

test("repeated shutdown signals flush an idle conversation with a completed child", async () => {
  const directory = await mkdtemp(join(tmpdir(), "runling-shutdown-"));
  try {
    await promisify(execFile)(process.execPath, ["--import", "tsx", "--input-type=module", "--eval", `
      import { RunStore } from ${JSON.stringify(new URL("../lib/server/run-store.ts", import.meta.url).href)};
      import { installShutdown } from ${JSON.stringify(new URL("./shutdown.ts", import.meta.url).href)};
      import { createAgentTasks, runAgentConversation } from "runling/agents";
      import { task, emptyTokenUsage } from "runling";
      import { setTimeout as delay } from "node:timers/promises";
      const store = new RunStore(process.argv[1]);
      await store.init();
      installShutdown(async () => {
        setTimeout(() => process.kill(process.pid, "SIGINT"), 5);
        setTimeout(() => process.kill(process.pid, "SIGTERM"), 10);
        await store.close();
      });
      await store.start("conversation", task(async ctx => {
        const tasks = createAgentTasks(ctx);
        let turns = 0;
        const owner = { steer: async () => false, async runOutcome() {
          if (++turns === 1) tasks.start("child", async () => "evidence", undefined);
          return { outcome: "completed", summary: "", usage: emptyTokenUsage() };
        } };
        try {
          await runAgentConversation(ctx, owner, "Investigate", {
            notifications: tasks.notifications, keepAlive: () => tasks.active,
            onBusy(busy) { if (!busy && turns === 2 && !ctx.signal.aborted) setTimeout(() => process.kill(process.pid, "SIGINT"), 1); },
          });
        } finally { await tasks.dispose(); await delay(50, undefined, { ref: false }); }
      }), undefined, "source");
    `, directory], { timeout: 10_000 });
    const [file] = await readdir(directory);
    const records = (await readFile(join(directory, file!), "utf8")).trim().split("\n").map(line => JSON.parse(line));
    expect(records.at(-1)).toMatchObject({ type: "finished", status: "cancelled" });
    expect(records.filter(record => record.type === "finished")).toHaveLength(1);
    expect(records.some(record => record.event?.type === "input.finished" && record.event.reason === "cancelled")).toBe(true);
  } finally { await rm(directory, { recursive: true, force: true }); }
});
