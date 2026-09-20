import { spawnProcess } from "../test/process.ts";
import { describe, expect, test } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const executable = resolve(import.meta.dirname, "runling.js");
const fixture = resolve(import.meta.dirname, "../test/fixtures/echo-workflow.ts");

describe.each([
  { mode: "compiled", flags: [] },
  { mode: "source", flags: ["--conditions=runling-source"] },
])("runling executable ($mode)", ({ flags }) => {
  test.each([
    { args: [], expected: "Usage: runling" },
    { args: ["--help"], expected: "serve" },
    { args: ["run", "--help"], expected: "<file> [prompt]" },
    { args: ["serve", "--help"], expected: "--config <path>" },
    { args: ["help", "run"], expected: "<file> [prompt]" },
    { args: ["--version"], expected: JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")).version },
  ])("prints help or version for $args", async ({ args, expected }) => {
    const child = spawnProcess([process.execPath, ...flags, executable, ...args], {
      cwd: import.meta.dirname,
      stdout: "pipe",
      stderr: "pipe",
    });
    const [exitCode, stdout, stderr] = await Promise.all([
      child.exited, new Response(child.stdout).text(), new Response(child.stderr).text(),
    ]);
    expect(exitCode).toBe(0);
    expect(stdout).toContain(expected);
    expect(stdout).not.toContain("Runling starting");
    expect(stderr).toBe("");
  });

  test("rejects a missing file before workflow execution", async () => {
    const child = spawnProcess([process.execPath, ...flags, executable, "run", "--json"], {
      stdout: "pipe", stderr: "pipe",
    });
    const [exitCode, stdout, stderr] = await Promise.all([
      child.exited, new Response(child.stdout).text(), new Response(child.stderr).text(),
    ]);
    expect(exitCode).toBe(1);
    expect(stdout).toBe("");
    expect(stderr).toContain("missing required argument 'file'");
    expect(stderr).not.toContain("at ");
  });

  test("treats a flag-like prompt as input after --", async () => {
    const child = spawnProcess([process.execPath, ...flags, executable, "run", fixture, "--json", "--", "--verbose"], {
      stdout: "pipe", stderr: "pipe",
    });
    const [exitCode, stdout] = await Promise.all([
      child.exited, new Response(child.stdout).text(),
    ]);
    expect(exitCode).toBe(0);
    expect(JSON.parse(stdout).result.summary).toBe("--verbose");
  });

  test("passes structured input and an explicit directory to a task", async () => {
    const directory = import.meta.dirname;
    const child = spawnProcess([
      process.execPath, ...flags, executable, "run",
      resolve(directory, "../test/fixtures/directory-task.ts"),
      "--input", JSON.stringify({ directory, prompt: "structured input" }), "--json",
    ], { stdout: "pipe", stderr: "pipe" });
    const [exitCode, stdout, stderr] = await Promise.all([
      child.exited, new Response(child.stdout).text(), new Response(child.stderr).text(),
    ]);
    expect(exitCode, stderr).toBe(0);
    expect(JSON.parse(stdout).output).toEqual({ directory, prompt: "structured input" });
  });

  test("reports malformed JSON input without starting the task", async () => {
    const child = spawnProcess([
      process.execPath, ...flags, executable, "run", fixture, "--input", "{", "--json",
    ], { stdout: "pipe", stderr: "pipe" });
    const [exitCode, stdout] = await Promise.all([child.exited, new Response(child.stdout).text()]);
    expect(exitCode).toBe(1);
    expect(JSON.parse(stdout)).toMatchObject({ ok: false, output: null, error: expect.any(String) });
  });

  test("loads a task file and passes context and input", async () => {
    const child = spawnProcess(
      [process.execPath, ...flags, executable, "run", fixture, "A workflow result"],
      {
        cwd: import.meta.dirname,
        stdout: "pipe",
        stderr: "pipe",
      },
    );

    const [exitCode, stdout, stderr] = await Promise.all([
      child.exited,
      new Response(child.stdout).text(),
      new Response(child.stderr).text(),
    ]);

    expect(exitCode).toBe(0);
    expect(stderr).toBe("");
    expect(stdout).toContain("A workflow result");
    expect(stdout).toContain("Finished in ");
  });

  test("runs a workflow without a prompt", async () => {
    const child = spawnProcess(
      [process.execPath, ...flags, executable, "run", fixture, "--json"],
      {
        stdout: "pipe",
        stderr: "pipe",
      },
    );

    const [exitCode, stdout] = await Promise.all([
      child.exited,
      new Response(child.stdout).text(),
    ]);

    expect(exitCode).toBe(0);
    expect(JSON.parse(stdout).result.summary).toBe("");
  });

  test("reports invalid invocations without a stack trace", async () => {
    const child = spawnProcess([process.execPath, ...flags, executable, "serve", "--port", "not-a-port"], {
      stdout: "pipe",
      stderr: "pipe",
    });

    const [exitCode, stderr] = await Promise.all([
      child.exited,
      new Response(child.stderr).text(),
    ]);

    expect(exitCode).toBe(1);
    expect(stderr).toContain("Port must be an integer from 1 through 65535");
    expect(stderr).not.toContain("at ");
  });

  test("prints one structured document to stdout in JSON mode", async () => {
    const child = spawnProcess(
      [process.execPath, ...flags, executable, "run", fixture, "--json", "A JSON result"],
      {
        stdout: "pipe",
        stderr: "pipe",
      },
    );

    const [exitCode, stdout, stderr] = await Promise.all([
      child.exited,
      new Response(child.stdout).text(),
      new Response(child.stderr).text(),
    ]);

    expect(exitCode).toBe(0);
    const execution = JSON.parse(stdout);
    expect(execution.ok).toBe(true);
    expect(execution.error).toBeNull();
    expect(execution.result.summary).toBe("A JSON result");
    expect(execution.result.outputs.id).toMatch(/^[a-z]+-[a-z]+-\d{4}$/);
    expect(execution.durationMs).toBeTypeOf("number");
    expect(execution.usage).toEqual({
      input: 0,
      output: 0,
      cacheRead: 0,
      cacheWrite: 0,
    });
    expect(stderr).toContain("Runling starting");
    expect(stderr).toContain("Finished in ");
  });

  test("reports failures as JSON with a nonzero exit status", async () => {
    const child = spawnProcess([process.execPath, ...flags, executable, "run", "missing-workflow.ts", "--json"], {
      stdout: "pipe",
      stderr: "pipe",
    });

    const [exitCode, stdout, stderr] = await Promise.all([
      child.exited,
      new Response(child.stdout).text(),
      new Response(child.stderr).text(),
    ]);

    expect(exitCode).toBe(1);
    expect(JSON.parse(stdout)).toMatchObject({
      ok: false,
      result: null,
      error: expect.stringContaining("missing-workflow.ts"),
    });
    expect(stderr).toContain("missing-workflow.ts");
  });
});
