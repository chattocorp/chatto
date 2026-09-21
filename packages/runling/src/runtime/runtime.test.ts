import { createWorkflowContext } from "./context.ts";
import { describe, expect, test } from "vitest";
import { runWorkflow } from "./runner.ts";
import { task } from "./workflow.ts";
import { input } from "./input.ts";
import { log } from "./log.ts";
import type { RunlingEvent } from "./events.ts";

describe("execution services", () => {
  test("isolates concurrent input handlers, logs, and verbosity", async () => {
    const first = Promise.withResolvers<string>();
    const second = Promise.withResolvers<string>();
    const events: RunlingEvent[][] = [[], []];
    const ask = task(async function ask(ctx, message: string) {
      const answer = await input(ctx, message);
      log.debug(`debug:${answer}`);
      log.info(answer);
      return answer;
    });
    const one = runWorkflow(ask, { input: "one", verbose: true, onInput: () => first.promise, onEvent: e => events[0]!.push(e) });
    const two = runWorkflow(ask, { input: "two", verbose: false, onInput: () => second.promise, onEvent: e => events[1]!.push(e) });
    second.resolve("second answer");
    first.resolve("first answer");
    expect((await one).output).toBe("first answer");
    expect((await two).output).toBe("second answer");
    expect(events[0]!.filter(e => e.type === "log").filter(e => e.source === undefined).map(e => e.message)).toEqual(["debug:first answer", "first answer"]);
    expect(events[1]!.filter(e => e.type === "log").filter(e => e.source === undefined).map(e => e.message)).toEqual(["second answer"]);
    await expect(input(createWorkflowContext(), "outside a run")).rejects.toThrow("this host cannot provide it");
  });
});
