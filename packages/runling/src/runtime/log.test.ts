import { ansiColor } from "./ansi.ts";
import { afterEach, describe, expect, test, vi } from "vitest";
import { log } from "./log.ts";
import { step } from "./step.ts";

describe("log", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    log.level = "info";
  });

  test("debug does not print at the default info level", () => {
    const print = vi.spyOn(console, "log");
    log.level = "info";

    log.debug("hidden detail");

    expect(print).not.toHaveBeenCalled();
  });

  test("debug prints at the debug level", () => {
    const print = vi.spyOn(console, "log");
    log.level = "debug";

    log.debug("visible detail");

    expect(print).toHaveBeenCalledTimes(1);
    expect(print.mock.calls[0]?.[0]).toContain("visible detail");
  });

  test("info, success, and error print at the info level", () => {
    const print = vi.spyOn(console, "log");
    const printError = vi.spyOn(console, "error");
    log.level = "info";

    log.info("an info message");
    log.success("a success message");
    log.error("an error message");

    expect(print).toHaveBeenCalledTimes(2);
    expect(print.mock.calls[0]?.[0]).toContain("an info message");
    expect(print.mock.calls[1]?.[0]).toContain("a success message");
    expect(printError).toHaveBeenCalledTimes(1);
    expect(printError.mock.calls[0]?.[0]).toContain("an error message");
  });

  test("info, success, and error print at the debug level too", () => {
    const print = vi.spyOn(console, "log");
    log.level = "debug";

    log.info("an info message");
    log.success("a success message");

    expect(print).toHaveBeenCalledTimes(2);
  });

  test("highlights text in a bold, bright color", () => {
    const highlighted = log.highlight("Reading", "#40c057");

    expect(highlighted.startsWith("\x1b[1m")).toBe(true);
    expect(highlighted).toContain(ansiColor("#40c057") ?? "");
    expect(highlighted).toContain("Reading");
    expect(highlighted.endsWith("\x1b[0m")).toBe(true);
  });

  test("can route informational output to stderr", async () => {
    const print = vi.spyOn(console, "log");
    const printError = vi.spyOn(console, "error");

    await log.withDestination("stderr", async () => {
      log.info("machine-readable companion log");
      log.success("completed");
    });

    expect(print).not.toHaveBeenCalled();
    expect(printError).toHaveBeenCalledTimes(2);
  });

  test("indents output one level deeper inside a step", () => {
    const print = vi.spyOn(console, "log");
    const result = step("Running a step", () => {
      log.info("inside the step");
      return 42;
    });

    expect(result).toBe(42);
    expect(print.mock.calls.map((call) => call[0] as string)).toHaveLength(2);
    expect(print.mock.calls[0]?.[0]).toContain("Running a step");
    expect(print.mock.calls[0]?.[0]).not.toMatch(/^\s/);
    expect(print.mock.calls[1]?.[0]).toContain("inside the step");
    expect(print.mock.calls[1]?.[0]).toMatch(/^ {2}/);
  });

  test("restores the indentation level after a step finishes", async () => {
    const print = vi.spyOn(console, "log");

    await step("outer", async () => {
      log.info("inside outer");
      await step("inner", async () => {
        log.info("inside inner");
      });
      log.info("back inside outer");
    });
    log.info("after the step");

    const lines = print.mock.calls.map((call) => call[0] as string);
    const depthOf = (message: string) => {
      const line = lines.find((line) => line.includes(message));
      expect(line).toBeDefined();
      return line?.match(/^ */)?.[0].length;
    };

    expect(depthOf("inside outer")).toBe(2);
    expect(depthOf("inside inner")).toBe(4);
    expect(depthOf("back inside outer")).toBe(2);
    expect(depthOf("after the step")).toBe(0);
  });

  test("restores the indentation level when a step throws", async () => {
    const print = vi.spyOn(console, "log");

    await expect(
      step("failing step", async () => {
        throw new Error("boom");
      }),
    ).rejects.toThrow("boom");

    log.info("after the failure");
    expect(print.mock.calls.at(-1)?.[0]).not.toMatch(/^\s/);
  });

  test("keeps concurrent steps at their own indentation level", async () => {
    const print = vi.spyOn(console, "log");
    let releaseA: () => void = () => {};
    const gate = new Promise<void>((resolveGate) => {
      releaseA = resolveGate;
    });

    const a = step("step a", async () => {
      await gate;
      log.info("from step a");
    });
    const b = step("step b", async () => {
      log.info("from step b");
    });

    await b;
    releaseA();
    await a;

    const lines = print.mock.calls.map((call) => call[0] as string);
    const depthOf = (message: string) =>
      lines.find((line) => line.includes(message))?.match(/^ */)?.[0].length;

    expect(depthOf("from step a")).toBe(2);
    expect(depthOf("from step b")).toBe(2);
  });

  test("supports sync and async work without a label", async () => {
    const print = vi.spyOn(console, "log");

    const sync = log.indented(() => {
      log.info("sync work");
      return "sync";
    });
    const async = log.indented(async () => {
      log.info("async work");
      return "async";
    });

    expect(sync).toBe("sync");
    await expect(async).resolves.toBe("async");
    expect(print.mock.calls[0]?.[0]).toMatch(/^ {2}/);
    expect(print.mock.calls[1]?.[0]).toMatch(/^ {2}/);
  });
});
