import { describe, expect, test, vi } from "vitest";
import { createCli } from "./cli.ts";

function harness() {
  const run = vi.fn(async () => {});
  const serve = vi.fn(async () => {});
  let stdout = "";
  let stderr = "";
  const cli = createCli("1.2.3", { run, serve });
  for (const command of [cli, ...cli.commands]) command.configureOutput({
    writeOut: (text) => { stdout += text; },
    writeErr: (text) => { stderr += text; },
  });
  return { run, serve, parse: (args: string[]) => cli.parseAsync(args, { from: "user" }),
    stdout: () => stdout, stderr: () => stderr };
}

describe("CLI commands", () => {
  test("passes a file, prompt, and defaults to the runner", async () => {
    const h = harness();
    await h.parse(["run", "workflow.ts", "make the change"]);
    expect(h.run).toHaveBeenCalledWith("workflow.ts", "make the change", {
      json: false, log: false, verbose: false,
    });
    expect(h.serve).not.toHaveBeenCalled();
  });

  test("accepts options around arguments and an omitted prompt", async () => {
    const h = harness();
    await h.parse(["run", "-v", "workflow.ts", "--json", "--log"]);
    expect(h.run).toHaveBeenCalledWith("workflow.ts", "", {
      json: true, log: true, verbose: true,
    });
  });

  test("preserves a flag-like prompt after the option terminator", async () => {
    const h = harness();
    await h.parse(["run", "workflow.ts", "--", "--json"]);
    expect(h.run).toHaveBeenCalledWith("workflow.ts", "--json", {
      json: false, log: false, verbose: false,
    });
  });

  test("passes JSON input without interpreting it as a prompt", async () => {
    const h = harness();
    const input = '{"directory":"/project","prompt":"change"}';
    await h.parse(["run", "workflow.ts", "--input", input]);
    expect(h.run).toHaveBeenCalledWith("workflow.ts", "", {
      json: false, log: false, verbose: false, input,
    });
  });

  test("rejects a prompt combined with JSON input", async () => {
    const h = harness();
    await expect(h.parse(["run", "workflow.ts", "prompt", "--input", "{}"])).rejects.toThrow("Use either a prompt or --input");
    expect(h.run).not.toHaveBeenCalled();
    expect(h.stderr()).toContain("Use either a prompt or --input");
  });

  test("passes server defaults", async () => {
    const h = harness();
    await h.parse(["serve"]);
    expect(h.serve).toHaveBeenCalledWith({ config: "runling.config.ts", host: "localhost", port: 5173, open: false });
    expect(h.run).not.toHaveBeenCalled();
  });

  test("passes parsed server options", async () => {
    const h = harness();
    await h.parse(["serve", "--port=3000", "--host", "::1", "--config", "custom.ts", "--open"]);
    expect(h.serve).toHaveBeenCalledWith({ config: "custom.ts", host: "::1", port: 3000, open: true });
  });

  test.each(["nope", "0", "65536", "1.5", "1e3", "", "-1"])("rejects invalid port %j before starting the server", async (port) => {
    const h = harness();
    await expect(h.parse(["serve", `--port=${port}`])).rejects.toThrow("Port must be an integer from 1 through 65535");
    expect(h.serve).not.toHaveBeenCalled();
  });

  test.each([
    ["run"], ["run", "workflow.ts", "too", "many"],
    ["run", "workflow.ts", "--unknown"], ["serve", "--json"],
    ["web"], ["workflow.ts"], ["--port", "3000"], ["serve", "extra"],
  ])("rejects invalid invocation %j without executing", async (...args) => {
    const h = harness();
    await expect(h.parse(args)).rejects.toThrow();
    expect(h.run).not.toHaveBeenCalled();
    expect(h.serve).not.toHaveBeenCalled();
    expect(h.stderr()).toContain("error:");
  });

  test.each([[], ["--help"], ["run", "--help"], ["serve", "--help"], ["help", "run"], ["--version"]])("shows help or version for %j without executing", async (...args) => {
    const h = harness();
    try { await h.parse(args); } catch (error) { expect(error).toMatchObject({ exitCode: 0 }); }
    expect(h.stdout()).toContain(args[0] === "--version" ? "1.2.3" : "Usage:");
    expect(h.stderr()).toBe("");
    expect(h.run).not.toHaveBeenCalled();
    expect(h.serve).not.toHaveBeenCalled();
  });
});
