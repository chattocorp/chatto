import { expect, test, vi } from "vitest";
import { createWorkflowContext, emptyTokenUsage } from "runling";
import { runChattoAgent } from "./agent.ts";

function fixture() {
  let messages: string[] = [];
  const listeners = new Set<() => void>();
  const inbox = {
    drain: () => messages.splice(0),
    prepend: (values: string[]) => {
      messages.unshift(...values);
    },
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    push: (text: string) => {
      messages.push(text);
      for (const listener of listeners) listener();
    },
  };
  const done = Promise.withResolvers<any>();
  const agent = {
    runOutcome: vi.fn(() => done.promise),
    steer: vi.fn(async (_text: string) => true),
  };
  const ctx = createWorkflowContext();
  const run = () =>
    runChattoAgent(ctx, agent, "Go", {
      destination: { roomId: "room", threadRootId: "root" },
      inbox,
      post: async () => {},
      reservedCommands: ["/approve"],
    });
  const finish = () =>
    done.resolve({
      outcome: "completed",
      summary: "Done",
      usage: emptyTokenUsage(),
    });
  return { inbox, listeners, done, agent, ctx, run, finish };
}

test("streams input and restores commands and missed messages in original order", async () => {
  const f = fixture();
  const late = Promise.withResolvers<boolean>();
  f.agent.steer.mockImplementation(async (text) =>
    text === "late" ? late.promise : true,
  );
  f.inbox.push("initial");
  const run = f.run();
  f.inbox.push(" /approve ");
  f.inbox.push("late");
  await vi.waitFor(() =>
    expect(f.agent.steer.mock.calls).toEqual([["initial"], ["late"]]),
  );
  f.finish();
  late.resolve(false);
  await run;
  f.inbox.push("after completion");

  expect(f.inbox.drain()).toEqual([" /approve ", "late", "after completion"]);
  expect(f.listeners.size).toBe(0);
});

test("restores rejected steering when an interaction fails", async () => {
  const f = fixture();
  f.agent.steer.mockRejectedValue(new Error("cannot steer"));
  const run = f.run();
  const failed = expect(run).rejects.toThrow("work failed");
  f.inbox.push("feedback");
  await vi.waitFor(() => expect(f.agent.steer).toHaveBeenCalledOnce());
  f.done.reject(new Error("work failed"));
  await failed;

  expect(f.inbox.drain()).toEqual(["feedback"]);
  expect(f.listeners.size).toBe(0);
});

test("identical messages have independent delivery acknowledgements", async () => {
  const f = fixture();
  f.agent.steer.mockResolvedValueOnce(true).mockResolvedValueOnce(false);
  const run = f.run();
  f.inbox.push("same");
  f.inbox.push("same");
  await vi.waitFor(() => expect(f.agent.steer).toHaveBeenCalledTimes(2));
  f.finish();
  await run;

  expect(f.inbox.drain()).toEqual(["same"]);
});

test("overflow remains in the host backlog and can be delivered on a later turn", async () => {
  const f = fixture();
  const gate = Promise.withResolvers<boolean>();
  f.agent.steer.mockImplementationOnce(() => gate.promise);
  const run = f.run();
  for (let i = 0; i < 100; i++) f.inbox.push(String(i));
  await vi.waitFor(() => expect(f.agent.steer).toHaveBeenCalledTimes(65));
  gate.resolve(true);
  await vi.waitFor(() => expect(f.agent.steer).toHaveBeenCalledTimes(65));
  f.finish();
  await run;

  const retained = f.inbox.drain();
  expect(retained).toEqual(
    Array.from({ length: 35 }, (_, i) => String(i + 65)),
  );
  f.inbox.prepend(retained);
  const next = Promise.withResolvers<any>();
  f.agent.runOutcome.mockReturnValueOnce(next.promise);
  const retry = f.run();
  await vi.waitFor(() => expect(f.agent.steer).toHaveBeenCalledTimes(100));
  next.resolve(await f.done.promise);
  await retry;
  expect(f.inbox.drain()).toEqual([]);
});

test("cancellation releases a stuck delivery and retains unacknowledged input", async () => {
  const f = fixture();
  f.agent.steer.mockReturnValue(new Promise(() => {}));
  const run = f.run();
  const failed = expect(run).rejects.toThrow("stop");
  f.inbox.push("pending");
  await vi.waitFor(() => expect(f.agent.steer).toHaveBeenCalledOnce());
  try {
    f.ctx.abort("stop");
  } catch {}
  await failed;

  expect(f.inbox.drain()).toEqual(["pending"]);
  expect(f.listeners.size).toBe(0);
});
