import { expect, test } from "vitest";
import { implementationProcess, ImplementationCommandError } from "./implementation-process.ts";

test("keeps failing command diagnostics out of the host error message", async () => {
  try {
    await implementationProcess(process.execPath, ["-e", "console.error('private diagnostic'); process.exit(2)"], { cwd: process.cwd(), signal: new AbortController().signal, captureDiagnostics: true });
    throw new Error("Expected failure");
  } catch (error) {
    expect(error).toBeInstanceOf(ImplementationCommandError);
    expect((error as Error).message).not.toContain("private");
    expect((error as ImplementationCommandError).output).toContain("private diagnostic");
  }
});

test("host command failures discard raw diagnostics by default", async () => {
  await expect(implementationProcess(process.execPath, ["-e", "console.error('private diagnostic'); process.exit(2)"], {
    cwd: process.cwd(), signal: new AbortController().signal,
  })).rejects.not.toHaveProperty("output");
});

test("cancels a shell and a child that ignores SIGTERM", async () => {
  const started = Date.now();
  await expect(implementationProcess("bash", ["-c", "trap '' TERM; sleep 30 & wait"], {
    cwd: process.cwd(), signal: new AbortController().signal, timeoutMs: 100,
  })).rejects.toThrow("cancelled or timed out");
  expect(Date.now() - started).toBeLessThan(5000);
});

test("removes selected inherited variables from repository commands", async () => {
  const key = "CHATTO_ALLOWED_USER_ID";
  const before = process.env[key];
  process.env[key] = "private-test-value";
  try {
    const output = await implementationProcess(process.execPath, ["-e", `process.stdout.write(process.env.${key} ?? "absent")`], {
      cwd: process.cwd(), signal: new AbortController().signal, unsetEnv: [key],
    });
    expect(output).toBe("absent");
  } finally {
    if (before === undefined) delete process.env[key];
    else process.env[key] = before;
  }
});
